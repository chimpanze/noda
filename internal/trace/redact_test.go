package trace

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactSecrets_BasicKeys(t *testing.T) {
	input := map[string]any{
		"username": "alice",
		"password": "s3cret",
		"token":    "abc123",
		"api_key":  "key-xyz",
	}

	result := redactSecrets(input)

	assert.Equal(t, "alice", result["username"])
	assert.Equal(t, "[REDACTED]", result["password"])
	assert.Equal(t, "[REDACTED]", result["token"])
	assert.Equal(t, "[REDACTED]", result["api_key"])
}

func TestRedactSecrets_CaseInsensitive(t *testing.T) {
	input := map[string]any{
		"Password":      "abc",
		"SECRET":        "xyz",
		"Authorization": "Bearer tok",
		"ApiKey":        "k",
	}

	result := redactSecrets(input)

	for k := range input {
		assert.Equal(t, "[REDACTED]", result[k], "expected key %q to be redacted", k)
	}
}

func TestRedactSecrets_NestedMaps(t *testing.T) {
	input := map[string]any{
		"config": map[string]any{
			"db_password": "hunter2",
			"host":        "localhost",
			"nested": map[string]any{
				"api_key": "nested-key",
				"name":    "test",
			},
		},
		"name": "safe",
	}

	result := redactSecrets(input)

	assert.Equal(t, "safe", result["name"])

	config := result["config"].(map[string]any)
	assert.Equal(t, "[REDACTED]", config["db_password"])
	assert.Equal(t, "localhost", config["host"])

	nested := config["nested"].(map[string]any)
	assert.Equal(t, "[REDACTED]", nested["api_key"])
	assert.Equal(t, "test", nested["name"])
}

func TestRedactSecrets_CredentialKeyRedacted(t *testing.T) {
	// Keys containing "credential" are sensitive, so their values are redacted
	input := map[string]any{
		"credentials": map[string]any{"user": "alice"},
	}
	result := redactSecrets(input)
	assert.Equal(t, "[REDACTED]", result["credentials"])
}

func TestRedactSecrets_PreservesNonMapValues(t *testing.T) {
	input := map[string]any{
		"count":  42,
		"items":  []string{"a", "b"},
		"active": true,
		"token":  "redact-me",
	}

	result := redactSecrets(input)

	assert.Equal(t, 42, result["count"])
	assert.Equal(t, []string{"a", "b"}, result["items"])
	assert.Equal(t, true, result["active"])
	assert.Equal(t, "[REDACTED]", result["token"])
}

func TestRedactSecrets_EmptyMap(t *testing.T) {
	result := redactSecrets(map[string]any{})
	assert.Empty(t, result)
}

func TestRedactSecrets_DoesNotModifyOriginal(t *testing.T) {
	input := map[string]any{
		"password": "original",
		"name":     "test",
	}

	_ = redactSecrets(input)

	assert.Equal(t, "original", input["password"])
}

func TestIsSensitiveKey(t *testing.T) {
	sensitive := []string{
		"password", "Password", "db_password",
		"secret", "client_secret",
		"token", "access_token", "refresh_token",
		"key", "api_key",
		"authorization", "Authorization",
		"credential", "credentials",
		"apikey", "APIKEY",
	}
	for _, k := range sensitive {
		assert.True(t, isSensitiveKey(k), "expected %q to be sensitive", k)
	}

	safe := []string{
		"name", "email", "host", "port", "path", "count",
		"custom_field", "monkey",
	}
	for _, k := range safe {
		assert.False(t, isSensitiveKey(k), "expected %q to be safe", k)
	}
}
