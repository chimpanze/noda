package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// TriggerResult holds the mapped trigger data ready for workflow execution.
type TriggerResult struct {
	Input   map[string]any
	Auth    *api.AuthData
	Trigger api.TriggerData
}

// MapTrigger evaluates trigger.input expressions against raw request data.
func MapTrigger(c fiber.Ctx, triggerConfig map[string]any, compiler *expr.Compiler) (*TriggerResult, error) {
	// Build raw request context for expression evaluation
	rawCtx := buildRawRequestContext(c)

	// Build trigger metadata
	traceID := uuid.New().String()
	if rid := c.Get("X-Request-Id"); rid != "" {
		traceID = rid
	}

	result := &TriggerResult{
		Input: make(map[string]any),
		Trigger: api.TriggerData{
			Type:      "http",
			Timestamp: time.Now().UTC(),
			TraceID:   traceID,
			ClientIP:  c.IP(),
			UserAgent: c.Get("User-Agent"),
		},
	}

	// Handle raw_body preservation
	if rawBody, ok := triggerConfig["raw_body"].(bool); ok && rawBody {
		rawCtx["raw_body"] = string(c.Body())
		// Mirror onto the request.* alias so {{ request.raw_body }} and
		// {{ raw_body }} agree (#275).
		if req, ok := rawCtx["request"].(map[string]any); ok {
			req["raw_body"] = rawCtx["raw_body"]
		}
	}

	// Evaluate input expressions
	inputMap, ok := triggerConfig["input"].(map[string]any)
	if ok {
		// Get file fields (to skip expression resolution)
		fileFields := getFileFields(triggerConfig)

		// Coercion policy (#331): only bare references into string-typed
		// transports are numerically coerced. "coerce": false disables it.
		coerceEnabled := true
		if v, ok := triggerConfig["coerce"].(bool); ok {
			coerceEnabled = v
		}
		bodyStringTyped := strings.Contains(strings.ToLower(c.Get("Content-Type")), "form")

		resolver := expr.NewResolver(compiler, rawCtx)
		for key, exprVal := range inputMap {
			// Skip file fields — pass raw streams
			if fileFields[key] {
				formFile, err := c.FormFile(key)
				if err != nil {
					return nil, fmt.Errorf("trigger mapping: file field %q: %w", key, err)
				}
				result.Input[key] = formFile
				continue
			}

			exprStr, ok := exprVal.(string)
			if !ok {
				result.Input[key] = exprVal
				continue
			}

			resolved, err := resolver.Resolve(exprStr)
			if err != nil {
				return nil, fmt.Errorf("trigger mapping: field %q: %w", key, err)
			}
			if coerceEnabled && shouldCoerce(exprStr, bodyStringTyped) {
				resolved = coerceNumeric(resolved)
			}
			result.Input[key] = resolved
		}
	}

	// Populate auth from JWT middleware
	result.Auth = extractAuth(c)

	return result, nil
}

// buildRawRequestContext creates the expression evaluation context from the raw HTTP request.
// Fields are top-level (body, query, params, headers) matching the architecture docs.
// Auth claims from JWT middleware are also included so {{ auth.sub }} works in trigger mappings.
func buildRawRequestContext(c fiber.Ctx) map[string]any {
	body := parseBody(c)
	params := parseParams(c)
	query := parseQuery(c)
	headers := parseHeaders(c)
	method := c.Method()
	path := c.Path()

	ctx := map[string]any{
		"body": body, "params": params, "query": query,
		"headers": headers, "method": method, "path": path,
	}
	// request.* alias: unifies HTTP route triggers with WebSocket connection
	// channel patterns (where request.* is already valid) and matches the
	// namespace AI agents reach for. Same members as the top-level keys.
	request := map[string]any{
		"body": body, "params": params, "query": query,
		"headers": headers, "method": method, "path": path,
	}
	ctx["request"] = request

	// Include auth claims so trigger mappings can reference {{ auth.sub }}
	if claims, _ := c.Locals(api.LocalJWTClaims).(map[string]any); claims != nil {
		authMap := map[string]any{
			"claims": claims,
		}
		if userID, ok := c.Locals(api.LocalJWTUserID).(string); ok {
			authMap["sub"] = userID
		}
		if roles, ok := c.Locals(api.LocalJWTRoles).([]string); ok {
			authMap["roles"] = roles
		}
		ctx["auth"] = authMap
		request["auth"] = authMap
	}

	return ctx
}

