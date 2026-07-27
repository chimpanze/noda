package server

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/routecfg"
)

// maxReportedScopes bounds how many referencing routes or endpoints a single
// middleware error names. Past that the remainder is counted rather than
// listed — the count is still reported, so the scope of the problem is never
// silently understated.
const maxReportedScopes = 5

// ValidateMiddlewareBuilds builds every middleware referenced by
// global_middleware, route groups, presets, routes, and connection endpoints,
// discarding the handlers, so factory-time config errors (limiter max=0,
// invalid durations, missing jwt config) surface at validate time instead of
// crashing the server at boot.
//
// Factories with build-time side effects are not called; offlineChecks lists
// every such type and the check that stands in for it. Their config is still
// resolved the way BuildMiddleware resolves it, so a reference to a missing
// middleware_instances entry fails here exactly as it fails at boot.
//
// Routes, connections, and endpoints are visited in sorted order, and a
// middleware that fails is reported once naming every scope that references
// it. Both matter for the same reason (#450): ranging over the config maps
// directly and deduplicating on the middleware name alone meant the error
// named whichever route Go's map iteration reached first, so it changed
// between runs on identical input and hid how many routes were affected.
func ValidateMiddlewareBuilds(rc *config.ResolvedConfig) []error {
	s := &Server{config: rc}
	var errs []error

	// A middleware's build depends only on its name and the project config,
	// never on the route referencing it, so each name is built at most once
	// and the resulting error is shared by every scope that names it.
	buildErr := map[string]error{}
	scopes := map[string][]string{}
	files := map[string][]string{}
	var failed []string

	// check records that `scope` references `name`. file is the source path of
	// the config declaring that scope, or "" for project-wide scopes such as
	// global_middleware.
	check := func(scope, file, name string) {
		err, built := buildErr[name]
		if !built {
			err = s.checkMiddlewareBuild(name)
			buildErr[name] = err
			if err != nil {
				failed = append(failed, name)
			}
		}
		if err == nil {
			return
		}
		if !slices.Contains(scopes[name], scope) {
			scopes[name] = append(scopes[name], scope)
		}
		if file != "" && !slices.Contains(files[name], file) {
			files[name] = append(files[name], file)
		}
	}

	for _, name := range s.getGlobalMiddleware() {
		check("global_middleware", "", name)
	}

	for _, id := range sortedSectionKeys(s.config.Routes) {
		names, err := s.resolveMiddlewareNames(s.config.Routes[id])
		if err != nil {
			// Typed, not a bare fmt.Errorf: the section keys are the absolute
			// source paths, so this fault knows its file, and a caller that
			// filters failures by file drops anything that arrives without one.
			errs = append(errs, &MiddlewareChainError{
				Files: []string{id},
				Err:   fmt.Errorf("route %q: %w", id, err),
			})
			continue
		}
		for _, name := range names {
			check(fmt.Sprintf("route %q", id), id, name)
		}
	}

	for _, connID := range sortedSectionKeys(s.config.Connections) {
		endpoints, _ := s.config.Connections[connID]["endpoints"].(map[string]any)
		epNames := make([]string, 0, len(endpoints))
		for epName := range endpoints {
			epNames = append(epNames, epName)
		}
		sort.Strings(epNames)

		for _, epName := range epNames {
			ep, _ := endpoints[epName].(map[string]any)
			if ep == nil {
				continue
			}
			scope := fmt.Sprintf("connection %q endpoint %q", connID, epName)
			names, err := s.resolveEndpointMiddlewareNames(ep)
			if err != nil {
				errs = append(errs, &MiddlewareChainError{
					Files: []string{connID},
					Err:   fmt.Errorf("%s: %w", scope, err),
				})
				continue
			}
			for _, name := range names {
				check(scope, connID, name)
			}
		}
	}

	for _, name := range failed {
		// files[name] is sorted within routes and sorted within connections
		// (each visited via sortedSectionKeys), but the two passes are not
		// merged in file order — a middleware referenced by both needs an
		// explicit sort here to be globally sorted.
		sort.Strings(files[name])
		errs = append(errs, &MiddlewareBuildError{
			Name:  name,
			Files: files[name],
			Err:   fmt.Errorf("%s: middleware %q: %w", joinScopes(scopes[name]), name, buildErr[name]),
		})
	}

	return errs
}

// sortedSectionKeys returns the keys of a config section in a stable order, so
// error messages derived from it do not depend on map iteration order.
func sortedSectionKeys(m map[string]map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// joinScopes renders the scopes referencing one failing middleware, listing at
// most maxReportedScopes of them and counting the rest.
func joinScopes(scopes []string) string {
	if len(scopes) <= maxReportedScopes {
		return strings.Join(scopes, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(scopes[:maxReportedScopes], ", "), len(scopes)-maxReportedScopes)
}

// noFactoryCheck substitutes for a factory that has nothing to validate
// offline. Config resolution still runs for these — see offlineChecks.
func noFactoryCheck(map[string]any) error { return nil }

// offlineChecks maps a middleware base type to the check that stands in for its
// factory when calling the factory would open a connection or start a
// goroutine. Membership in this map is the single definition of "validated
// offline": checkMiddlewareBuild resolves the config for every entry before
// dispatching here, so an entry cannot skip the instance lookup the way a
// hand-written `return nil` case could — which is how security.csrf and
// auth.session came to accept a reference to a nonexistent middleware_instances
// entry that boot rejects.
var offlineChecks = map[string]func(cfg map[string]any) error{
	// auth.session is server-scoped: it needs the live service registry, so
	// there is nothing a validate-time build could check.
	"auth.session": noFactoryCheck,

	// security.csrf is offline for the opposite reason: there is nothing to
	// check (newCSRFMiddleware only copies cookie settings out of the config
	// and sets no TrustedOrigins, the one field csrf.New panics on), and
	// calling it is not free. fiber's csrf.New defaults to in-memory storage,
	// whose constructor starts a goroutine with a 10-second GC ticker that
	// nothing here can close. This runs on every `noda start`, every dev-mode
	// reload, and every editor save, so building it to learn it always
	// succeeds leaked a goroutine per validation for the life of a dev
	// session. Resolving the config below does not call csrf.New, so the
	// instance lookup costs no goroutine.
	"security.csrf": noFactoryCheck,

	// The rest are validated up to, but not including, the external call:
	// redis-backed limiter/idempotency storage is not connected, and OIDC
	// discovery is not fetched.
	"auth.oidc":   func(cfg map[string]any) error { _, err := parseOIDCConfig(cfg); return err },
	"limiter":     func(cfg map[string]any) error { _, _, err := parseLimiterConfig(cfg); return err },
	"idempotency": func(cfg map[string]any) error { _, _, err := parseIdempotencyConfig(cfg); return err },
}

// checkMiddlewareBuild validates that a middleware would build at boot,
// substituting offline config checks for the factories that open connections.
func (s *Server) checkMiddlewareBuild(name string) error {
	baseType, _ := ParseMiddlewareName(name)

	check, offline := offlineChecks[baseType]
	if !offline {
		_, err := BuildMiddleware(name, s.config.Root)
		return err
	}

	// Resolve the config exactly as BuildMiddleware would. This sits outside
	// the per-type dispatch deliberately: a missing middleware_instances entry
	// fails at boot, so it must fail here too, for every offline-checked type,
	// by construction rather than by each case remembering to ask.
	cfg, err := s.middlewareConfigFor(name)
	if err != nil {
		return err
	}
	return check(cfg)
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
	return routecfg.ExtractMiddlewareConfig(name, s.config.Root), nil
}
