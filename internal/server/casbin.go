package server

import (
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	"github.com/gofiber/fiber/v3"
)

// newCasbinMiddleware creates a Casbin enforcement middleware.
// Config comes from security.casbin in root config:
//
//	{
//	  "model": "<inline model text or file path>",
//	  "policies": [["p", "admin", "/api/*", "GET"], ...],
//	  "tenant_param": "workspace_id"  // optional, for multi-tenant RBAC
//	}
func newCasbinMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("casbin.enforce: security.casbin config is required")
	}

	enforcer, err := buildEnforcer(cfg)
	if err != nil {
		return nil, fmt.Errorf("casbin.enforce: %w", err)
	}

	tenantParam, _ := cfg["tenant_param"].(string)

	return func(c fiber.Ctx) error {
		sub := extractSubject(c)
		if sub == "" {
			return fiber.NewError(fiber.StatusForbidden, "access denied")
		}

		obj := c.Path()
		act := c.Method()

		var allowed bool
		if tenantParam != "" {
			tenant := c.Params(tenantParam)
			if tenant == "" {
				// Try query param as fallback
				tenant = c.Query(tenantParam)
			}
			if tenant == "" {
				return fiber.NewError(fiber.StatusForbidden, "missing required tenant parameter")
			}
			allowed, err = enforcer.Enforce(sub, tenant, obj, act)
		} else {
			allowed, err = enforcer.Enforce(sub, obj, act)
		}
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "authorization error")
		}

		if !allowed {
			return fiber.NewError(fiber.StatusForbidden, "access denied")
		}

		return c.Next()
	}, nil
}

// extractSubject gets the user identity from JWT locals (set by auth.jwt middleware).
func extractSubject(c fiber.Ctx) string {
	if uid, ok := c.Locals(LocalJWTUserID).(string); ok && uid != "" {
		return uid
	}
	return ""
}

// buildEnforcer creates a Casbin enforcer from config.
func buildEnforcer(cfg map[string]any) (*casbin.Enforcer, error) {
	modelText, _ := cfg["model"].(string)
	if modelText == "" {
		return nil, fmt.Errorf("model is required")
	}

	// Load model — if it contains [request_definition] it's inline text, otherwise a file path
	var m casbinmodel.Model
	var err error
	if strings.Contains(modelText, "[request_definition]") {
		m, err = casbinmodel.NewModelFromString(modelText)
	} else {
		m, err = casbinmodel.NewModelFromFile(modelText)
	}
	if err != nil {
		return nil, fmt.Errorf("load model: %w", err)
	}

	// Create enforcer with model only (no adapter — we'll add policies programmatically)
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, fmt.Errorf("create enforcer: %w", err)
	}

	// Load policies from config
	if err := loadPolicies(e, cfg); err != nil {
		return nil, fmt.Errorf("load policies: %w", err)
	}

	return e, nil
}

// loadPolicies adds policies from config to the enforcer.
// Supports:
//   - "policies": [["p", "admin", "/api/*", "GET"], ...]  (policy rules)
//   - "role_links": [["g", "alice", "admin"], ...]          (role assignments)
func loadPolicies(e *casbin.Enforcer, cfg map[string]any) error {
	// Load policy rules
	if policies, ok := cfg["policies"].([]any); ok {
		for i, pRaw := range policies {
			rule, err := toStringSlice(pRaw)
			if err != nil {
				return fmt.Errorf("policy %d: %w", i, err)
			}
			if len(rule) < 2 {
				return fmt.Errorf("policy %d: too few fields", i)
			}
			// First element is the policy type (p, p2, etc.)
			ptype := rule[0]
			params := rule[1:]
			if _, err := e.AddNamedPolicy(ptype, params); err != nil {
				return fmt.Errorf("policy %d: %w", i, err)
			}
		}
	}

	// Load role links (grouping policies)
	if links, ok := cfg["role_links"].([]any); ok {
		for i, lRaw := range links {
			rule, err := toStringSlice(lRaw)
			if err != nil {
				return fmt.Errorf("role_link %d: %w", i, err)
			}
			if len(rule) < 2 {
				return fmt.Errorf("role_link %d: too few fields", i)
			}
			gtype := rule[0]
			params := rule[1:]
			if _, err := e.AddNamedGroupingPolicy(gtype, params); err != nil {
				return fmt.Errorf("role_link %d: %w", i, err)
			}
		}
	}

	return nil
}

// toStringSlice converts an []any to []string.
func toStringSlice(v any) ([]string, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", v)
	}
	result := make([]string, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			result[i] = fmt.Sprintf("%v", item)
		} else {
			result[i] = s
		}
	}
	return result, nil
}
