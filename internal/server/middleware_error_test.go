package server

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brokenLimiterConfig returns a project whose limiter has max=0 — accepted by
// file-level schema validation, rejected by the limiter factory at build time.
// Routes are keyed by source file path, matching how config.ValidateAll keys
// rc.Routes.
func brokenLimiterConfig(routeFiles ...string) *config.ResolvedConfig {
	routes := map[string]map[string]any{}
	for _, f := range routeFiles {
		routes[f] = map[string]any{
			"id":         "r",
			"method":     "GET",
			"path":       "/r",
			"middleware": []any{"limiter"},
		}
	}
	return &config.ResolvedConfig{
		Root: map[string]any{
			"middleware": map[string]any{
				"limiter": map[string]any{"max": float64(0)},
			},
		},
		Routes: routes,
	}
}

// Every file referencing the broken middleware must be recoverable, not just
// the first: the editor marks all of them.
func TestValidateMiddlewareBuilds_ErrorCarriesEveryReferencingFile(t *testing.T) {
	rc := brokenLimiterConfig("/proj/routes/a.json", "/proj/routes/b.json")

	errs := ValidateMiddlewareBuilds(rc)
	require.Len(t, errs, 1, "one failing middleware is reported once, naming every scope")

	var mwErr *MiddlewareBuildError
	require.ErrorAs(t, errs[0], &mwErr)
	assert.Equal(t, "limiter", mwErr.Name)
	assert.Equal(t, []string{"/proj/routes/a.json", "/proj/routes/b.json"}, mwErr.Files,
		"files must be in sorted order, matching the scope order in the message")
}

// global_middleware is declared project-wide, so it has no referencing file.
// internal/startup maps an empty Files to the root config.
func TestValidateMiddlewareBuilds_GlobalMiddlewareHasNoFile(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"global_middleware": []any{"limiter"},
			"middleware":        map[string]any{"limiter": map[string]any{"max": float64(0)}},
		},
	}

	errs := ValidateMiddlewareBuilds(rc)
	require.Len(t, errs, 1)

	var mwErr *MiddlewareBuildError
	require.ErrorAs(t, errs[0], &mwErr)
	assert.Empty(t, mwErr.Files)
}

// Files must be sorted globally, not just within routes and within
// connections separately: ValidateMiddlewareBuilds visits routes and
// connections as two separate sorted passes, so without a final sort the
// slice would read "sorted within routes, then sorted within connections"
// rather than one sorted list. The connection file here sorts before the
// route file, so an unsorted (append-order) result would read
// [/z-route.json, /a-conn.json] instead of the correct
// [/a-conn.json, /z-route.json].
func TestValidateMiddlewareBuilds_FilesSortedAcrossRoutesAndConnections(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"middleware": map[string]any{
				"limiter": map[string]any{"max": float64(0)},
			},
		},
		Routes: map[string]map[string]any{
			"/z-route.json": {
				"id":         "r",
				"method":     "GET",
				"path":       "/r",
				"middleware": []any{"limiter"},
			},
		},
		Connections: map[string]map[string]any{
			"/a-conn.json": {
				"id": "c",
				"endpoints": map[string]any{
					"ep": map[string]any{
						"middleware": []any{"limiter"},
					},
				},
			},
		},
	}

	errs := ValidateMiddlewareBuilds(rc)
	require.Len(t, errs, 1)

	var mwErr *MiddlewareBuildError
	require.ErrorAs(t, errs[0], &mwErr)
	assert.Equal(t, []string{"/a-conn.json", "/z-route.json"}, mwErr.Files)
}

// The message is what `noda validate` prints and what #450's tests pin.
func TestMiddlewareBuildError_TextIsUnchanged(t *testing.T) {
	rc := brokenLimiterConfig("/proj/routes/a.json")

	errs := ValidateMiddlewareBuilds(rc)
	require.Len(t, errs, 1)
	assert.Equal(t,
		`route "/proj/routes/a.json": middleware "limiter": limiter: max=0 is not allowed; set an explicit max request count`,
		errs[0].Error())
}
