package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
)

func vmRC(root map[string]any, routes map[string]map[string]any) *config.ResolvedConfig {
	if root == nil {
		root = map[string]any{}
	}
	return &config.ResolvedConfig{Root: root, Routes: routes}
}

func errsContain(t *testing.T, errs []error, substr string) {
	t.Helper()
	for _, e := range errs {
		if strings.Contains(e.Error(), substr) {
			return
		}
	}
	t.Fatalf("expected an error containing %q, got %v", substr, errs)
}

func TestValidateMiddlewareBuilds_LimiterWithoutConfig(t *testing.T) {
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"limiter"}},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), "max=0")
}

func TestValidateMiddlewareBuilds_ValidConfigPasses(t *testing.T) {
	rc := vmRC(map[string]any{
		"middleware": map[string]any{
			"limiter": map[string]any{"max": float64(20), "expiration": "1m"},
		},
	}, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"limiter", "compress"}},
	})
	if errs := ValidateMiddlewareBuilds(rc); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateMiddlewareBuilds_UnknownMiddleware(t *testing.T) {
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"no.such.middleware"}},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), "unknown middleware")
}

func TestValidateMiddlewareBuilds_RedisStorageNotContacted(t *testing.T) {
	// redis_url points nowhere routable — validation must not try to connect
	rc := vmRC(map[string]any{
		"middleware": map[string]any{
			"limiter":     map[string]any{"max": float64(5), "storage": "redis", "redis_url": "redis://192.0.2.1:6379"},
			"idempotency": map[string]any{"storage": "redis", "redis_url": "redis://192.0.2.1:6379"},
		},
	}, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"limiter", "idempotency"}},
	})
	if errs := ValidateMiddlewareBuilds(rc); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateMiddlewareBuilds_OIDCConfigCheckedWithoutDiscovery(t *testing.T) {
	// missing issuer_url → error
	rc := vmRC(map[string]any{
		"security": map[string]any{"oidc": map[string]any{"client_id": "app"}},
	}, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"auth.oidc"}},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), "issuer_url")

	// complete config → no error and no discovery fetch (unroutable issuer)
	rc = vmRC(map[string]any{
		"security": map[string]any{"oidc": map[string]any{"issuer_url": "https://192.0.2.1/realm", "client_id": "app"}},
	}, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"auth.oidc"}},
	})
	if errs := ValidateMiddlewareBuilds(rc); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidateMiddlewareBuilds_AuthSessionSkipped(t *testing.T) {
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"auth.session"}},
	})
	if errs := ValidateMiddlewareBuilds(rc); len(errs) != 0 {
		t.Fatalf("auth.session must not be built at validate time, got %v", errs)
	}
}

func TestValidateMiddlewareBuilds_JWTWithoutSecret(t *testing.T) {
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"auth.jwt"}},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), "auth.jwt")
}

func TestValidateMiddlewareBuilds_PresetExpansion(t *testing.T) {
	rc := vmRC(map[string]any{
		"middleware_presets": map[string]any{
			"public": []any{"limiter"},
		},
	}, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware_preset": "public"},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), "max=0")
}

func TestValidateMiddlewareBuilds_GlobalMiddleware(t *testing.T) {
	rc := vmRC(map[string]any{
		"global_middleware": []any{"timeout"},
		"middleware": map[string]any{
			"timeout": map[string]any{"duration": "not-a-duration"},
		},
	}, nil)
	errsContain(t, ValidateMiddlewareBuilds(rc), "invalid duration")
}

func TestValidateMiddlewareBuilds_ConnectionEndpoints(t *testing.T) {
	rc := vmRC(nil, nil)
	rc.Connections = map[string]map[string]any{
		"chat": {
			"endpoints": map[string]any{
				"room": map[string]any{"middleware": []any{"limiter"}},
			},
		},
	}
	errsContain(t, ValidateMiddlewareBuilds(rc), "max=0")
}

func TestValidateMiddlewareBuilds_OrderingViolation(t *testing.T) {
	// casbin.enforce alone does not violate the ordering rule — the rule only
	// fires when its dependency is present *after* it — so a chain with only
	// casbin.enforce fails on its missing config instead, which is a different
	// check. Pair it with a trailing auth.jwt to exercise the ordering rule.
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"casbin.enforce", "auth.jwt"}},
	})
	errsContain(t, ValidateMiddlewareBuilds(rc), `must appear before "casbin.enforce"`)
}

