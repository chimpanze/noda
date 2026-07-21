package server

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// server-1: overlapping route groups must MERGE (outermost first), not first-match-wins.
func TestGetGroupMiddleware_MergesOverlappingGroups(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{
			"auth":      []any{"auth.jwt"},
			"adminonly": []any{"casbin.enforce"},
		},
		"route_groups": map[string]any{
			"/api":       map[string]any{"middleware_preset": "auth"},
			"/api/admin": map[string]any{"middleware_preset": "adminonly"},
		},
	})
	// Deterministic across many runs (old code returned a random single group).
	for range 50 {
		mw, err := srv.getGroupMiddleware("/api/admin/users")
		require.NoError(t, err)
		assert.Equal(t, []string{"auth.jwt", "casbin.enforce"}, mw,
			"both groups' middleware must merge, outermost (/api) first")
	}
}

// server-3: prefix matching must respect path-segment boundaries.
func TestGetGroupMiddleware_SegmentAware(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{"base": []any{"recover"}},
		"route_groups": map[string]any{
			"/api": map[string]any{"middleware_preset": "base"},
		},
	})

	got, err := srv.getGroupMiddleware("/api-docs")
	require.NoError(t, err)
	assert.Empty(t, got, "/api group must NOT match /api-docs")

	got, err = srv.getGroupMiddleware("/api")
	require.NoError(t, err)
	assert.Equal(t, []string{"recover"}, got, "/api group must match exact /api")

	got, err = srv.getGroupMiddleware("/api/x")
	require.NoError(t, err)
	assert.Equal(t, []string{"recover"}, got, "/api group must match /api/x")
}

// server-3: a root "/" prefix is intended to match everything.
func TestGetGroupMiddleware_RootPrefixMatchesAll(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{"base": []any{"recover"}},
		"route_groups": map[string]any{
			"/": map[string]any{"middleware_preset": "base"},
		},
	})
	got, err := srv.getGroupMiddleware("/anything/here")
	require.NoError(t, err)
	assert.Equal(t, []string{"recover"}, got)
}

// server-1: the merged group chain must still be deduped by the production
// resolver (getGroupMiddleware itself returns the un-deduped union).
func TestResolveMiddlewareChain_MergedGroupsDeduped(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{
			"outer": []any{"recover", "requestid"},
			"inner": []any{"requestid", "recover"},
		},
		"route_groups": map[string]any{
			"/api":       map[string]any{"middleware_preset": "outer"},
			"/api/admin": map[string]any{"middleware_preset": "inner"},
		},
	})
	handlers, err := srv.ResolveMiddlewareChain(map[string]any{"id": "r", "path": "/api/admin/x"})
	require.NoError(t, err)
	// union [recover, requestid, requestid, recover] -> dedup -> 2 handlers.
	assert.Len(t, handlers, 2)
}

// server-1: ordering constraints must be validated across the MERGED chain,
// so a parent group whose middleware would precede a child's auth still errors.
func TestResolveMiddlewareChain_MergedGroupsOrderValidated(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{
			"outer": []any{"casbin.enforce"},
			"inner": []any{"auth.jwt"},
		},
		"route_groups": map[string]any{
			"/api":       map[string]any{"middleware_preset": "outer"},
			"/api/admin": map[string]any{"middleware_preset": "inner"},
		},
	})
	_, err := srv.ResolveMiddlewareChain(map[string]any{"id": "r", "path": "/api/admin/x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must appear before")
}

const jwtTestSecret = "test-secret-key-that-is-at-least-32-bytes-long"

func jwtTestApp(t *testing.T, jwtCfg map[string]any) *fiber.App {
	t.Helper()
	jwtCfg["secret"] = jwtTestSecret
	h, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{"jwt": jwtCfg},
	})
	require.NoError(t, err)
	app := fiber.New()
	app.Use(h)
	app.Get("/protected", func(c fiber.Ctx) error {
		_ = c.Locals(api.LocalJWTClaims)
		return c.SendString("ok")
	})
	return app
}

func doJWT(t *testing.T, app *fiber.App, claims jwt.MapClaims) int {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(jwtTestSecret))
	require.NoError(t, err)
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+s)
	resp, err := app.Test(req)
	require.NoError(t, err)
	return resp.StatusCode
}

// server-2: audience validated only when configured.
func TestJWT_AudienceValidation(t *testing.T) {
	exp := time.Now().Add(time.Hour).Unix()

	// audience configured: mismatch rejected, match accepted.
	app := jwtTestApp(t, map[string]any{"audience": "expected-aud"})
	assert.Equal(t, 401, doJWT(t, app, jwt.MapClaims{"sub": "u", "aud": "other-aud", "exp": exp}),
		"token with wrong aud must be rejected when audience configured")
	assert.Equal(t, 200, doJWT(t, app, jwt.MapClaims{"sub": "u", "aud": "expected-aud", "exp": exp}),
		"token with correct aud must be accepted")

	// audience not configured: aud ignored.
	app2 := jwtTestApp(t, map[string]any{})
	assert.Equal(t, 200, doJWT(t, app2, jwt.MapClaims{"sub": "u", "aud": "anything", "exp": exp}),
		"aud must be ignored when audience not configured")
}

// server-2: issuer validated only when configured.
func TestJWT_IssuerValidation(t *testing.T) {
	exp := time.Now().Add(time.Hour).Unix()
	app := jwtTestApp(t, map[string]any{"issuer": "https://good.example"})
	assert.Equal(t, 401, doJWT(t, app, jwt.MapClaims{"sub": "u", "iss": "https://evil.example", "exp": exp}),
		"wrong iss must be rejected when issuer configured")
	assert.Equal(t, 200, doJWT(t, app, jwt.MapClaims{"sub": "u", "iss": "https://good.example", "exp": exp}))
}

// server-2: require_expiry rejects tokens with no exp only when enabled (default off).
func TestJWT_RequireExpiry(t *testing.T) {
	// default: no exp accepted (backward compatible).
	def := jwtTestApp(t, map[string]any{})
	assert.Equal(t, 200, doJWT(t, def, jwt.MapClaims{"sub": "u"}),
		"token without exp must be accepted by default")

	// require_expiry true: no exp rejected, with exp accepted.
	strict := jwtTestApp(t, map[string]any{"require_expiry": true})
	assert.Equal(t, 401, doJWT(t, strict, jwt.MapClaims{"sub": "u"}),
		"token without exp must be rejected when require_expiry=true")
	assert.Equal(t, 200, doJWT(t, strict, jwt.MapClaims{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()}))
}
