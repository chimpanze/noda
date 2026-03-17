package oidc

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type exchangeDescriptor struct{}

func (d *exchangeDescriptor) Name() string { return "exchange" }
func (d *exchangeDescriptor) Description() string {
	return "Exchanges an authorization code for OIDC tokens"
}
func (d *exchangeDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *exchangeDescriptor) ConfigSchema() map[string]any {
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
			"redirect_uri": map[string]any{
				"type":        "string",
				"description": "Callback URL used during authorization (expression)",
			},
			"code": map[string]any{
				"type":        "string",
				"description": "Authorization code to exchange (expression, typically {{ query.code }})",
			},
		},
		"required": []any{"issuer_url", "client_id", "client_secret", "redirect_uri", "code"},
	}
}
func (d *exchangeDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Object with id_token, access_token, refresh_token, claims, and expires_at",
		"error":   "Token exchange error",
	}
}

type exchangeExecutor struct{}

func newExchangeExecutor(_ map[string]any) api.NodeExecutor {
	return &exchangeExecutor{}
}

func (e *exchangeExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *exchangeExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	// Resolve config values
	issuerURL, err := resolveString(nCtx, config, "issuer_url", "oidc.exchange")
	if err != nil {
		return "", nil, err
	}
	clientID, err := resolveString(nCtx, config, "client_id", "oidc.exchange")
	if err != nil {
		return "", nil, err
	}
	clientSecret, err := resolveString(nCtx, config, "client_secret", "oidc.exchange")
	if err != nil {
		return "", nil, err
	}
	redirectURI, err := resolveString(nCtx, config, "redirect_uri", "oidc.exchange")
	if err != nil {
		return "", nil, err
	}
	code, err := resolveString(nCtx, config, "code", "oidc.exchange")
	if err != nil {
		return "", nil, err
	}

	// Perform OIDC discovery
	provider, err := gooidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return "", nil, fmt.Errorf("oidc.exchange: OIDC discovery failed: %w", err)
	}

	// Build OAuth2 config
	oauth2Config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Endpoint:     provider.Endpoint(),
	}

	// Exchange the authorization code for tokens
	token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		return "", nil, fmt.Errorf("oidc.exchange: token exchange failed: %w", err)
	}

	// Extract the ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", nil, fmt.Errorf("oidc.exchange: no id_token in token response")
	}

	// Verify the ID token
	verifier := provider.Verifier(&gooidc.Config{ClientID: clientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", nil, fmt.Errorf("oidc.exchange: ID token verification failed: %w", err)
	}

	// Extract claims
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return "", nil, fmt.Errorf("oidc.exchange: claims extraction failed: %w", err)
	}

	result := map[string]any{
		"id_token":     rawIDToken,
		"access_token": token.AccessToken,
		"claims":       claims,
	}

	if token.RefreshToken != "" {
		result["refresh_token"] = token.RefreshToken
	}

	if !token.Expiry.IsZero() {
		result["expires_at"] = token.Expiry.Unix()
	}

	return api.OutputSuccess, result, nil
}

// resolveString resolves a config expression to a string value.
func resolveString(nCtx api.ExecutionContext, config map[string]any, key, prefix string) (string, error) {
	expr, _ := config[key].(string)
	if expr == "" {
		return "", fmt.Errorf("%s: %s is required", prefix, key)
	}
	val, err := nCtx.Resolve(expr)
	if err != nil {
		return "", fmt.Errorf("%s: %s: %w", prefix, key, err)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("%s: %s must resolve to a string, got %T", prefix, key, val)
	}
	return str, nil
}
