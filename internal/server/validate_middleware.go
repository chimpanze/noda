package server

import (
	"fmt"

	"github.com/chimpanze/noda/internal/config"
)

// ValidateMiddlewareBuilds builds every middleware referenced by
// global_middleware, route groups, presets, routes, and connection endpoints,
// discarding the handlers, so factory-time config errors (limiter max=0,
// invalid durations, missing jwt config) surface at validate time instead of
// crashing the server at boot.
//
// Factories with build-time side effects are validated up to, but not
// including, the external call: redis-backed limiter/idempotency storage is
// not connected and OIDC discovery is not fetched. auth.session is
// server-scoped (it needs the live service registry) and has no build-time
// validation to run.
func ValidateMiddlewareBuilds(rc *config.ResolvedConfig) []error {
	s := &Server{config: rc}
	var errs []error
	checked := map[string]bool{}

	check := func(scope, name string) {
		if checked[name] {
			return
		}
		checked[name] = true
		if err := s.checkMiddlewareBuild(name); err != nil {
			errs = append(errs, fmt.Errorf("%s: middleware %q: %w", scope, name, err))
		}
	}

	for _, name := range s.getGlobalMiddleware() {
		check("global_middleware", name)
	}

	for id, route := range s.config.Routes {
		names, err := s.resolveMiddlewareNames(route)
		if err != nil {
			errs = append(errs, fmt.Errorf("route %q: %w", id, err))
			continue
		}
		for _, name := range names {
			check(fmt.Sprintf("route %q", id), name)
		}
	}

	for connID, conn := range s.config.Connections {
		endpoints, _ := conn["endpoints"].(map[string]any)
		for epName, epAny := range endpoints {
			ep, _ := epAny.(map[string]any)
			if ep == nil {
				continue
			}
			names, err := s.resolveEndpointMiddlewareNames(ep)
			if err != nil {
				errs = append(errs, fmt.Errorf("connection %q endpoint %q: %w", connID, epName, err))
				continue
			}
			for _, name := range names {
				check(fmt.Sprintf("connection %q endpoint %q", connID, epName), name)
			}
		}
	}

	return errs
}

// checkMiddlewareBuild validates that a middleware would build at boot,
// substituting offline config checks for the factories that open connections.
func (s *Server) checkMiddlewareBuild(name string) error {
	baseType, _ := ParseMiddlewareName(name)
	switch baseType {
	case "auth.session":
		return nil
	case "auth.oidc":
		cfg, err := s.middlewareConfigFor(name)
		if err != nil {
			return err
		}
		_, err = parseOIDCConfig(cfg)
		return err
	case "limiter":
		cfg, err := s.middlewareConfigFor(name)
		if err != nil {
			return err
		}
		_, _, err = parseLimiterConfig(cfg)
		return err
	case "idempotency":
		cfg, err := s.middlewareConfigFor(name)
		if err != nil {
			return err
		}
		_, _, err = parseIdempotencyConfig(cfg)
		return err
	default:
		_, err := BuildMiddleware(name, s.config.Root)
		return err
	}
}

// middlewareConfigFor resolves a middleware's config the same way
// BuildMiddleware does: middleware_instances for "name:instance" references,
// the middleware/security sections otherwise.
func (s *Server) middlewareConfigFor(name string) (map[string]any, error) {
	_, instance := ParseMiddlewareName(name)
	if instance != "" {
		cfg := extractInstanceConfig(name, s.config.Root)
		if cfg == nil {
			return nil, fmt.Errorf("middleware instance %q not found in middleware_instances", name)
		}
		return cfg, nil
	}
	return extractMiddlewareConfig(name, s.config.Root), nil
}
