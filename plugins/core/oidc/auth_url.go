package oidc

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type authURLDescriptor struct{}

func (d *authURLDescriptor) Name() string { return "auth_url" }
func (d *authURLDescriptor) Description() string {
	return "Builds an OIDC authorization URL for user redirect"
}
func (d *authURLDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *authURLDescriptor) ConfigSchema() map[string]any {
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
			"redirect_uri": map[string]any{
				"type":        "string",
				"description": "Callback URL after authentication (expression)",
			},
			"scopes": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"default":     []any{"openid", "profile", "email"},
				"description": "OAuth2 scopes to request",
			},
			"state": map[string]any{
				"type":        "string",
				"description": "State parameter for CSRF protection (expression)",
			},
			"extra_params": map[string]any{
				"type":        "object",
				"description": "Additional query parameters to include in the authorization URL",
			},
		},
		"required": []any{"issuer_url", "client_id", "redirect_uri", "state"},
	}
}
func (d *authURLDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Object with url and state fields",
		"error":   "Error building authorization URL",
	}
}

type authURLExecutor struct{}

func newAuthURLExecutor(_ map[string]any) api.NodeExecutor {
	return &authURLExecutor{}
}

func (e *authURLExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *authURLExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	// Resolve issuer URL
	issuerExpr, _ := config["issuer_url"].(string)
	issuerVal, err := nCtx.Resolve(issuerExpr)
	if err != nil {
		return "", nil, fmt.Errorf("oidc.auth_url: issuer_url: %w", err)
	}
	issuerURL, ok := issuerVal.(string)
	if !ok {
		return "", nil, fmt.Errorf("oidc.auth_url: issuer_url must resolve to a string")
	}

	// Resolve client ID
	clientIDExpr, _ := config["client_id"].(string)
	clientIDVal, err := nCtx.Resolve(clientIDExpr)
	if err != nil {
		return "", nil, fmt.Errorf("oidc.auth_url: client_id: %w", err)
	}
	clientID, ok := clientIDVal.(string)
	if !ok {
		return "", nil, fmt.Errorf("oidc.auth_url: client_id must resolve to a string")
	}

	// Resolve redirect URI
	redirectExpr, _ := config["redirect_uri"].(string)
	redirectVal, err := nCtx.Resolve(redirectExpr)
	if err != nil {
		return "", nil, fmt.Errorf("oidc.auth_url: redirect_uri: %w", err)
	}
	redirectURI, ok := redirectVal.(string)
	if !ok {
		return "", nil, fmt.Errorf("oidc.auth_url: redirect_uri must resolve to a string")
	}

	// Resolve state
	stateExpr, _ := config["state"].(string)
	stateVal, err := nCtx.Resolve(stateExpr)
	if err != nil {
		return "", nil, fmt.Errorf("oidc.auth_url: state: %w", err)
	}
	state, ok := stateVal.(string)
	if !ok {
		return "", nil, fmt.Errorf("oidc.auth_url: state must resolve to a string")
	}

	// Resolve scopes
	scopes := []string{"openid", "profile", "email"}
	if scopesCfg, ok := config["scopes"].([]any); ok {
		scopes = make([]string, 0, len(scopesCfg))
		for _, s := range scopesCfg {
			if str, ok := s.(string); ok {
				scopes = append(scopes, str)
			}
		}
	}

	// Perform OIDC discovery
	provider, err := gooidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return "", nil, fmt.Errorf("oidc.auth_url: OIDC discovery failed: %w", err)
	}

	// Build OAuth2 config
	oauth2Config := &oauth2.Config{
		ClientID:    clientID,
		RedirectURL: redirectURI,
		Endpoint:    provider.Endpoint(),
		Scopes:      scopes,
	}

	// Build authorization URL with optional extra params
	var opts []oauth2.AuthCodeOption
	if extraParams, ok := config["extra_params"].(map[string]any); ok {
		for k, v := range extraParams {
			if str, ok := v.(string); ok {
				resolved, err := nCtx.Resolve(str)
				if err != nil {
					return "", nil, fmt.Errorf("oidc.auth_url: extra_params.%s: %w", k, err)
				}
				if s, ok := resolved.(string); ok {
					opts = append(opts, oauth2.SetAuthURLParam(k, s))
				}
			}
		}
	}

	url := oauth2Config.AuthCodeURL(state, opts...)

	return api.OutputSuccess, map[string]any{
		"url":   url,
		"state": state,
	}, nil
}