func parseBody(c fiber.Ctx) any {
	contentType := strings.ToLower(c.Get("Content-Type"))
	body := c.Body()
	if len(body) == 0 {
		return nil
	}

	// Try JSON first
	if contentType == "" || strings.Contains(contentType, "json") {
		var parsed any
		if err := json.Unmarshal(body, &parsed); err == nil {
			return parsed
		}
	}

	// Try form data. Content-Type media types are case-insensitive (RFC 7231),
	// so match against the lowercased contentType computed above rather than
	// relying on fasthttp's PostArgs(), which only recognizes the exact
	// lowercase "application/x-www-form-urlencoded" prefix (#331).
	if strings.Contains(contentType, "form") {
		form := make(map[string]any)
		if strings.Contains(contentType, "multipart") {
			mf, err := c.MultipartForm()
			if err == nil && mf != nil {
				for k, v := range mf.Value {
					if len(v) == 1 {
						form[k] = v[0]
					} else {
						form[k] = v
					}
				}
				return form
			}
			// fasthttp's MultipartForm() only recognizes the exact lowercase
			// "multipart/form-data" media type; parse manually for
			// case-varied media types (e.g. "MULTIPART/FORM-DATA") per RFC
			// 7231 (#339). Use the raw (non-lowercased) header so the
			// boundary parameter's case is preserved.
			if mediatype, params, mErr := mime.ParseMediaType(c.Get("Content-Type")); mErr == nil && strings.HasPrefix(mediatype, "multipart/") {
				mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
				mform, mErr := mr.ReadForm(int64(len(body)) + 1)
				if mErr == nil && mform != nil {
					for k, v := range mform.Value {
						if len(v) == 1 {
							form[k] = v[0]
						} else {
							form[k] = v
						}
					}
					return form
				}
			}
		} else if values, err := url.ParseQuery(string(body)); err == nil || len(values) > 0 {
			// url.ParseQuery returns the pairs it did manage to parse alongside
			// an error for bad percent-escapes; use them rather than discarding
			// to the raw-string fallback below -- lenient like the previous
			// fasthttp parser. Note: unlike fasthttp, Go's ParseQuery (since
			// 1.17) rejects semicolon-separated pairs as a deliberate delta.
			for k, v := range values {
				if len(v) == 1 {
					form[k] = v[0]
				} else {
					vals := make([]any, len(v))
					for i, s := range v {
						vals[i] = s
					}
					form[k] = vals
				}
			}
			if len(form) > 0 {
				return form
			}
		}
	}

	// Return raw string as fallback
	return string(body)
}

func parseParams(c fiber.Ctx) map[string]any {
	params := make(map[string]any)
	// Fiber v3: use Route().Params to get param names, then Params() to get values
	for _, param := range c.Route().Params {
		params[param] = c.Params(param)
	}
	return params
}

func parseQuery(c fiber.Ctx) map[string]any {
	query := make(map[string]any)
	for k, v := range c.Queries() {
		query[k] = v
	}
	return query
}

func parseHeaders(c fiber.Ctx) map[string]any {
	headers := make(map[string]any)
	for k, v := range c.GetReqHeaders() {
		if len(v) == 1 {
			headers[k] = v[0]
		} else if len(v) > 1 {
			vals := make([]any, len(v))
			for i, s := range v {
				vals[i] = s
			}
			headers[k] = vals
		}
	}
	return headers
}

// extractAuth reads JWT claims from Fiber locals (set by JWT middleware).
func extractAuth(c fiber.Ctx) *api.AuthData {
	claims, _ := c.Locals(api.LocalJWTClaims).(map[string]any)
	if claims == nil {
		return nil
	}

	auth := &api.AuthData{
		Claims: claims,
	}
	if userID, ok := c.Locals(api.LocalJWTUserID).(string); ok {
		auth.UserID = userID
	}
	if roles, ok := c.Locals(api.LocalJWTRoles).([]string); ok {
		auth.Roles = roles
	}

	return auth
}

// getFileFields returns a set of field names that should be treated as file streams.
func getFileFields(triggerConfig map[string]any) map[string]bool {
	fields := make(map[string]bool)
	if files, ok := triggerConfig["files"].([]any); ok {
		for _, f := range files {
			if name, ok := f.(string); ok {
				fields[name] = true
			}
		}
	}
	return fields
}

// transportRef matches input expressions that are a single bare member-access
// reference into a transport namespace: {{ params.x }}, {{ query.x }},
// {{ headers["X-Y"] }}, {{ body.x }}, and their request.* aliases. Computed
// expressions and literals never match — their result type is authoritative.
var transportRef = regexp.MustCompile(`^\{\{\s*(?:request\.)?(params|query|headers|body)(?:\.[A-Za-z_][A-Za-z0-9_]*|\[[^\]{}]+\])+\s*\}\}$`)

// shouldCoerce reports whether a trigger-input expression's resolved value
// should go through coerceNumeric. params/query/headers always arrive as
// strings; body values are string-typed only for form-encoded requests (#331).
func shouldCoerce(exprStr string, bodyStringTyped bool) bool {
	m := transportRef.FindStringSubmatch(strings.TrimSpace(exprStr))
	if m == nil {
		return false
	}
	if m[1] == "body" {
		return bodyStringTyped
	}
	return true
}

// coerceNumeric attempts to convert string values to numeric types.
// HTTP query parameters and route params are always strings, but downstream
// expressions often need numeric types for arithmetic. Applied only to bare
// transport references — see shouldCoerce.
func coerceNumeric(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return v
}
