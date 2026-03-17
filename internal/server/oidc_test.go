package server

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockOIDCProvider creates a test HTTP server that serves OIDC discovery and JWKS endpoints.
func mockOIDCProvider(t *testing.T) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	var serverURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                serverURL,
			"authorization_endpoint":                serverURL + "/auth",
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

	server := httptest.NewServer(mux)
	serverURL = server.URL

	return server, privateKey
}

// signOIDCToken creates a signed JWT using the test RSA key.
func signOIDCToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key-1"
	tokenStr, err := token.SignedString(key)
	require.NoError(t, err)
	return tokenStr
}

func TestOIDCMiddleware_MissingConfig(t *testing.T) {
	_, err := newOIDCMiddleware(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "security.oidc config is required")
}

func TestOIDCMiddleware_MissingIssuerURL(t *testing.T) {
	_, err := newOIDCMiddleware(map[string]any{
		"client_id": "my-client",
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "issuer_url is required")
}

func TestOIDCMiddleware_MissingClientID(t *testing.T) {
	_, err := newOIDCMiddleware(map[string]any{
		"issuer_url": "https://example.com",
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_id is required")
}

func TestOIDCMiddleware_InvalidIssuerURL(t *testing.T) {
	_, err := newOIDCMiddleware(map[string]any{
		"issuer_url": "http://localhost:1/nonexistent",
		"client_id":  "my-client",
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OIDC discovery failed")
}

func TestOIDCMiddleware_ValidToken(t *testing.T) {
	oidcServer, privateKey := mockOIDCProvider(t)
	defer oidcServer.Close()

	handler, err := newOIDCMiddleware(map[string]any{
		"issuer_url": oidcServer.URL,
		"client_id":  "test-client",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(handler)
	app.Get("/protected", func(c fiber.Ctx) error {
		claims := c.Locals(api.LocalJWTClaims)
		userID := c.Locals(api.LocalJWTUserID)
		roles := c.Locals(api.LocalJWTRoles)
		return c.JSON(map[string]any{
			"claims":  claims,
			"user_id": userID,
			"roles":   roles,
		})
	})

	tokenStr := signOIDCToken(t, privateKey, jwt.MapClaims{
		"iss":   oidcServer.URL,
		"aud":   "test-client",
		"sub":   "user-456",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"roles": []string{"admin", "editor"},
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "user-456")
	assert.Contains(t, bodyStr, "admin")
}

func TestOIDCMiddleware_MissingAuthHeader(t *testing.T) {
	oidcServer, _ := mockOIDCProvider(t)
	defer oidcServer.Close()

	handler, err := newOIDCMiddleware(map[string]any{
		"issuer_url": oidcServer.URL,
		"client_id":  "test-client",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(handler)
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestOIDCMiddleware_InvalidAuthFormat(t *testing.T) {
	oidcServer, _ := mockOIDCProvider(t)
	defer oidcServer.Close()

	handler, err := newOIDCMiddleware(map[string]any{
		"issuer_url": oidcServer.URL,
		"client_id":  "test-client",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(handler)
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestOIDCMiddleware_ExpiredToken(t *testing.T) {
	oidcServer, privateKey := mockOIDCProvider(t)
	defer oidcServer.Close()

	handler, err := newOIDCMiddleware(map[string]any{
		"issuer_url": oidcServer.URL,
		"client_id":  "test-client",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(handler)
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	tokenStr := signOIDCToken(t, privateKey, jwt.MapClaims{
		"iss": oidcServer.URL,
		"aud": "test-client",
		"sub": "user-456",
		"exp": time.Now().Add(-time.Hour).Unix(),
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestOIDCMiddleware_WrongAudience(t *testing.T) {
	oidcServer, privateKey := mockOIDCProvider(t)
	defer oidcServer.Close()

	handler, err := newOIDCMiddleware(map[string]any{
		"issuer_url": oidcServer.URL,
		"client_id":  "test-client",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(handler)
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	tokenStr := signOIDCToken(t, privateKey, jwt.MapClaims{
		"iss": oidcServer.URL,
		"aud": "wrong-client",
		"sub": "user-456",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestOIDCMiddleware_CustomClaims(t *testing.T) {
	oidcServer, privateKey := mockOIDCProvider(t)
	defer oidcServer.Close()

	handler, err := newOIDCMiddleware(map[string]any{
		"issuer_url":    oidcServer.URL,
		"client_id":     "test-client",
		"user_id_claim": "email",
		"roles_claim":   "groups",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(handler)
	app.Get("/protected", func(c fiber.Ctx) error {
		userID := c.Locals(api.LocalJWTUserID)
		roles := c.Locals(api.LocalJWTRoles)
		return c.JSON(map[string]any{
			"user_id": userID,
			"roles":   roles,
		})
	})

	tokenStr := signOIDCToken(t, privateKey, jwt.MapClaims{
		"iss":    oidcServer.URL,
		"aud":    "test-client",
		"sub":    "user-456",
		"email":  "test@example.com",
		"groups": []string{"staff", "devops"},
		"exp":    time.Now().Add(time.Hour).Unix(),
		"iat":    time.Now().Unix(),
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "test@example.com")
	assert.Contains(t, bodyStr, "staff")
	assert.Contains(t, bodyStr, "devops")
}

func TestOIDCMiddleware_RequiredScopes_Valid(t *testing.T) {
	oidcServer, privateKey := mockOIDCProvider(t)
	defer oidcServer.Close()

	handler, err := newOIDCMiddleware(map[string]any{
		"issuer_url":      oidcServer.URL,
		"client_id":       "test-client",
		"required_scopes": []any{"openid", "profile"},
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(handler)
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	tokenStr := signOIDCToken(t, privateKey, jwt.MapClaims{
		"iss":   oidcServer.URL,
		"aud":   "test-client",
		"sub":   "user-456",
		"scope": "openid profile email",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestOIDCMiddleware_RequiredScopes_Missing(t *testing.T) {
	oidcServer, privateKey := mockOIDCProvider(t)
	defer oidcServer.Close()

	handler, err := newOIDCMiddleware(map[string]any{
		"issuer_url":      oidcServer.URL,
		"client_id":       "test-client",
		"required_scopes": []any{"admin"},
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(handler)
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	tokenStr := signOIDCToken(t, privateKey, jwt.MapClaims{
		"iss":   oidcServer.URL,
		"aud":   "test-client",
		"sub":   "user-456",
		"scope": "openid profile",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestOIDCMiddleware_SameLocalsAsJWT(t *testing.T) {
	oidcServer, privateKey := mockOIDCProvider(t)
	defer oidcServer.Close()

	handler, err := newOIDCMiddleware(map[string]any{
		"issuer_url": oidcServer.URL,
		"client_id":  "test-client",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(handler)
	app.Get("/protected", func(c fiber.Ctx) error {
		_, hasClaims := c.Locals(api.LocalJWTClaims).(map[string]any)
		_, hasUserID := c.Locals(api.LocalJWTUserID).(string)
		_, hasRoles := c.Locals(api.LocalJWTRoles).([]string)

		return c.JSON(map[string]any{
			"has_claims":  hasClaims,
			"has_user_id": hasUserID,
			"has_roles":   hasRoles,
		})
	})

	tokenStr := signOIDCToken(t, privateKey, jwt.MapClaims{
		"iss":   oidcServer.URL,
		"aud":   "test-client",
		"sub":   "user-789",
		"roles": []string{"viewer"},
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, `"has_claims":true`)
	assert.Contains(t, bodyStr, `"has_user_id":true`)
	assert.Contains(t, bodyStr, `"has_roles":true`)
}

func TestBuildMiddleware_OIDC_ViaRegistry(t *testing.T) {
	oidcServer, _ := mockOIDCProvider(t)
	defer oidcServer.Close()

	h, err := BuildMiddleware("auth.oidc", map[string]any{
		"security": map[string]any{
			"oidc": map[string]any{
				"issuer_url": oidcServer.URL,
				"client_id":  "test-client",
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, h)
}

func TestOIDCMiddleware_InvalidToken(t *testing.T) {
	oidcServer, _ := mockOIDCProvider(t)
	defer oidcServer.Close()

	handler, err := newOIDCMiddleware(map[string]any{
		"issuer_url": oidcServer.URL,
		"client_id":  "test-client",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(handler)
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-string")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}
