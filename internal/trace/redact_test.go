package trace

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chimpanze/noda/pkg/api"
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
	// items is a concretely-typed []string; the reflection-based redactor
	// recurses it element-wise (to catch any nested sensitive maps) and
	// returns a []any of the same (unredacted, since no elements are
	// sensitive) contents rather than preserving the original slice type.
	assert.Equal(t, []any{"a", "b"}, result["items"])
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

func TestRedactSecrets_SessionCookieObjectValueRedacted(t *testing.T) {
	// Mirrors plugins/auth/service.go SessionCookieObject's output shape:
	// a "cookie" key holding a map with "name"/"value" among other fields.
	input := map[string]any{
		"user_id": "u1",
		"cookie": map[string]any{
			"name":      "noda_session",
			"value":     "raw-session-token-should-not-leak",
			"path":      "/",
			"max_age":   float64(3600),
			"secure":    true,
			"http_only": true,
		},
	}

	result := redactSecrets(input)

	cookie := result["cookie"].(map[string]any)
	assert.Equal(t, "[REDACTED]", cookie["value"])
	assert.Equal(t, "noda_session", cookie["name"], "non-sensitive fields must survive")
	assert.Equal(t, "u1", result["user_id"])
}

func TestRedactSecrets_ClearCookieObjectValueRedacted(t *testing.T) {
	// Mirrors plugins/auth/service.go ClearCookieObject's output shape.
	input := map[string]any{
		"revoked_count": float64(1),
		"clear_cookie": map[string]any{
			"name":  "noda_session",
			"value": "",
		},
	}

	result := redactSecrets(input)

	clearCookie := result["clear_cookie"].(map[string]any)
	assert.Equal(t, "[REDACTED]", clearCookie["value"])
}

func TestRedactSecrets_UnrelatedValueKeyNotRedacted(t *testing.T) {
	// A "value" key that isn't nested under a known cookie-container key,
	// or whose containing map isn't cookie-shaped (no "name" key), must not
	// be redacted — the fix is scoped narrowly to avoid over-redaction.
	input := map[string]any{
		"settings": map[string]any{
			"value": "some-config-value",
		},
	}

	result := redactSecrets(input)

	settings := result["settings"].(map[string]any)
	assert.Equal(t, "some-config-value", settings["value"])
}

func TestRedactHTTPResponse_BodyAndCookiesRedacted(t *testing.T) {
	resp := &api.HTTPResponse{
		Status: 200,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer leaked-header-token",
		},
		Cookies: []api.Cookie{
			{Name: "noda_session", Value: "raw-session-token-should-not-leak", Path: "/", HTTPOnly: true},
		},
		Body: map[string]any{
			"token": "raw-body-token-should-not-leak",
			"email": "alice@example.com",
		},
	}

	result := redactHTTPResponse(resp)

	body := result["body"].(map[string]any)
	assert.Equal(t, "[REDACTED]", body["token"])
	assert.Equal(t, "alice@example.com", body["email"])

	cookies := result["cookies"].([]any)
	cookie := cookies[0].(map[string]any)
	assert.Equal(t, "[REDACTED]", cookie["value"])
	assert.Equal(t, "noda_session", cookie["name"])

	headers := result["headers"].(map[string]any)
	assert.Equal(t, "[REDACTED]", headers["Authorization"])
	assert.Equal(t, "application/json", headers["Content-Type"])
}

func TestRedactHTTPResponse_SliceBodyRedacted(t *testing.T) {
	resp := &api.HTTPResponse{
		Status: 200,
		Body: []any{
			map[string]any{"token": "raw-token-should-not-leak", "email": "alice@example.com"},
			[]any{map[string]any{"api_key": "nested-key-should-not-leak"}},
		},
	}

	result := redactHTTPResponse(resp)

	body := result["body"].([]any)
	first := body[0].(map[string]any)
	assert.Equal(t, "[REDACTED]", first["token"])
	assert.Equal(t, "alice@example.com", first["email"])

	nested := body[1].([]any)[0].(map[string]any)
	assert.Equal(t, "[REDACTED]", nested["api_key"])
}

