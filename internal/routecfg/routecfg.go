// Package routecfg holds pure helpers over route and middleware config maps,
// shared by the HTTP server (OpenAPI generation, middleware setup) and the
// editor API (codegen, middleware listing).
package routecfg

// NormalizeRoutes returns the route objects in a route file, which can be a
// single route object or a group file with routes under arbitrary keys.
func NormalizeRoutes(data map[string]any) []map[string]any {
	if _, hasMethod := data["method"]; hasMethod {
		return []map[string]any{data}
	}
	var routes []map[string]any
	for _, v := range data {
		if rm, ok := v.(map[string]any); ok {
			if _, hasMethod := rm["method"]; hasMethod {
				routes = append(routes, rm)
			}
		}
	}
	return routes
}

// middlewareConfigPaths maps middleware names to alternative config lookup paths.
// Each path is a sequence of nested keys in the root config.
// The "middleware" section is always checked first for all middleware.
var middlewareConfigPaths = map[string][]string{
	"security.cors":    {"security", "cors"},
	"security.headers": {"security", "headers"},
	"security.csrf":    {"security", "csrf"},
	"auth.jwt":         {"security", "jwt"},
	"auth.oidc":        {"security", "oidc"},
	"auth.session":     {"security", "session"},
	"casbin.enforce":   {"security", "casbin"},
	"livekit.webhook":  {"security", "livekit"},
}

// ExtractMiddlewareConfig extracts the config block for a specific middleware.
func ExtractMiddlewareConfig(name string, rootConfig map[string]any) map[string]any {
	if mw, ok := rootConfig["middleware"].(map[string]any); ok {
		if cfg, ok := mw[name].(map[string]any); ok {
			return cfg
		}
	}
	if path, ok := middlewareConfigPaths[name]; ok {
		cfg := rootConfig
		for _, key := range path {
			next, ok := cfg[key].(map[string]any)
			if !ok {
				return nil
			}
			cfg = next
		}
		return cfg
	}
	return nil
}
