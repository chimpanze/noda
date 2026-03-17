package oidc

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type refreshDescriptor struct{}

func (d *refreshDescriptor) Name() string { return "refresh" }
func (d *refreshDescriptor) Description() string {
	return "Refreshes OIDC tokens using a refresh token"
}
func (d *refreshDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *refreshDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"issuer_url": map[string]any{
				"type":        "string",
				"description": "OIDC provider issuer URL (expression)",
			},
			"client_id": map[string]any{
				"type":        "string",
				"description": "OAuth2 client ID (expression)",
			},
			"client_secret": map[string]any{
				"type":        "string",
				"description": "OAuth2 client secret (expression)",
			},
			"refresh_token": map[string]any{
				"type":        "string",
				"description": "Refresh token to use (expression)",
			},
		},
		"required": []any{"issuer_url", "client_id", "client_secret", "refresh_token"},
	}
}
func (d *refreshDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Object with new access_token, refresh_token, id_token, and expires_at",
		"error":   "Token refresh error",
	}
}

type refreshExecutor struct{}

func newRefreshExecutor(_ map[string]any) api.NodeExecutor {
	return &refreshExecutor{}
}

func (e *refreshExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *refreshExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	issuerURL, err := resolveString(nCtx, config, "issuer_url", "oidc.refresh")
	if err != nil {
		return "", nil, err
	}
	clientID, err := resolveString(nCtx, config, "client_id", "oidc.refresh")
	if err != nil {
		return "", nil, err
	}
	clientSecret, err := resolveString(nCtx, config, "client_secret", "oidc.refresh")
	if err != nil {
		return "", nil, err
	}
	refreshToken, err := resolveString(nCtx, config, "refresh_token", "oidc.refresh")
	if err != nil {
		return "", nil, err
	}

	// Perform OIDC discovery
	provider, err := gooidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return "", nil, fmt.Errorf("oidc.refresh: OIDC discovery failed: %w", err)
	}

	// Build OAuth2 config
	oauth2Config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
	}

	// Create a token source from the refresh token
	tokenSource := oauth2Config.TokenSource(ctx, &oauth2.Token{
		RefreshToken: refreshToken,
	})

	// Get new token
	newToken, err := tokenSource.Token()
	if err != nil {
		return "", nil, fmt.Errorf("oidc.refresh: token refresh failed: %w", err)
	}

	result := map[string]any{
		"access_token": newToken.AccessToken,
	}

	if newToken.RefreshToken != "" {
		result["refresh_token"] = newToken.RefreshToken
	}

	if rawIDToken, ok := newToken.Extra("id_token").(string); ok && rawIDToken != "" {
		result["id_token"] = rawIDToken

		// Verify and extract claims from the new ID token
		verifier := provider.Verifier(&gooidc.Config{ClientID: clientID})
		idToken, err := verifier.Verify(ctx, rawIDToken)
		if err == nil {
			var claims map[string]any
			if err := idToken.Claims(&claims); err == nil {
				result["claims"] = claims
			}
		}
	}

	if !newToken.Expiry.IsZero() {
		result["expires_at"] = newToken.Expiry.Unix()
	}

	return api.OutputSuccess, result, nil
}
