package response

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type jsonDescriptor struct{}

func (d *jsonDescriptor) Name() string                           { return "json" }
func (d *jsonDescriptor) Description() string                    { return "Builds an HTTP JSON response" }
func (d *jsonDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *jsonDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status":  map[string]any{"type": "string", "description": "HTTP status code"},
			"body":    map[string]any{"title": "body", "description": "Response body"},
			"headers": map[string]any{"type": "object", "description": "Response headers"},
			"cookies": map[string]any{
				"type":        "array",
				"description": "Response cookies",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":      map[string]any{"type": "string", "description": "Cookie name"},
						"value":     map[string]any{"type": "string", "description": "Cookie value (supports expressions)"},
						"path":      map[string]any{"type": "string", "description": "Cookie path"},
						"domain":    map[string]any{"type": "string", "description": "Cookie domain"},
						"max_age":   map[string]any{"type": "number", "description": "Time to live in seconds"},
						"secure":    map[string]any{"type": "boolean", "description": "HTTPS only"},
						"http_only": map[string]any{"type": "boolean", "description": "No JavaScript access"},
						"same_site": map[string]any{"type": "string", "enum": []any{"Strict", "Lax", "None"}, "description": "SameSite attribute"},
					},
					"required": []any{"name", "value"},
				},
			},
		},
		"required": []any{"status", "body"},
	}
}
func (d *jsonDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "HTTP response set (status + JSON body + optional headers/cookies)",
		"error":   "Expression evaluation error",
	}
}

type jsonExecutor struct{}

func newJSONExecutor(_ map[string]any) api.NodeExecutor {
	return &jsonExecutor{}
}

func (e *jsonExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *jsonExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	// Resolve status (default 200 if absent)
	status := 200
	switch sv := config["status"].(type) {
	case string:
		if sv != "" {
			statusVal, err := nCtx.Resolve(sv)
			if err != nil {
				return "", nil, fmt.Errorf("response.json: status: %w", err)
			}
			if n, ok := plugin.ToInt(statusVal); ok {
				status = n
			}
		}
	default:
		if n, ok := plugin.ToInt(sv); ok {
			status = n
		}
	}

	// Resolve body — handles string expressions, maps, arrays, and scalars
	var body any
	var err error
	switch bv := config["body"].(type) {
	case string:
		body, err = nCtx.Resolve(bv)
		if err != nil {
			return "", nil, fmt.Errorf("response.json: body: %w", err)
		}
	case map[string]any:
		body, err = resolveDeep(nCtx, bv)
		if err != nil {
			return "", nil, fmt.Errorf("response.json: body: %w", err)
		}
	default:
		body = bv
	}

	// Resolve headers
	headers, err := plugin.ResolveHeaders(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("response.json: %w", err)
	}

	// Resolve cookies — supports both inline array (visual editor) and string expression
	var cookies []api.Cookie
	switch cv := config["cookies"].(type) {
	case string:
		if cv != "" {
			cookiesVal, err := nCtx.Resolve(cv)
			if err != nil {
				return "", nil, fmt.Errorf("response.json: cookies: %w", err)
			}
			cookies = toCookies(cookiesVal)
		}
	case []any:
		resolved, err := resolveDeep(nCtx, cv)
		if err != nil {
			return "", nil, fmt.Errorf("response.json: cookies: %w", err)
		}
		cookies = toCookies(resolved)
	}

	resp := &api.HTTPResponse{
		Status:  status,
		Headers: headers,
		Cookies: cookies,
		Body:    body,
	}

	return api.OutputSuccess, resp, nil
}

func toCookies(v any) []api.Cookie {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var cookies []api.Cookie
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c := api.Cookie{
			Name:  strVal(m, "name"),
			Value: strVal(m, "value"),
		}
		if p, ok := m["path"].(string); ok {
			c.Path = p
		}
		if d, ok := m["domain"].(string); ok {
			c.Domain = d
		}
		if ma, ok := m["max_age"].(float64); ok {
			c.MaxAge = int(ma)
		}
		if s, ok := m["secure"].(bool); ok {
			c.Secure = s
		}
		if h, ok := m["http_only"].(bool); ok {
			c.HTTPOnly = h
		}
		if ss, ok := m["same_site"].(string); ok {
			c.SameSite = ss
		}
		cookies = append(cookies, c)
	}
	return cookies
}

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

const maxResolveDepth = 100

// resolveDeep recursively resolves expression strings within a nested map/slice structure.
func resolveDeep(nCtx api.ExecutionContext, v any) (any, error) {
	return resolveDeepWithDepth(nCtx, v, 0)
}

func resolveDeepWithDepth(nCtx api.ExecutionContext, v any, depth int) (any, error) {
	if depth > maxResolveDepth {
		return nil, fmt.Errorf("resolve depth exceeds maximum %d", maxResolveDepth)
	}
	switch val := v.(type) {
	case string:
		return nCtx.Resolve(val)
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, item := range val {
			resolved, err := resolveDeepWithDepth(nCtx, item, depth+1)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", k, err)
			}
			result[k] = resolved
		}
		return result, nil
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			resolved, err := resolveDeepWithDepth(nCtx, item, depth+1)
			if err != nil {
				return nil, err
			}
			result[i] = resolved
		}
		return result, nil
	default:
		return v, nil
	}
}
