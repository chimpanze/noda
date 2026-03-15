package server

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// ResolveMiddlewareChain resolves the full middleware chain for a route.
// Order: group → route-specific. Global middleware is applied via app.Use().
func (s *Server) ResolveMiddlewareChain(route map[string]any) ([]fiber.Handler, error) {
	var middlewareNames []string

	// 1. Group middleware (based on route path matching route_groups)
	routePath, _ := route["path"].(string)
	groupMW, err := s.getGroupMiddleware(routePath)
	if err != nil {
		return nil, err
	}
	middlewareNames = append(middlewareNames, groupMW...)

	// 2. Route-level middleware
	if preset, ok := route["middleware_preset"].(string); ok && preset != "" {
		expanded, err := s.expandPreset(preset)
		if err != nil {
			return nil, fmt.Errorf("route %v: %w", route["id"], err)
		}
		middlewareNames = append(middlewareNames, expanded...)
	}
	if routeMW, ok := route["middleware"].([]any); ok {
		for _, mw := range routeMW {
			if name, ok := mw.(string); ok {
				middlewareNames = append(middlewareNames, name)
			}
		}
	}

	// Deduplicate while preserving order
	middlewareNames = dedupe(middlewareNames)

	// Validate ordering constraints (e.g., auth.jwt must precede casbin.enforce)
	if err := ValidateMiddlewareOrder(middlewareNames); err != nil {
		return nil, err
	}

	// Build handlers
	handlers := make([]fiber.Handler, 0, len(middlewareNames))
	for _, name := range middlewareNames {
		h, err := BuildMiddleware(name, s.config.Root)
		if err != nil {
			return nil, fmt.Errorf("middleware %q: %w", name, err)
		}
		handlers = append(handlers, h)
	}

	return handlers, nil
}

// ValidatePresets checks that all preset names referenced in routes and groups exist.
func (s *Server) ValidatePresets() []error {
	presets := s.getPresets()
	var errs []error

	// Check route-level presets
	for id, route := range s.config.Routes {
		if preset, ok := route["middleware_preset"].(string); ok && preset != "" {
			if _, exists := presets[preset]; !exists {
				errs = append(errs, fmt.Errorf("route %q: unknown middleware preset %q", id, preset))
			}
		}
	}

	// Check group-level presets
	groups := s.getRouteGroups()
	for prefix, group := range groups {
		if preset, ok := group["middleware_preset"].(string); ok && preset != "" {
			if _, exists := presets[preset]; !exists {
				errs = append(errs, fmt.Errorf("route group %q: unknown middleware preset %q", prefix, preset))
			}
		}
	}

	return errs
}

func (s *Server) getGlobalMiddleware() []string {
	if mw, ok := s.config.Root["global_middleware"].([]any); ok {
		result := make([]string, 0, len(mw))
		for _, v := range mw {
			if name, ok := v.(string); ok {
				result = append(result, name)
			}
		}
		return result
	}
	return nil
}

func (s *Server) getGroupMiddleware(routePath string) ([]string, error) {
	groups := s.getRouteGroups()
	for prefix, group := range groups {
		if strings.HasPrefix(routePath, prefix) {
			// Check for middleware_preset on group
			if preset, ok := group["middleware_preset"].(string); ok && preset != "" {
				return s.expandPreset(preset)
			}
			// Check for direct middleware list
			if mw, ok := group["middleware"].([]any); ok {
				result := make([]string, 0, len(mw))
				for _, v := range mw {
					if name, ok := v.(string); ok {
						result = append(result, name)
					}
				}
				return result, nil
			}
		}
	}
	return nil, nil
}

func (s *Server) getRouteGroups() map[string]map[string]any {
	groups := make(map[string]map[string]any)
	if rg, ok := s.config.Root["route_groups"].(map[string]any); ok {
		for prefix, v := range rg {
			if group, ok := v.(map[string]any); ok {
				groups[prefix] = group
			}
		}
	}
	return groups
}

func (s *Server) getPresets() map[string][]string {
	presets := make(map[string][]string)
	if mp, ok := s.config.Root["middleware_presets"].(map[string]any); ok {
		for name, v := range mp {
			if arr, ok := v.([]any); ok {
				mws := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						mws = append(mws, s)
					}
				}
				presets[name] = mws
			}
		}
	}
	return presets
}

func (s *Server) expandPreset(name string) ([]string, error) {
	presets := s.getPresets()
	mws, ok := presets[name]
	if !ok {
		return nil, fmt.Errorf("unknown middleware preset %q", name)
	}
	return mws, nil
}

func dedupe(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// middlewareOrderRules defines ordering constraints: the key middleware
// must appear after all listed prerequisites in the chain.
var middlewareOrderRules = map[string][]string{
	"casbin.enforce": {"auth.jwt"},
}

// ValidateMiddlewareOrder checks that middleware ordering constraints are satisfied.
// Instance names (e.g. "auth.jwt:v1") are resolved to their base type for ordering checks.
func ValidateMiddlewareOrder(chain []string) error {
	// Track first and last positions of each base type
	firstPos := make(map[string]int, len(chain))
	lastPos := make(map[string]int, len(chain))
	for i, name := range chain {
		base, _ := ParseMiddlewareName(name)
		if _, exists := firstPos[base]; !exists {
			firstPos[base] = i
		}
		lastPos[base] = i
	}

	for mw, deps := range middlewareOrderRules {
		mwFirst, hasMW := firstPos[mw]
		if !hasMW {
			continue
		}
		for _, dep := range deps {
			depLast, hasDep := lastPos[dep]
			if !hasDep {
				continue
			}
			if depLast > mwFirst {
				return fmt.Errorf("middleware %q must appear before %q in the chain", dep, mw)
			}
		}
	}
	return nil
}
