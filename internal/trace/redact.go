package trace

import (
	"reflect"
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
	"stream_key",
	"signing_key",
	"private_key",
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
// matching common sensitive patterns. Nested maps and slices (of any
// concrete element type) are walked recursively via redactValueDepth.
func redactSecrets(m map[string]any) map[string]any {
	return redactStringMap(m, 0)
}

const maxRedactDepth = 32

// redactValue returns a deep, redacted copy of any value. Maps (string-keyed)
// have sensitive keys replaced and values recursed; slices/arrays of any
// element type are recursed element-wise; scalars pass through. This handles
// concretely-typed values like []map[string]any (db.query results) that a
// plain map[string]any/[]any type switch would miss.
func redactValue(v any) any { return redactValueDepth(v, 0) }

func redactValueDepth(v any, depth int) any {
	if depth > maxRedactDepth {
		return v
	}
	switch val := v.(type) {
	case nil:
		return nil
	case map[string]any:
		return redactStringMap(val, depth)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return v // non-string keys: can't classify; leave as-is
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			k := iter.Key().String()
			if IsSensitiveKey(k) {
				out[k] = "[REDACTED]"
			} else {
				out[k] = redactValueDepth(iter.Value().Interface(), depth+1)
			}
		}
		return out
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return v
		}
		n := rv.Len()
		out := make([]any, n)
		for i := 0; i < n; i++ {
			out[i] = redactValueDepth(rv.Index(i).Interface(), depth+1)
		}
		return out
	default:
		return v
	}
}

// redactStringMap preserves the existing map[string]any behavior, including
// the narrow cookie-container redaction, but recurses values through
// redactValueDepth so nested concretely-typed maps/slices are covered too.
func redactStringMap(m map[string]any, depth int) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if IsSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = redactValueDepth(v, depth+1)
		if isCookieContainerKey(k) {
			if inner, ok := out[k].(map[string]any); ok {
				redactCookieValue(inner)
			}
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

	out["body"] = redactValue(resp.Body)

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
