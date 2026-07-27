package server

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

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

// Two middleware faults are found before any middleware is built: a preset
// that does not expand, and a violated ordering constraint. Both were returned
// as bare fmt.Errorf values, so internal/startup could attribute neither and
// the editor — which shows a file only the failures naming it — dropped them
// both. A route with either fault saved as valid while the project could not
// boot. rc.Routes and rc.Connections are keyed by absolute source path, so the
// attribution is available at the point the error is made.
func TestValidateMiddlewareBuilds_AttributesChainFaultsToTheirFile(t *testing.T) {
	const routeFile = "/proj/routes/r.json"
	const connFile = "/proj/connections/chat.json"

	for _, tc := range []struct {
		name     string
		rc       func() *config.ResolvedConfig
		wantFile string
		wantText string
	}{
		{
			name: "middleware ordering violation on a route",
			rc: func() *config.ResolvedConfig {
				return vmRC(nil, map[string]map[string]any{
					routeFile: {"id": "r", "path": "/x", "middleware": []any{"casbin.enforce", "auth.jwt"}},
				})
			},
			wantFile: routeFile,
			wantText: `route "` + routeFile + `": middleware "auth.jwt" must appear before "casbin.enforce" in the chain`,
		},
		{
			name: "unknown middleware preset on a route",
			rc: func() *config.ResolvedConfig {
				return vmRC(nil, map[string]map[string]any{
					routeFile: {"id": "r", "path": "/x", "middleware_preset": "nope"},
				})
			},
			wantFile: routeFile,
			wantText: `route "` + routeFile + `": route r: unknown middleware preset "nope"`,
		},
		{
			name: "unknown middleware preset on a connection endpoint",
			rc: func() *config.ResolvedConfig {
				rc := vmRC(nil, nil)
				rc.Connections = map[string]map[string]any{
					connFile: {"endpoints": map[string]any{
						"room": map[string]any{"middleware_preset": "nope"},
					}},
				}
				return rc
			},
			wantFile: connFile,
			wantText: `connection "` + connFile + `" endpoint "room": unknown middleware preset "nope"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateMiddlewareBuilds(tc.rc())
			if len(errs) != 1 {
				t.Fatalf("expected exactly one error, got %d: %v", len(errs), errs)
			}

			var chainErr *MiddlewareChainError
			if !errors.As(errs[0], &chainErr) {
				t.Fatalf("error must be a *MiddlewareChainError so callers can attribute it, got %T: %v", errs[0], errs[0])
			}
			if got := chainErr.MiddlewareFiles(); len(got) != 1 || got[0] != tc.wantFile {
				t.Errorf("MiddlewareFiles() = %v, want [%s]", got, tc.wantFile)
			}
			// Typing these errors must not have changed a byte of what the CLI
			// prints — #450's determinism tests pin this text.
			if got := errs[0].Error(); got != tc.wantText {
				t.Errorf("message text changed:\n got: %s\nwant: %s", got, tc.wantText)
			}
		})
	}
}

// Every error ValidateMiddlewareBuilds returns must carry file attribution, so
// that a fault added later cannot reach the editor unattributed the way these
// two did. errors.As on the interface is what internal/startup matches.
func TestValidateMiddlewareBuilds_EveryErrorCarriesAttribution(t *testing.T) {
	routes := brokenLimiterRoutes("/proj/routes/a.json", "/proj/routes/b.json")
	routes["/proj/routes/order.json"] = map[string]any{
		"id": "order", "path": "/order", "middleware": []any{"casbin.enforce", "auth.jwt"},
	}
	routes["/proj/routes/preset.json"] = map[string]any{
		"id": "preset", "path": "/preset", "middleware_preset": "nope",
	}

	errs := ValidateMiddlewareBuilds(vmRC(nil, routes))
	if len(errs) == 0 {
		t.Fatal("fixture must produce errors")
	}
	for _, err := range errs {
		var filesErr MiddlewareFilesError
		if !errors.As(err, &filesErr) {
			t.Errorf("error carries no file attribution (%T): %v", err, err)
			continue
		}
		if len(filesErr.MiddlewareFiles()) == 0 {
			t.Errorf("attribution is empty, so a per-file caller drops it: %v", err)
		}
	}
}

// security.csrf must be checked offline like the other side-effecting
// factories. fiber's csrf.New defaults to in-memory storage, whose constructor
// starts a GC goroutine on a 10-second ticker that nothing closes — and this
// validation now runs on every `noda start`, every dev-mode reload, and every
// editor save, so building it leaked one goroutine per run for the life of a
// dev session.
func TestValidateMiddlewareBuilds_CSRFLeaksNoGoroutines(t *testing.T) {
	rc := vmRC(nil, map[string]map[string]any{
		"r1": {"id": "r1", "path": "/x", "middleware": []any{"security.csrf"}},
	})
	if errs := ValidateMiddlewareBuilds(rc); len(errs) != 0 {
		t.Fatalf("security.csrf must validate cleanly, got %v", errs)
	}

	// Settle first, so goroutines left by earlier tests are not counted here.
	settle()
	before := runtime.NumGoroutine()

	const runs = 30
	for range runs {
		ValidateMiddlewareBuilds(rc)
	}

	settle()
	// A threshold rather than equality: unrelated runtime goroutines come and
	// go. The leak this guards adds one per run, so anything under a handful
	// cannot be it, and 30 runs makes the leaking case unmistakable.
	if grew := runtime.NumGoroutine() - before; grew > 5 {
		t.Errorf("%d validation runs left %d extra goroutines; security.csrf must not be built to be validated", runs, grew)
	}
}

// An offline-checked middleware substitutes a config check for its factory,
// but it must still resolve its config the way BuildMiddleware does, or a
// reference to a middleware_instances entry that does not exist passes
// validate and then fails at boot. security.csrf and auth.session both did:
// their cases returned nil before the instance lookup ran.
//
// This ranges offlineChecks — the same map checkMiddlewareBuild dispatches on
// — rather than a list of type names written out here. That is the point: an
// offline case added to that map later is covered by this guard automatically,
// with no second list to keep in sync. A separate list would be the same
// defect one layer up.
func TestValidateMiddlewareBuilds_OfflineChecksResolveTheirInstanceConfig(t *testing.T) {
	if len(offlineChecks) == 0 {
		t.Fatal("offlineChecks is empty, so this guard would assert nothing")
	}
	for baseType := range offlineChecks {
		t.Run(baseType, func(t *testing.T) {
			name := baseType + ":definitely-not-configured"
			rc := vmRC(nil, map[string]map[string]any{
				"r1": {"id": "r1", "path": "/x", "middleware": []any{name}},
			})
			errsContain(t, ValidateMiddlewareBuilds(rc),
				fmt.Sprintf("middleware instance %q not found in middleware_instances", name))
		})
	}
}

// The counterpart: a plain, non-instance reference has no middleware_instances
// entry to find, and middlewareConfigFor must return the extracted config with
// a nil error for it. Resolving config for every offline type would otherwise
// reject every project that uses security.csrf or auth.session without an
// instance suffix — which is nearly all of them.
func TestValidateMiddlewareBuilds_OfflineChecksAcceptTheNonInstanceForm(t *testing.T) {
	root := map[string]any{
		"middleware": map[string]any{
			"limiter":     map[string]any{"max": float64(20), "expiration": "1m"},
			"idempotency": map[string]any{},
		},
		"security": map[string]any{
			"oidc": map[string]any{"issuer_url": "https://192.0.2.1/realm", "client_id": "app"},
		},
	}
	for baseType := range offlineChecks {
		t.Run(baseType, func(t *testing.T) {
			rc := vmRC(root, map[string]map[string]any{
				"r1": {"id": "r1", "path": "/x", "middleware": []any{baseType}},
			})
			if errs := ValidateMiddlewareBuilds(rc); len(errs) != 0 {
				t.Fatalf("%s without an instance suffix must validate cleanly, got %v", baseType, errs)
			}
		})
	}
}

// settle gives goroutines started or stopped by the code under test a chance
// to be scheduled before they are counted.
func settle() {
	for range 10 {
		runtime.Gosched()
		time.Sleep(2 * time.Millisecond)
	}
}