// brokenLimiterRoutes builds n routes all referencing the same unconfigured
// limiter, named so that map iteration order and sorted order differ.
func brokenLimiterRoutes(names ...string) map[string]map[string]any {
	routes := make(map[string]map[string]any, len(names))
	for _, name := range names {
		routes[name] = map[string]any{"id": name, "path": "/" + name, "middleware": []any{"limiter"}}
	}
	return routes
}

// #450: naming only the first route reached hid how many were affected.
func TestValidateMiddlewareBuilds_NamesEveryAffectedRoute(t *testing.T) {
	rc := vmRC(nil, brokenLimiterRoutes("tasks", "create-task", "get-task"))

	errs := ValidateMiddlewareBuilds(rc)
	if len(errs) != 1 {
		t.Fatalf("one broken middleware must produce one error, got %d: %v", len(errs), errs)
	}
	got := errs[0].Error()
	for _, route := range []string{"create-task", "get-task", "tasks"} {
		if !strings.Contains(got, `route "`+route+`"`) {
			t.Errorf("error must name affected route %q, got: %s", route, got)
		}
	}
	// Sorted, so the reported order does not depend on map iteration.
	if !strings.HasPrefix(got, `route "create-task", route "get-task", route "tasks": middleware "limiter": `) {
		t.Errorf("routes must be listed in sorted order, got: %s", got)
	}
}

// #450: identical input produced a different message between runs, because
// Go randomizes map iteration order. 50 rounds over 4 routes / 2 connections
// makes an unsorted range astronomically unlikely to agree with itself.
func TestValidateMiddlewareBuilds_ErrorsAreStableAcrossRuns(t *testing.T) {
	newRC := func() *config.ResolvedConfig {
		routes := brokenLimiterRoutes("delta", "alpha", "charlie")
		// A route whose middleware cannot be resolved at all: that error is
		// emitted per route, so its position depends on iteration order too.
		routes["bravo"] = map[string]any{"id": "bravo", "path": "/bravo", "middleware_preset": "no-such-preset"}

		rc := vmRC(nil, routes)
		// Two connections, one with two endpoints — three more scopes for the
		// same broken limiter, reached through a second nested map each time.
		rc.Connections = map[string]map[string]any{
			"zulu":    {"endpoints": map[string]any{"yankee": map[string]any{"middleware": []any{"limiter"}}}},
			"whiskey": {"endpoints": map[string]any{"victor": map[string]any{"middleware": []any{"limiter"}}}},
		}
		return rc
	}

	render := func(errs []error) string {
		parts := make([]string, len(errs))
		for i, e := range errs {
			parts[i] = e.Error()
		}
		return strings.Join(parts, "\n")
	}

	// Guard the fixture: the run must actually reach every kind of scope, and
	// stay under the truncation limit, or "stable" would be trivially true.
	want := render(ValidateMiddlewareBuilds(newRC()))
	for _, required := range []string{
		`route "alpha"`,
		`connection "whiskey" endpoint "victor"`,
		`connection "zulu" endpoint "yankee"`,
		`unknown middleware preset "no-such-preset"`,
	} {
		if !strings.Contains(want, required) {
			t.Fatalf("fixture must produce %s, got: %s", required, want)
		}
	}
	if strings.Contains(want, "more") {
		t.Fatalf("fixture must stay under the truncation limit, got: %s", want)
	}
	for i := range 50 {
		if got := render(ValidateMiddlewareBuilds(newRC())); got != want {
			t.Fatalf("run %d differed from run 0\nfirst: %s\nlater: %s", i+1, want, got)
		}
	}
}

func TestValidateMiddlewareBuilds_CountsScopesBeyondTheListedLimit(t *testing.T) {
	names := make([]string, 0, maxReportedScopes+3)
	for i := range maxReportedScopes + 3 {
		names = append(names, fmt.Sprintf("route%02d", i))
	}
	rc := vmRC(nil, brokenLimiterRoutes(names...))

	errs := ValidateMiddlewareBuilds(rc)
	if len(errs) != 1 {
		t.Fatalf("expected one error, got %d: %v", len(errs), errs)
	}
	got := errs[0].Error()
	if !strings.Contains(got, "and 3 more") {
		t.Errorf("scopes beyond the listed limit must be counted, not dropped, got: %s", got)
	}
	if strings.Contains(got, `route "route05"`) {
		t.Errorf("only %d scopes should be listed, got: %s", maxReportedScopes, got)
	}
}
