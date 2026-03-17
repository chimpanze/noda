package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockOIDCExchangeServer creates a mock OIDC server with token exchange support.
func mockOIDCExchangeServer(t *testing.T) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	var serverURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                serverURL,
			"authorization_endpoint":                serverURL + "/authorize",
			"token_endpoint":                        serverURL + "/token",
			"jwks_uri":                              serverURL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"alg": "RS256",
					"use": "sig",
					"kid": "test-key-1",
					"n":   base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.E)).Bytes()),
				},
			},
		})
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		code := r.FormValue("code")
		grantType := r.FormValue("grant_type")
		clientID := r.FormValue("client_id")
		if clientID == "" {
			// Try basic auth
			clientID, _, _ = r.BasicAuth()
		}
		if clientID == "" {
			clientID = "test-client"
		}

		if grantType == "authorization_code" && code == "valid-code" {
			idToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
				"iss":   serverURL,
				"aud":   clientID,
				"sub":   "user-from-exchange",
				"email": "exchanged@example.com",
				"exp":   time.Now().Add(time.Hour).Unix(),
				"iat":   time.Now().Unix(),
			})
			idToken.Header["kid"] = "test-key-1"
			idTokenStr, _ := idToken.SignedString(privateKey)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "mock-access-token",
				"token_type":    "Bearer",
				"refresh_token": "mock-refresh-token",
				"id_token":      idTokenStr,
				"expires_in":    3600,
			})
			return
		}

		if grantType == "refresh_token" {
			idToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
				"iss":   serverURL,
				"aud":   clientID,
				"sub":   "user-refreshed",
				"email": "refreshed@example.com",
				"exp":   time.Now().Add(time.Hour).Unix(),
				"iat":   time.Now().Unix(),
			})
			idToken.Header["kid"] = "test-key-1"
			idTokenStr, _ := idToken.SignedString(privateKey)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "new-access-token",
				"token_type":    "Bearer",
				"refresh_token": "new-refresh-token",
				"id_token":      idTokenStr,
				"expires_in":    3600,
			})
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             "invalid_grant",
			"error_description": "invalid code",
		})
	})

	server := httptest.NewServer(mux)
	serverURL = server.URL
	return server, privateKey
}

func TestExchange_ValidCode(t *testing.T) {
	oidcServer, _ := mockOIDCExchangeServer(t)
	defer oidcServer.Close()

	executor := newExchangeExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"issuer_url":    oidcServer.URL,
		"client_id":     "test-client",
		"client_secret": "test-secret",
		"redirect_uri":  "http://localhost:8080/callback",
		"code":          "valid-code",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, "mock-access-token", result["access_token"])
	assert.Equal(t, "mock-refresh-token", result["refresh_token"])
	assert.NotEmpty(t, result["id_token"])
	assert.NotNil(t, result["claims"])

	claims := result["claims"].(map[string]any)
	assert.Equal(t, "user-from-exchange", claims["sub"])
	assert.Equal(t, "exchanged@example.com", claims["email"])
}

func TestExchange_InvalidCode(t *testing.T) {
	oidcServer, _ := mockOIDCExchangeServer(t)
	defer oidcServer.Close()

	executor := newExchangeExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"issuer_url":    oidcServer.URL,
		"client_id":     "test-client",
		"client_secret": "test-secret",
		"redirect_uri":  "http://localhost:8080/callback",
		"code":          "invalid-code",
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token exchange failed")
}

func TestExchange_MissingConfig(t *testing.T) {
	executor := newExchangeExecutor(nil)
	execCtx := engine.NewExecutionContext()

	tests := []struct {
		name   string
		config map[string]any
		errMsg string
	}{
		{
			name:   "missing issuer_url",
			config: map[string]any{"client_id": "x", "client_secret": "x", "redirect_uri": "x", "code": "x"},
			errMsg: "issuer_url is required",
		},
		{
			name:   "missing client_id",
			config: map[string]any{"issuer_url": "x", "client_secret": "x", "redirect_uri": "x", "code": "x"},
			errMsg: "client_id is required",
		},
		{
			name:   "missing client_secret",
			config: map[string]any{"issuer_url": "x", "client_id": "x", "redirect_uri": "x", "code": "x"},
			errMsg: "client_secret is required",
		},
		{
			name:   "missing code",
			config: map[string]any{"issuer_url": "x", "client_id": "x", "client_secret": "x", "redirect_uri": "x"},
			errMsg: "code is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executor.Execute(context.Background(), execCtx, tt.config, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

func TestExchange_Descriptor(t *testing.T) {
	d := &exchangeDescriptor{}
	assert.Equal(t, "exchange", d.Name())
	assert.Nil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	require.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "issuer_url")
	assert.Contains(t, props, "client_id")
	assert.Contains(t, props, "client_secret")
	assert.Contains(t, props, "redirect_uri")
	assert.Contains(t, props, "code")
}

func TestExchange_Outputs(t *testing.T) {
	executor := newExchangeExecutor(nil)
	assert.Equal(t, []string{"success", "error"}, executor.Outputs())
}
