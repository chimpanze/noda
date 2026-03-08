package response

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type jsonDescriptor struct{}

func (d *jsonDescriptor) Name() string                           { return "json" }
func (d *jsonDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *jsonDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status":  map[string]any{"type": "string"},
			"body":    map[string]any{"type": "string"},
			"headers": map[string]any{"type": "object"},
			"cookies": map[string]any{"type": "string"},
		},
		"required": []any{"status", "body"},
	}
}

type jsonExecutor struct{}

func newJSONExecutor(_ map[string]any) api.NodeExecutor {
	return &jsonExecutor{}
}

func (e *jsonExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *jsonExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	// Resolve status
	statusExpr, _ := config["status"].(string)
	if statusExpr == "" {
		statusExpr = "200"
	}
	statusVal, err := nCtx.Resolve(statusExpr)
	if err != nil {
		return "", nil, fmt.Errorf("response.json: status: %w", err)
	}
	status := toInt(statusVal)
	if status == 0 {
		status = 200
	}

	// Resolve body
	bodyExpr, _ := config["body"].(string)
	body, err := nCtx.Resolve(bodyExpr)
	if err != nil {
		return "", nil, fmt.Errorf("response.json: body: %w", err)
	}

	// Resolve headers
	var headers map[string]string
	if headersRaw, ok := config["headers"].(map[string]any); ok {
		headers = make(map[string]string, len(headersRaw))
		for k, v := range headersRaw {
			exprStr, ok := v.(string)
			if !ok {
				headers[k] = fmt.Sprintf("%v", v)
				continue
			}
			resolved, err := nCtx.Resolve(exprStr)
			if err != nil {
				return "", nil, fmt.Errorf("response.json: header %q: %w", k, err)
			}
			headers[k] = fmt.Sprintf("%v", resolved)
		}
	}

	// Resolve cookies
	var cookies []api.Cookie
	if cookiesExpr, ok := config["cookies"].(string); ok && cookiesExpr != "" {
		cookiesVal, err := nCtx.Resolve(cookiesExpr)
		if err != nil {
			return "", nil, fmt.Errorf("response.json: cookies: %w", err)
		}
		cookies = toCookies(cookiesVal)
	}

	resp := &api.HTTPResponse{
		Status:  status,
		Headers: headers,
		Cookies: cookies,
		Body:    body,
	}

	return "success", resp, nil
}

func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		// Handle static integer strings
		var n int
		if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
			return n
		}
		return 0
	default:
		return 0
	}
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
