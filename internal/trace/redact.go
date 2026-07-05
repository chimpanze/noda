package trace

import (
	"strings"

	"github.com/chimpanze/noda/pkg/api"
)

// sensitiveContains lists substrings (lowercase) that make any key sensitive.
var sensitiveContains = []string{
	"password",
	"secret",
	"token",
	"authorization",
	"credential",
	"api_key",
	"apikey",
}

// sensitiveExact lists exact key names (lowercase) that are sensitive.
var sensitiveExact = []string{
	"key",
}

// cookieObjectKeys lists keys whose value, when a map shaped like a cookie
// object (see plugins/auth/service.go SessionCookieObject/ClearCookieObject),
// carries a raw session token under "value". Rather than redacting every
// "value" key in the codebase (which would be overly broad and could hide
// unrelated, non-sensitive data), we scope the redaction narrowly to the
// known cookie-object shape produced by the auth plugin: a map with both a
// "name" and a "value" key nested under one of these container keys.
var cookieObjectKeys = []string{"cookie", "clear_cookie"}

// redactSecrets returns a deep copy of the map with values redacted for keys
// matching common sensitive patterns. Nested maps are walked recursively.
// Slices and non-map values are left untouched.
func redactSecrets(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if IsSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			out[k] = redactSecrets(val)
			if isCookieContainerKey(k) {
				redactCookieValue(out[k].(map[string]any))
			}
		case []any:
			out[k] = redactSlice(val)
		default:
			out[k] = v
		}
	}
	return out
}

// isCookieContainerKey reports whether key is one of the known cookie
// object container keys ("cookie", "clear_cookie") emitted by the auth
// plugin's SessionCookieObject/ClearCookieObject helpers.
func isCookieContainerKey(key string) bool {
	lower := strings.ToLower(key)
	for _, k := range cookieObjectKeys {
		if lower == k {
			return true
		}
	}
	return false
}

// isCookieShapedMap reports whether m looks like a cookie object: it has
// both a "name" and a "value" key. This is the narrow shape check used to
// decide whether to redact a nested "value" field.
func isCookieShapedMap(m map[string]any) bool {
	_, hasName := m["name"]
	_, hasValue := m["value"]
	return hasName && hasValue
}

// redactCookieValue redacts the "value" field of m in place, if m looks
// like a cookie object (see isCookieShapedMap). This covers the raw session
// token that SessionCookieObject/ClearCookieObject place under "value",
// which would otherwise pass through redactSecrets untouched since "value"
// alone does not match any sensitive key pattern.
func redactCookieValue(m map[string]any) {
	if isCookieShapedMap(m) {
		m["value"] = "[REDACTED]"
	}
}

// redactHTTPResponse converts an *api.HTTPResponse into a redacted
// map[string]any suitable for tracing. response.json emits *api.HTTPResponse
// directly as an Event's Data, bypassing the map[string]any redaction path in
// EventHub.Emit — so without this conversion, login/register responses would
// leak the raw session token via Body (which often embeds the same value as
// the cookie) and via Cookies[].Value. Response cookies are session-bearing
// by nature, so Cookie.Value is always redacted here (unlike the narrower,
// shape-gated redaction used for the auth plugin's cookie/clear_cookie
// config objects in redactSecrets).
func redactHTTPResponse(resp *api.HTTPResponse) map[string]any {
	if resp == nil {
		return nil
	}

	headers := make(map[string]any, len(resp.Headers))
	for k, v := range resp.Headers {
		if IsSensitiveKey(k) {
			headers[k] = "[REDACTED]"
			continue
		}
		headers[k] = v
	}

	cookies := make([]any, 0, len(resp.Cookies))
	for _, c := range resp.Cookies {
		cookies = append(cookies, map[string]any{
			"name":      c.Name,
			"value":     "[REDACTED]",
			"path":      c.Path,
			"domain":    c.Domain,
			"max_age":   c.MaxAge,
			"secure":    c.Secure,
			"http_only": c.HTTPOnly,
			"same_site": c.SameSite,
		})
	}

	out := map[string]any{
		"status":  resp.Status,
		"headers": headers,
		"cookies": cookies,
	}

	switch body := resp.Body.(type) {
	case map[string]any:
		out["body"] = redactSecrets(body)
	default:
		out["body"] = body
	}

	return out
}

// redactSlice returns a copy of the slice with sensitive values in nested maps redacted.
func redactSlice(s []any) []any {
	out := make([]any, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]any:
			out[i] = redactSecrets(val)
		case []any:
			out[i] = redactSlice(val)
		default:
			out[i] = v
		}
	}
	return out
}

// IsSensitiveKey checks whether the key matches any sensitive pattern (case-insensitive).
func IsSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, pattern := range sensitiveContains {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	for _, exact := range sensitiveExact {
		if lower == exact {
			return true
		}
	}
	return false
}
