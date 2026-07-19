package server

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/chimpanze/noda/internal/routecfg"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
)

// newSessionMiddleware validates opaque session tokens issued by the auth
// plugin. Server-scoped because it needs the ServiceRegistry at request time.
func (s *Server) newSessionMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	serviceName := "auth"
	if v, ok := cfg["service"].(string); ok && v != "" {
		serviceName = v
	}
	return func(c fiber.Ctx) error {
		svcAny, ok := s.services.Get(serviceName)
		if !ok {
			slog.Error("auth.session: service not found", "service", serviceName)
			return fiber.NewError(fiber.StatusInternalServerError, "auth misconfigured")
		}
		authn, ok := svcAny.(api.SessionAuthenticator)
		if !ok {
			slog.Error("auth.session: service does not implement SessionAuthenticator", "service", serviceName)
			return fiber.NewError(fiber.StatusInternalServerError, "auth misconfigured")
		}
		db, ok := s.services.Get(authn.DatabaseServiceName())
		if !ok {
			slog.Error("auth.session: database service not found", "service", authn.DatabaseServiceName())
			return fiber.NewError(fiber.StatusInternalServerError, "auth misconfigured")
		}

		token := c.Cookies(authn.SessionCookieName())
		if token == "" {
			header := c.Get("Authorization")
			if t := strings.TrimPrefix(header, "Bearer "); t != header {
				token = t
			}
		}
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token")
		}

		ad, err := authn.AuthenticateSession(c.Context(), db, token)
		if err != nil {
			slog.Error("auth.session: validation error", "error", err)
			return fiber.NewError(fiber.StatusInternalServerError, "internal error")
		}
		if ad == nil {
			slog.Debug("auth.session: invalid session token")
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token")
		}

		c.Locals(api.LocalJWTClaims, ad.Claims)
		c.Locals(api.LocalJWTUserID, ad.UserID)
		c.Locals(api.LocalJWTRoles, ad.Roles)
		return c.Next()
	}, nil
}

// buildMiddleware resolves server-scoped middleware first, then falls back to
// the package-level registry.
func (s *Server) buildMiddleware(name string) (fiber.Handler, error) {
	baseType, instance := ParseMiddlewareName(name)
	factory, ok := s.serverMiddleware[baseType]
	if !ok {
		return BuildMiddleware(name, s.config.Root)
	}
	var mwConfig map[string]any
	if instance != "" {
		mwConfig = extractInstanceConfig(name, s.config.Root)
		if mwConfig == nil {
			return nil, fmt.Errorf("middleware instance %q not found in middleware_instances", name)
		}
	} else {
		mwConfig = routecfg.ExtractMiddlewareConfig(name, s.config.Root)
	}
	return factory(mwConfig, s.config.Root)
}
