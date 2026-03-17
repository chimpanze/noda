package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockOIDCDiscovery(t *testing.T) *httptest.Server {
	t.Helper()

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

	server := httptest.NewServer(mux)
	serverURL = server.URL
	return server
}

func TestAuthURL_Basic(t *testing.T) {
	oidcServer := mockOIDCDiscovery(t)
	defer oidcServer.Close()

	executor := newAuthURLExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"issuer_url":   oidcServer.URL,
		"client_id":    "my-client-id",
		"redirect_uri": "http://localhost:8080/callback",
		"state":        "random-state-123",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	url := result["url"].(string)
	assert.Contains(t, url, oidcServer.URL+"/authorize")
	assert.Contains(t, url, "client_id=my-client-id")
	assert.Contains(t, url, "redirect_uri=")
	assert.Contains(t, url, "state=random-state-123")
	assert.Contains(t, url, "scope=openid+profile+email")
	assert.Equal(t, "random-state-123", result["state"])
}

func TestAuthURL_CustomScopes(t *testing.T) {
	oidcServer := mockOIDCDiscovery(t)
	defer oidcServer.Close()

	executor := newAuthURLExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"issuer_url":   oidcServer.URL,
		"client_id":    "my-client-id",
		"redirect_uri": "http://localhost:8080/callback",
		"state":        "state-456",
		"scopes":       []any{"openid", "email"},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	url := result["url"].(string)
	assert.Contains(t, url, "scope=openid+email")
	assert.NotContains(t, url, "profile")
}

func TestAuthURL_ExtraParams(t *testing.T) {
	oidcServer := mockOIDCDiscovery(t)
	defer oidcServer.Close()

	executor := newAuthURLExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"issuer_url":   oidcServer.URL,
		"client_id":    "my-client-id",
		"redirect_uri": "http://localhost:8080/callback",
		"state":        "state-789",
		"extra_params": map[string]any{
			"prompt": "consent",
		},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	url := result["url"].(string)
	assert.Contains(t, url, "prompt=consent")
}

func TestAuthURL_InvalidIssuer(t *testing.T) {
	executor := newAuthURLExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"issuer_url":   "http://localhost:1/nonexistent",
		"client_id":    "my-client-id",
		"redirect_uri": "http://localhost:8080/callback",
		"state":        "state",
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OIDC discovery failed")
}

func TestAuthURL_Descriptor(t *testing.T) {
	d := &authURLDescriptor{}
	assert.Equal(t, "auth_url", d.Name())
	assert.Nil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	require.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "issuer_url")
	assert.Contains(t, props, "client_id")
	assert.Contains(t, props, "redirect_uri")
	assert.Contains(t, props, "state")
	assert.Contains(t, props, "scopes")
	assert.Contains(t, props, "extra_params")
}

func TestAuthURL_Outputs(t *testing.T) {
	executor := newAuthURLExecutor(nil)
	assert.Equal(t, []string{"success", "error"}, executor.Outputs())
}
