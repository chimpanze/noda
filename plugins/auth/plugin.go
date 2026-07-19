package auth

import (
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

// Plugin implements first-party authentication primitives.
type Plugin struct{}

func (p *Plugin) Name() string      { return "auth" }
func (p *Plugin) Prefix() string    { return "auth" }
func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &createUserDescriptor{}, Factory: newCreateUserExecutor},
		{Descriptor: &getUserDescriptor{}, Factory: newGetUserExecutor},
		{Descriptor: &verifyCredentialsDescriptor{}, Factory: newVerifyCredentialsExecutor},
		{Descriptor: &createSessionDescriptor{}, Factory: newCreateSessionExecutor},
		{Descriptor: &revokeSessionDescriptor{}, Factory: newRevokeSessionExecutor},
		{Descriptor: &createTokenDescriptor{}, Factory: newCreateTokenExecutor},
		{Descriptor: &consumeTokenDescriptor{}, Factory: newConsumeTokenExecutor},
		{Descriptor: &setPasswordDescriptor{}, Factory: newSetPasswordExecutor},
	}
}

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	return newService(config)
}

// ServiceConfigSchema documents the auth service `config` block. Every key
// here is read by newService (plugins/auth/service.go) — additionalProperties
// is false at each level because newService silently ignores unknown keys
// (a typo'd key would otherwise fail closed with no error).
func (p *Plugin) ServiceConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"database": map[string]any{
				"type":        "string",
				"description": "Name of the db service (services.*) the auth plugin stores its users/sessions/tokens tables in",
			},
			"session": map[string]any{
				"type":        "object",
				"description": "Session cookie/TTL configuration",
				"properties": map[string]any{
					"ttl": map[string]any{
						"type":        "string",
						"description": "Session lifetime as a Go duration (default 720h)",
					},
					"cookie": map[string]any{
						"type":        "object",
						"description": "Session cookie attributes",
						"properties": map[string]any{
							"name":      map[string]any{"type": "string", "description": "Cookie name (default noda_session)"},
							"path":      map[string]any{"type": "string", "description": "Cookie path (default /)"},
							"domain":    map[string]any{"type": "string", "description": "Cookie domain (default empty = host-only)"},
							"same_site": map[string]any{"type": "string", "enum": []any{"Lax", "Strict", "None"}, "description": "SameSite attribute (default Lax)"},
							"secure":    map[string]any{"type": "boolean", "description": "Secure attribute (default true)"},
							"http_only": map[string]any{"type": "boolean", "description": "HttpOnly attribute (default true)"},
						},
						"additionalProperties": false,
					},
				},
				"additionalProperties": false,
			},
			"argon2": map[string]any{
				"type":        "object",
				"description": "Argon2id password hashing parameters; unset fields keep the library default",
				"properties": map[string]any{
					"memory_kib":  map[string]any{"type": "integer", "description": "Memory cost in KiB"},
					"iterations":  map[string]any{"type": "integer", "description": "Number of iterations"},
					"salt_len":    map[string]any{"type": "integer", "description": "Salt length in bytes"},
					"key_len":     map[string]any{"type": "integer", "description": "Derived key length in bytes"},
					"parallelism": map[string]any{"type": "integer", "description": "Degree of parallelism"},
				},
				"additionalProperties": false,
			},
			"tokens": map[string]any{
				"type":        "object",
				"description": "TTLs for single-use auth tokens, as Go durations",
				"properties": map[string]any{
					"verify_email_ttl":   map[string]any{"type": "string", "description": "Email verification token TTL (default 24h)"},
					"reset_password_ttl": map[string]any{"type": "string", "description": "Password reset token TTL (default 1h)"},
				},
				"additionalProperties": false,
			},
		},
		"required":             []any{"database"},
		"additionalProperties": false,
	}
}

func (p *Plugin) HealthCheck(service any) error {
	if _, ok := service.(*Service); !ok {
		return fmt.Errorf("auth: invalid service type")
	}
	return nil
}

func (p *Plugin) Shutdown(any) error { return nil }
