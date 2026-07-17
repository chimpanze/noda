package oidc

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
)

func TestConfigSchemasMatchExecutors(t *testing.T) {
	tests := []struct {
		nodeType     string
		schema       map[string]any
		minimalValid map[string]any // smallest config the executor accepts (from docs example)
		emptyValid   bool           // does the executor run with config {}?
		invalid      map[string]any // one config the executor would reject/misuse
	}{
		{"oidc.auth_url", (&authURLDescriptor{}).ConfigSchema(),
			map[string]any{
				"issuer_url":   "{{ secrets.OIDC_ISSUER_URL }}",
				"client_id":    "{{ secrets.OIDC_CLIENT_ID }}",
				"redirect_uri": "http://localhost:3000/auth/callback",
				"state":        "{{ $uuid() }}",
			}, false,
			map[string]any{
				"issuer_url":   "{{ secrets.OIDC_ISSUER_URL }}",
				"client_id":    "{{ secrets.OIDC_CLIENT_ID }}",
				"redirect_uri": "http://localhost:3000/auth/callback",
				"state":        "{{ $uuid() }}",
				"scopes":       "not-an-array",
			}},
		{"oidc.exchange", (&exchangeDescriptor{}).ConfigSchema(),
			map[string]any{
				"issuer_url":    "{{ secrets.OIDC_ISSUER_URL }}",
				"client_id":     "{{ secrets.OIDC_CLIENT_ID }}",
				"client_secret": "{{ secrets.OIDC_CLIENT_SECRET }}",
				"redirect_uri":  "http://localhost:3000/auth/callback",
				"code":          "{{ input.code }}",
			}, false,
			map[string]any{
				"issuer_url":    "{{ secrets.OIDC_ISSUER_URL }}",
				"client_id":     "{{ secrets.OIDC_CLIENT_ID }}",
				"client_secret": "{{ secrets.OIDC_CLIENT_SECRET }}",
				"redirect_uri":  "http://localhost:3000/auth/callback",
				"code":          true,
			}},
		{"oidc.refresh", (&refreshDescriptor{}).ConfigSchema(),
			map[string]any{
				"issuer_url":    "{{ secrets.OIDC_ISSUER_URL }}",
				"client_id":     "{{ secrets.OIDC_CLIENT_ID }}",
				"client_secret": "{{ secrets.OIDC_CLIENT_SECRET }}",
				"refresh_token": "{{ input.refresh_token }}",
			}, false,
			map[string]any{
				"issuer_url":    "{{ secrets.OIDC_ISSUER_URL }}",
				"client_id":     "{{ secrets.OIDC_CLIENT_ID }}",
				"client_secret": "{{ secrets.OIDC_CLIENT_SECRET }}",
				"refresh_token": 42,
			}},
	}
	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			assert.Empty(t, registry.CheckSchemaVocabulary(tt.schema))
			assert.Empty(t, registry.ValidateNodeConfig(tt.schema, tt.minimalValid), "minimal valid config must pass")
			emptyErrs := registry.ValidateNodeConfig(tt.schema, map[string]any{})
			if tt.emptyValid {
				assert.Empty(t, emptyErrs, "executor accepts {}, schema must too")
			} else {
				assert.NotEmpty(t, emptyErrs, "executor rejects {}, schema must too")
			}
			assert.NotEmpty(t, registry.ValidateNodeConfig(tt.schema, tt.invalid))
		})
	}
}
