package server

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/etag"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	fiberlogger "github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	fibertimeout "github.com/gofiber/fiber/v3/middleware/timeout"
	"github.com/golang-jwt/jwt/v5"
)

// MiddlewareFactory creates a Fiber handler from config.
type MiddlewareFactory func(config map[string]any, rootConfig map[string]any) (fiber.Handler, error)

// middlewareRegistry maps middleware names to their factory functions.
var middlewareRegistry = map[string]MiddlewareFactory{
	"recover":          newRecoverMiddleware,
	"logger":           newLoggerMiddleware,
	"requestid":        newRequestIDMiddleware,
	"security.cors":    newCORSMiddleware,
	"security.headers": newHelmetMiddleware,
	"security.csrf":    newCSRFMiddleware,
	"limiter":          newLimiterMiddleware,
	"timeout":          newTimeoutMiddleware,
	"compress":         newCompressMiddleware,
	"etag":             newETagMiddleware,
	"auth.jwt":         newJWTMiddleware,
	"casbin.enforce":   newCasbinMiddleware,
	"livekit.webhook":  newLiveKitWebhookMiddleware,
}

// ParseMiddlewareName splits a middleware name into its base type and instance.
// For "auth.jwt:v1" it returns ("auth.jwt", "v1"). For "auth.jwt" it returns ("auth.jwt", "").
func ParseMiddlewareName(name string) (baseType, instance string) {
	if idx := strings.Index(name, ":"); idx >= 0 {
		return name[:idx], name[idx+1:]
	}
	return name, ""
}

// extractInstanceConfig looks up the config for a named middleware instance
// from the middleware_instances section of the root config.
func extractInstanceConfig(name string, rootConfig map[string]any) map[string]any {
	instances, ok := rootConfig["middleware_instances"].(map[string]any)
	if !ok {
		return nil
	}
	entry, ok := instances[name].(map[string]any)
	if !ok {
		return nil
	}
	cfg, _ := entry["config"].(map[string]any)
	return cfg
}

// BuildMiddleware creates a Fiber handler from a middleware name and root config.
// If the name contains a ":" (e.g. "auth.jwt:v1"), it resolves the factory by
// base type and looks up config from middleware_instances.
func BuildMiddleware(name string, rootConfig map[string]any) (fiber.Handler, error) {
	baseType, instance := ParseMiddlewareName(name)

	factory, ok := middlewareRegistry[baseType]
	if !ok {
		return nil, fmt.Errorf("unknown middleware: %q", name)
	}

	var mwConfig map[string]any
	if instance != "" {
		mwConfig = extractInstanceConfig(name, rootConfig)
		if mwConfig == nil {
			return nil, fmt.Errorf("middleware instance %q not found in middleware_instances", name)
		}
	} else {
		mwConfig = extractMiddlewareConfig(name, rootConfig)
	}

	return factory(mwConfig, rootConfig)
}

// middlewareConfigPaths maps middleware names to alternative config lookup paths.
// Each path is a sequence of nested keys in the root config.
// The "middleware" section is always checked first for all middleware.
var middlewareConfigPaths = map[string][]string{
	"security.cors":    {"security", "cors"},
	"security.headers": {"security", "headers"},
	"security.csrf":    {"security", "csrf"},
	"auth.jwt":         {"security", "jwt"},
	"casbin.enforce":   {"security", "casbin"},
	"livekit.webhook":  {"security", "livekit"},
}

// extractMiddlewareConfig extracts the config block for a specific middleware.
func extractMiddlewareConfig(name string, rootConfig map[string]any) map[string]any {
	// Try middleware section first
	if mw, ok := rootConfig["middleware"].(map[string]any); ok {
		if cfg, ok := mw[name].(map[string]any); ok {
			return cfg
		}
	}
	// Try alternative config path
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

func newRecoverMiddleware(_ map[string]any, _ map[string]any) (fiber.Handler, error) {
	return recover.New(), nil
}

func newLoggerMiddleware(_ map[string]any, _ map[string]any) (fiber.Handler, error) {
	return fiberlogger.New(), nil
}

func newRequestIDMiddleware(_ map[string]any, _ map[string]any) (fiber.Handler, error) {
	return requestid.New(), nil
}

func newCORSMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	corsCfg := cors.Config{}
	if cfg != nil {
		if v, ok := cfg["allow_origins"].(string); ok {
			corsCfg.AllowOrigins = splitTrim(v)
		}
		if v, ok := cfg["allow_methods"].(string); ok {
			corsCfg.AllowMethods = splitTrim(v)
		}
		if v, ok := cfg["allow_headers"].(string); ok {
			corsCfg.AllowHeaders = splitTrim(v)
		}
		if v, ok := cfg["allow_credentials"].(bool); ok {
			corsCfg.AllowCredentials = v
		}
	}
	// Reject wildcard origin with credentials — browsers ignore this combination
	// and it may indicate a misconfiguration.
	for _, origin := range corsCfg.AllowOrigins {
		if origin == "*" && corsCfg.AllowCredentials {
			return nil, fmt.Errorf("security.cors: allow_origins \"*\" with allow_credentials is insecure and rejected by browsers")
		}
	}
	return cors.New(corsCfg), nil
}

