package server

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

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
}

// BuildMiddleware creates a Fiber handler from a middleware name and root config.
func BuildMiddleware(name string, rootConfig map[string]any) (fiber.Handler, error) {
	factory, ok := middlewareRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown middleware: %q", name)
	}

	// Extract middleware-specific config from root config
	mwConfig := extractMiddlewareConfig(name, rootConfig)
	return factory(mwConfig, rootConfig)
}

// extractMiddlewareConfig extracts the config block for a specific middleware.
func extractMiddlewareConfig(name string, rootConfig map[string]any) map[string]any {
	// Try middleware section first
	if mw, ok := rootConfig["middleware"].(map[string]any); ok {
		if cfg, ok := mw[name].(map[string]any); ok {
			return cfg
		}
	}
	// Try security section for security.* middleware
	if strings.HasPrefix(name, "security.") {
		if sec, ok := rootConfig["security"].(map[string]any); ok {
			shortName := strings.TrimPrefix(name, "security.")
			if cfg, ok := sec[shortName].(map[string]any); ok {
				return cfg
			}
		}
	}
	// JWT config under security.jwt
	if name == "auth.jwt" {
		if sec, ok := rootConfig["security"].(map[string]any); ok {
			if cfg, ok := sec["jwt"].(map[string]any); ok {
				return cfg
			}
		}
	}
	// Casbin config under security.casbin
	if name == "casbin.enforce" {
		if sec, ok := rootConfig["security"].(map[string]any); ok {
			if cfg, ok := sec["casbin"].(map[string]any); ok {
				return cfg
			}
		}
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

func newCSRFMiddleware(_ map[string]any, _ map[string]any) (fiber.Handler, error) {
	return csrf.New(), nil
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
			}
		}
	}
	return limiter.New(limiterCfg), nil
}

func newTimeoutMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	d := 30 * time.Second
	if cfg != nil {
		if v, ok := cfg["duration"].(string); ok {
			if parsed, err := time.ParseDuration(v); err == nil {
				d = parsed
			}
		}
	}
	// Use Fiber v3 timeout middleware by wrapping c.Next() as the handler
	return func(c fiber.Ctx) error {
		wrapped := fibertimeout.New(func(c fiber.Ctx) error {
			return c.Next()
		}, fibertimeout.Config{Timeout: d})
		return wrapped(c)
	}, nil
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
		c.Locals("jwt_claims", map[string]any(claims))
		if sub, ok := claims["sub"].(string); ok {
			c.Locals("jwt_user_id", sub)
		}
		if roles, ok := claims["roles"].([]any); ok {
			roleStrs := make([]string, 0, len(roles))
			for _, r := range roles {
				if s, ok := r.(string); ok {
					roleStrs = append(roleStrs, s)
				}
			}
			c.Locals("jwt_roles", roleStrs)
		}

		return c.Next()
	}, nil
}

// applyMiddlewareChain applies a list of middleware names to a fiber group or route.
func (s *Server) applyMiddlewareChain(handlers []fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		// Execute handlers in order, then call Next
		for _, h := range handlers {
			if err := h(c); err != nil {
				return err
			}
		}
		return c.Next()
	}
}

