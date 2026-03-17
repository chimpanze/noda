package oidc

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefresh_ValidToken(t *testing.T) {
	oidcServer, _ := mockOIDCExchangeServer(t)
	defer oidcServer.Close()

	executor := newRefreshExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"issuer_url":    oidcServer.URL,
		"client_id":     "test-client",
		"client_secret": "test-secret",
		"refresh_token": "mock-refresh-token",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, "new-access-token", result["access_token"])
	assert.Equal(t, "new-refresh-token", result["refresh_token"])
	assert.NotEmpty(t, result["id_token"])
	assert.NotNil(t, result["claims"])
}

func TestRefresh_MissingConfig(t *testing.T) {
	executor := newRefreshExecutor(nil)
	execCtx := engine.NewExecutionContext()

	tests := []struct {
		name   string
		config map[string]any
		errMsg string
	}{
		{
			name:   "missing issuer_url",
			config: map[string]any{"client_id": "x", "client_secret": "x", "refresh_token": "x"},
			errMsg: "issuer_url is required",
		},
		{
			name:   "missing client_id",
			config: map[string]any{"issuer_url": "x", "client_secret": "x", "refresh_token": "x"},
			errMsg: "client_id is required",
		},
		{
			name:   "missing client_secret",
			config: map[string]any{"issuer_url": "x", "client_id": "x", "refresh_token": "x"},
			errMsg: "client_secret is required",
		},
		{
			name:   "missing refresh_token",
			config: map[string]any{"issuer_url": "x", "client_id": "x", "client_secret": "x"},
			errMsg: "refresh_token is required",
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

func TestRefresh_Descriptor(t *testing.T) {
	d := &refreshDescriptor{}
	assert.Equal(t, "refresh", d.Name())
	assert.Nil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	require.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "issuer_url")
	assert.Contains(t, props, "client_id")
	assert.Contains(t, props, "client_secret")
	assert.Contains(t, props, "refresh_token")
}

func TestRefresh_Outputs(t *testing.T) {
	executor := newRefreshExecutor(nil)
	assert.Equal(t, []string{"success", "error"}, executor.Outputs())
}