func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func newHelmetMiddleware(_ map[string]any, _ map[string]any) (fiber.Handler, error) {
	return helmet.New(), nil
}

func newCSRFMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	csrfCfg := csrf.Config{}
	if cfg != nil {
		if v, ok := cfg["cookie_name"].(string); ok {
			csrfCfg.CookieName = v
		}
		if v, ok := cfg["cookie_secure"].(bool); ok {
			csrfCfg.CookieSecure = v
		}
		if v, ok := cfg["cookie_http_only"].(bool); ok {
			csrfCfg.CookieHTTPOnly = v
		}
		if v, ok := cfg["cookie_same_site"].(string); ok {
			csrfCfg.CookieSameSite = v
		}
		if v, ok := cfg["cookie_session_only"].(bool); ok {
			csrfCfg.CookieSessionOnly = v
		}
		if v, ok := cfg["single_use_token"].(bool); ok {
			csrfCfg.SingleUseToken = v
		}
	}
	return csrf.New(csrfCfg), nil
}

func newLimiterMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	limiterCfg := limiter.Config{}
	if cfg != nil {
		if v, ok := cfg["max"].(float64); ok {
			limiterCfg.Max = int(v)
		}
		if v, ok := cfg["expiration"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil {
				limiterCfg.Expiration = d
			} else {
				return nil, fmt.Errorf("limiter: invalid expiration %q: %w", v, err)
			}
		}
	}
	if limiterCfg.Max == 0 {
		return nil, fmt.Errorf("limiter: max=0 is not allowed; set an explicit max request count")
	}
	return limiter.New(limiterCfg), nil
}

func newTimeoutMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	d := 30 * time.Second
	if cfg != nil {
		if v, ok := cfg["duration"].(string); ok {
			parsed, err := time.ParseDuration(v)
			if err != nil {
				return nil, fmt.Errorf("timeout: invalid duration %q: %w", v, err)
			}
			d = parsed
		}
	}
	return fibertimeout.New(func(c fiber.Ctx) error {
		return c.Next()
	}, fibertimeout.Config{Timeout: d}), nil
}

func newCompressMiddleware(_ map[string]any, _ map[string]any) (fiber.Handler, error) {
	return compress.New(), nil
}

func newETagMiddleware(_ map[string]any, _ map[string]any) (fiber.Handler, error) {
	return etag.New(), nil
}

// newJWTMiddleware creates a JWT validation middleware.
// It validates the token and stores claims in Fiber locals for trigger mapping.
func newJWTMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("auth.jwt: security.jwt config is required")
	}

	secret, _ := cfg["secret"].(string)
	if secret == "" {
		return nil, fmt.Errorf("auth.jwt: secret is required")
	}

	// Reject weak secrets
	if len(secret) < 32 {
		return nil, fmt.Errorf("auth.jwt: secret is shorter than 32 bytes; use a stronger secret")
	}

	algorithm, _ := cfg["algorithm"].(string)
	if algorithm == "" {
		algorithm = "HS256"
	}

	var signingMethod jwt.SigningMethod
	switch algorithm {
	case "HS256":
		signingMethod = jwt.SigningMethodHS256
	case "HS384":
		signingMethod = jwt.SigningMethodHS384
	case "HS512":
		signingMethod = jwt.SigningMethodHS512
	default:
		return nil, fmt.Errorf("auth.jwt: unsupported algorithm %q", algorithm)
	}

	// Custom JWT middleware: parse token, validate, store claims in locals
	return func(c fiber.Ctx) error {
		auth := c.Get("Authorization")
		if auth == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing authorization header")
		}

		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		if tokenStr == auth {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid authorization format")
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if t.Method.Alg() != signingMethod.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(secret), nil
		})
		if err != nil {
			slog.Debug("jwt validation failed", "error", err)
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token")
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token claims")
		}

		// Store claims in Fiber locals for trigger mapping to access
		c.Locals(api.LocalJWTClaims, map[string]any(claims))
		if sub, ok := claims["sub"].(string); ok {
			c.Locals(api.LocalJWTUserID, sub)
		}
		if roles, ok := claims["roles"].([]any); ok {
			roleStrs := make([]string, 0, len(roles))
			for _, r := range roles {
				if s, ok := r.(string); ok {
					roleStrs = append(roleStrs, s)
				}
			}
			c.Locals(api.LocalJWTRoles, roleStrs)
		}

		return c.Next()
	}, nil
}