func TestEventHub_HTTPResponseDataRedacted(t *testing.T) {
	hub := NewEventHub()
	received := make(chan []byte, 1)
	unsub := hub.Subscribe(func(data []byte) {
		received <- data
	})
	defer unsub()

	hub.Emit(Event{
		Type: EventNodeCompleted,
		Data: &api.HTTPResponse{
			Status: 200,
			Cookies: []api.Cookie{
				{Name: "noda_session", Value: "raw-session-token-should-not-leak"},
			},
			Body: map[string]any{"token": "raw-body-token-should-not-leak"},
		},
	})

	data := <-received
	assert.NotContains(t, string(data), "raw-session-token-should-not-leak")
	assert.NotContains(t, string(data), "raw-body-token-should-not-leak")
	assert.Contains(t, string(data), "[REDACTED]")
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
		assert.True(t, IsSensitiveKey(k), "expected %q to be sensitive", k)
	}

	safe := []string{
		"name", "email", "host", "port", "path", "count",
		"custom_field", "monkey",
	}
	for _, k := range safe {
		assert.False(t, IsSensitiveKey(k), "expected %q to be safe", k)
	}
}

func TestRedactValue_TypedSliceOfMaps(t *testing.T) {
	in := []map[string]any{
		{"id": 1, "password": "hunter2"},
		{"id": 2, "api_key": "sk-abc"},
	}
	out := redactValue(in).([]any)
	require.Equal(t, "[REDACTED]", out[0].(map[string]any)["password"])
	require.Equal(t, "[REDACTED]", out[1].(map[string]any)["api_key"])
	require.Equal(t, 1, out[0].(map[string]any)["id"])
}

func TestRedactValue_StreamKey(t *testing.T) {
	out := redactValue(map[string]any{"stream_key": "live_xyz", "room": "r1"}).(map[string]any)
	require.Equal(t, "[REDACTED]", out["stream_key"])
	require.Equal(t, "r1", out["room"])
}

func TestEmit_RedactsSliceData(t *testing.T) {
	hub := NewEventHub()
	got := make(chan []byte, 1)
	unsub := hub.Subscribe(func(b []byte) { got <- b })
	defer unsub()
	hub.Emit(Event{Type: "node.completed", Data: []map[string]any{{"password": "p"}}})
	select {
	case b := <-got:
		require.NotContains(t, string(b), "\"p\"")
		require.Contains(t, string(b), "[REDACTED]")
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
}

// Past the recursion cap the redactor must fail CLOSED: returning the raw
// value would leak a deeply nested secret — over-redaction is the safe
// direction for degenerate (>32-deep) payloads (#280).
func TestRedactValue_PastDepthCapScrubbed(t *testing.T) {
	leaf := "raw-secret-material"
	v := any(map[string]any{"leaf": leaf})
	for range maxRedactDepth + 2 {
		v = map[string]any{"nest": v}
	}
	b, err := json.Marshal(redactValue(v))
	require.NoError(t, err)
	assert.NotContains(t, string(b), leaf, "leaf value must not survive past the depth cap")
	assert.Contains(t, string(b), "[REDACTED: max depth]")
}

func TestRedactValue_UnderDepthCapPassesThrough(t *testing.T) {
	b, err := json.Marshal(redactValue(map[string]any{"a": map[string]any{"b": "plain"}}))
	require.NoError(t, err)
	assert.Contains(t, string(b), "plain", "shallow non-sensitive value must pass through")
}

// Non-string-keyed maps can't be classified key-by-key; like the depth cap,
// the redactor must fail closed rather than pass the whole map through raw.
func TestRedactValue_NonStringKeyedMapScrubbed(t *testing.T) {
	out := redactValue(map[int]any{1: map[string]any{"password": "hunter2"}})
	b, err := json.Marshal(out)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "hunter2")
	assert.Contains(t, string(b), "[REDACTED: unclassifiable keys]")
}
