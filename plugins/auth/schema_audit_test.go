package auth

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
		{"auth.create_session", (&createSessionDescriptor{}).ConfigSchema(),
			map[string]any{"user_id": "{{ auth.sub }}", "ttl": "720h"}, false,
			map[string]any{"user_id": true}},

		{"auth.create_user", (&createUserDescriptor{}).ConfigSchema(),
			map[string]any{
				"email": "{{ input.email }}", "password": "{{ input.password }}",
				"roles": []any{"user"}, "metadata": map[string]any{"source": "signup"},
			}, false,
			map[string]any{"email": "e@example.com", "password": true}},

		{"auth.get_user", (&getUserDescriptor{}).ConfigSchema(),
			map[string]any{"user_id": "{{ auth.sub }}"}, false,
			map[string]any{"user_id": true}},

		{"auth.verify_credentials", (&verifyCredentialsDescriptor{}).ConfigSchema(),
			map[string]any{"email": "{{ input.email }}", "password": "{{ input.password }}"}, false,
			map[string]any{"email": true, "password": "x"}},

		{"auth.revoke_session", (&revokeSessionDescriptor{}).ConfigSchema(),
			map[string]any{"session_id": "{{ input.session_id }}"}, false,
			map[string]any{"session_id": true}},

		{"auth.create_token", (&createTokenDescriptor{}).ConfigSchema(),
			map[string]any{"user_id": "{{ input.user_id }}", "purpose": "reset_password", "ttl": "1h"}, false,
			map[string]any{"user_id": "u1", "purpose": "not-a-purpose"}},

		{"auth.consume_token", (&consumeTokenDescriptor{}).ConfigSchema(),
			map[string]any{"token": "{{ input.token }}", "purpose": "reset_password"}, false,
			map[string]any{"token": "t", "purpose": "not-a-purpose"}},

		{"auth.set_password", (&setPasswordDescriptor{}).ConfigSchema(),
			map[string]any{"user_id": "{{ auth.sub }}", "password": "{{ input.password }}", "revoke_sessions": false}, false,
			map[string]any{"password": "x"}},
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
