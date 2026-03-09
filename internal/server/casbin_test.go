package server

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const aclModel = `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && keyMatch2(r.obj, p.obj) && r.act == p.act`

const rbacModel = `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && r.act == p.act`

const multiTenantModel = `[request_definition]
r = sub, tenant, obj, act

[policy_definition]
p = sub, tenant, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.tenant) && r.tenant == p.tenant && keyMatch2(r.obj, p.obj) && r.act == p.act`

func TestCasbin_PermittedRequest(t *testing.T) {
	cfg := map[string]any{
		"model": aclModel,
		"policies": []any{
			[]any{"p", "alice", "/api/data", "GET"},
		},
	}

	mw, err := newCasbinMiddleware(cfg, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Get("/api/data", func(c fiber.Ctx) error {
		c.Locals(LocalJWTUserID, "alice")
		return c.Next()
	}, mw, func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/api/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", string(body))
}

func TestCasbin_DeniedRequest(t *testing.T) {
	cfg := map[string]any{
		"model": aclModel,
		"policies": []any{
			[]any{"p", "alice", "/api/data", "GET"},
		},
	}

	mw, err := newCasbinMiddleware(cfg, nil)
	require.NoError(t, err)

	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		if fe, ok := err.(*fiber.Error); ok {
			return c.Status(fe.Code).SendString(fe.Message)
		}
		return c.Status(500).SendString(err.Error())
	}})
	app.Get("/api/data", func(c fiber.Ctx) error {
		c.Locals(LocalJWTUserID, "bob") // bob has no policy
		return c.Next()
	}, mw, func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/api/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestCasbin_NoSubject_Denied(t *testing.T) {
	cfg := map[string]any{
		"model": aclModel,
		"policies": []any{
			[]any{"p", "alice", "/api/data", "GET"},
		},
	}

	mw, err := newCasbinMiddleware(cfg, nil)
	require.NoError(t, err)

	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		if fe, ok := err.(*fiber.Error); ok {
			return c.Status(fe.Code).SendString(fe.Message)
		}
		return c.Status(500).SendString(err.Error())
	}})
	// No jwt_user_id set
	app.Get("/api/data", mw, func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/api/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

func TestCasbin_RBAC_AdminVsMember(t *testing.T) {
	cfg := map[string]any{
		"model": rbacModel,
		"policies": []any{
			[]any{"p", "admin", "/api/*", "GET"},
			[]any{"p", "admin", "/api/*", "POST"},
			[]any{"p", "admin", "/api/*", "DELETE"},
			[]any{"p", "member", "/api/data", "GET"},
		},
		"role_links": []any{
			[]any{"g", "alice", "admin"},
			[]any{"g", "bob", "member"},
		},
	}

	mw, err := newCasbinMiddleware(cfg, nil)
	require.NoError(t, err)

	errHandler := func(c fiber.Ctx, err error) error {
		if fe, ok := err.(*fiber.Error); ok {
			return c.Status(fe.Code).SendString(fe.Message)
		}
		return c.Status(500).SendString(err.Error())
	}

	setUser := func(userID string) fiber.Handler {
		return func(c fiber.Ctx) error {
			c.Locals(LocalJWTUserID, userID)
			return c.Next()
		}
	}

	handler := func(c fiber.Ctx) error {
		return c.SendString("ok")
	}

	t.Run("admin can GET", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: errHandler})
		app.Get("/api/data", setUser("alice"), mw, handler)
		resp, err := app.Test(httptest.NewRequest("GET", "/api/data", nil))
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("admin can DELETE", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: errHandler})
		app.Delete("/api/users", setUser("alice"), mw, handler)
		resp, err := app.Test(httptest.NewRequest("DELETE", "/api/users", nil))
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("member can GET /api/data", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: errHandler})
		app.Get("/api/data", setUser("bob"), mw, handler)
		resp, err := app.Test(httptest.NewRequest("GET", "/api/data", nil))
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("member cannot DELETE", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: errHandler})
		app.Delete("/api/data", setUser("bob"), mw, handler)
		resp, err := app.Test(httptest.NewRequest("DELETE", "/api/data", nil))
		require.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})

	t.Run("member cannot access /api/users", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: errHandler})
		app.Get("/api/users", setUser("bob"), mw, handler)
		resp, err := app.Test(httptest.NewRequest("GET", "/api/users", nil))
		require.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})
}

func TestCasbin_MultiTenant(t *testing.T) {
	cfg := map[string]any{
		"model": multiTenantModel,
		"policies": []any{
			[]any{"p", "admin", "ws-1", "/api/ws-1/*", "GET"},
			[]any{"p", "admin", "ws-1", "/api/ws-1/*", "POST"},
			[]any{"p", "member", "ws-2", "/api/ws-2/*", "GET"},
		},
		"role_links": []any{
			[]any{"g", "alice", "admin", "ws-1"},
			[]any{"g", "alice", "member", "ws-2"},
		},
		"tenant_param": "workspace_id",
	}

	mw, err := newCasbinMiddleware(cfg, nil)
	require.NoError(t, err)

	errHandler := func(c fiber.Ctx, err error) error {
		if fe, ok := err.(*fiber.Error); ok {
			return c.Status(fe.Code).SendString(fe.Message)
		}
		return c.Status(500).SendString(err.Error())
	}

	setUser := func(userID string) fiber.Handler {
		return func(c fiber.Ctx) error {
			c.Locals(LocalJWTUserID, userID)
			return c.Next()
		}
	}

	handler := func(c fiber.Ctx) error {
		return c.SendString("ok")
	}

	t.Run("alice admin in ws-1 can POST", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: errHandler})
		app.Post("/api/:workspace_id/data", setUser("alice"), mw, handler)
		resp, err := app.Test(httptest.NewRequest("POST", "/api/ws-1/data", nil))
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("alice member in ws-2 can GET", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: errHandler})
		app.Get("/api/:workspace_id/data", setUser("alice"), mw, handler)
		resp, err := app.Test(httptest.NewRequest("GET", "/api/ws-2/data", nil))
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("alice member in ws-2 cannot POST", func(t *testing.T) {
		app := fiber.New(fiber.Config{ErrorHandler: errHandler})
		app.Post("/api/:workspace_id/data", setUser("alice"), mw, handler)
		resp, err := app.Test(httptest.NewRequest("POST", "/api/ws-2/data", nil))
		require.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})
}

func TestCasbin_WildcardPath(t *testing.T) {
	cfg := map[string]any{
		"model": aclModel,
		"policies": []any{
			[]any{"p", "alice", "/api/users/:id", "GET"},
		},
	}

	mw, err := newCasbinMiddleware(cfg, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Get("/api/users/:id", func(c fiber.Ctx) error {
		c.Locals(LocalJWTUserID, "alice")
		return c.Next()
	}, mw, func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/api/users/123", nil))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestCasbin_MissingConfig(t *testing.T) {
	_, err := newCasbinMiddleware(nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
}

func TestCasbin_MissingModel(t *testing.T) {
	_, err := newCasbinMiddleware(map[string]any{}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model is required")
}

func TestCasbin_BuildMiddleware_Integration(t *testing.T) {
	// Test that casbin.enforce is registered and extractMiddlewareConfig works
	rootConfig := map[string]any{
		"security": map[string]any{
			"casbin": map[string]any{
				"model": aclModel,
				"policies": []any{
					[]any{"p", "alice", "/api/data", "GET"},
				},
			},
		},
	}

	mw, err := BuildMiddleware("casbin.enforce", rootConfig)
	require.NoError(t, err)
	require.NotNil(t, mw)
}
