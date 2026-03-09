package server

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const e2eRBACModel = `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && r.act == p.act`

const e2eMultiTenantModel = `[request_definition]
r = sub, tenant, obj, act

[policy_definition]
p = sub, tenant, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.tenant) && r.tenant == p.tenant && keyMatch2(r.obj, p.obj) && r.act == p.act`

func makeToken(t *testing.T, secret, sub string, roles []string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": sub,
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	if len(roles) > 0 {
		claims["roles"] = roles
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return tokenStr
}

// TestE2E_Casbin_PermittedAndDenied tests full JWT + Casbin middleware chain through the server.
func TestE2E_Casbin_PermittedAndDenied(t *testing.T) {
	secret := "test-casbin-secret"

	srv := newTestServer(t,
		map[string]map[string]any{
			"get-data": {
				"method":     "GET",
				"path":       "/api/data",
				"middleware": []any{"auth.jwt", "casbin.enforce"},
				"trigger": map[string]any{
					"workflow": "respond-ok",
					"input":   map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"respond-ok": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   map[string]any{"message": "success"},
						},
					},
				},
				"edges": []any{},
			},
		},
		map[string]any{
			"security": map[string]any{
				"jwt": map[string]any{"secret": secret},
				"casbin": map[string]any{
					"model": e2eRBACModel,
					"policies": []any{
						[]any{"p", "admin", "/api/*", "GET"},
						[]any{"p", "admin", "/api/*", "POST"},
						[]any{"p", "viewer", "/api/data", "GET"},
					},
					"role_links": []any{
						[]any{"g", "alice", "admin"},
						[]any{"g", "bob", "viewer"},
					},
				},
			},
		},
	)

	t.Run("admin permitted", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/data", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "alice", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("viewer permitted on /api/data GET", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/data", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "bob", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("unauthorized user denied", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/data", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "charlie", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})

	t.Run("no token returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/data", nil)
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 401, resp.StatusCode)
	})
}

// TestE2E_Casbin_AdminVsMember tests role-based access with different privilege levels.
func TestE2E_Casbin_AdminVsMember(t *testing.T) {
	secret := "test-roles-secret"

	srv := newTestServer(t,
		map[string]map[string]any{
			"get-users": {
				"method":     "GET",
				"path":       "/api/users",
				"middleware": []any{"auth.jwt", "casbin.enforce"},
				"trigger": map[string]any{
					"workflow": "respond-ok",
					"input":   map[string]any{},
				},
			},
			"delete-user": {
				"method":     "DELETE",
				"path":       "/api/users",
				"middleware": []any{"auth.jwt", "casbin.enforce"},
				"trigger": map[string]any{
					"workflow": "respond-ok",
					"input":   map[string]any{},
				},
			},
			"post-data": {
				"method":     "POST",
				"path":       "/api/data",
				"middleware": []any{"auth.jwt", "casbin.enforce"},
				"trigger": map[string]any{
					"workflow": "respond-ok",
					"input":   map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"respond-ok": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": map[string]any{"ok": true}},
					},
				},
				"edges": []any{},
			},
		},
		map[string]any{
			"security": map[string]any{
				"jwt": map[string]any{"secret": secret},
				"casbin": map[string]any{
					"model": e2eRBACModel,
					"policies": []any{
						[]any{"p", "admin", "/api/*", "GET"},
						[]any{"p", "admin", "/api/*", "POST"},
						[]any{"p", "admin", "/api/*", "DELETE"},
						[]any{"p", "member", "/api/data", "GET"},
						[]any{"p", "member", "/api/data", "POST"},
					},
					"role_links": []any{
						[]any{"g", "admin-user", "admin"},
						[]any{"g", "member-user", "member"},
					},
				},
			},
		},
	)

	t.Run("admin can GET /api/users", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/users", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "admin-user", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("admin can DELETE /api/users", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/users", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "admin-user", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("member can POST /api/data", func(t *testing.T) {
		payload := `{"value":"test"}`
		req := httptest.NewRequest("POST", "/api/data", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "member-user", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("member cannot GET /api/users", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/users", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "member-user", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})

	t.Run("member cannot DELETE /api/users", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/users", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "member-user", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})
}

// TestE2E_Casbin_MultiTenant tests tenant-scoped authorization.
func TestE2E_Casbin_MultiTenant(t *testing.T) {
	secret := "test-tenant-secret"

	srv := newTestServer(t,
		map[string]map[string]any{
			"workspace-data": {
				"method":     "GET",
				"path":       "/api/:workspace_id/data",
				"middleware": []any{"auth.jwt", "casbin.enforce"},
				"trigger": map[string]any{
					"workflow": "respond-ok",
					"input":   map[string]any{},
				},
			},
			"workspace-settings": {
				"method":     "POST",
				"path":       "/api/:workspace_id/settings",
				"middleware": []any{"auth.jwt", "casbin.enforce"},
				"trigger": map[string]any{
					"workflow": "respond-ok",
					"input":   map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"respond-ok": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": map[string]any{"ok": true}},
					},
				},
				"edges": []any{},
			},
		},
		map[string]any{
			"security": map[string]any{
				"jwt": map[string]any{"secret": secret},
				"casbin": map[string]any{
					"model":        e2eMultiTenantModel,
					"tenant_param": "workspace_id",
					"policies": []any{
						[]any{"p", "owner", "acme", "/api/acme/*", "GET"},
						[]any{"p", "owner", "acme", "/api/acme/*", "POST"},
						[]any{"p", "viewer", "acme", "/api/acme/data", "GET"},
						[]any{"p", "viewer", "globex", "/api/globex/data", "GET"},
					},
					"role_links": []any{
						[]any{"g", "alice", "owner", "acme"},
						[]any{"g", "alice", "viewer", "globex"},
						[]any{"g", "bob", "viewer", "acme"},
					},
				},
			},
		},
	)

	t.Run("alice owner in acme can POST settings", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/acme/settings", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "alice", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("alice viewer in globex can GET data", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/globex/data", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "alice", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("alice cannot POST in globex", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/globex/settings", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "alice", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})

	t.Run("bob viewer in acme can GET data", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/acme/data", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "bob", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("bob cannot POST in acme", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/acme/settings", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "bob", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})

	t.Run("bob has no access to globex", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/globex/data", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "bob", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})
}

// TestE2E_Casbin_MiddlewarePreset tests using casbin.enforce via middleware presets.
func TestE2E_Casbin_MiddlewarePreset(t *testing.T) {
	secret := "test-preset-secret"

	srv := newTestServer(t,
		map[string]map[string]any{
			"admin-endpoint": {
				"method":            "GET",
				"path":              "/admin/dashboard",
				"middleware_preset": "admin_only",
				"trigger": map[string]any{
					"workflow": "respond-ok",
					"input":   map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"respond-ok": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": map[string]any{"ok": true}},
					},
				},
				"edges": []any{},
			},
		},
		map[string]any{
			"middleware_presets": map[string]any{
				"admin_only": []any{"auth.jwt", "casbin.enforce"},
			},
			"security": map[string]any{
				"jwt": map[string]any{"secret": secret},
				"casbin": map[string]any{
					"model": e2eRBACModel,
					"policies": []any{
						[]any{"p", "admin", "/admin/*", "GET"},
					},
					"role_links": []any{
						[]any{"g", "admin-user", "admin"},
					},
				},
			},
		},
	)

	t.Run("admin preset permits admin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/dashboard", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "admin-user", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("admin preset denies non-admin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/dashboard", nil)
		req.Header.Set("Authorization", "Bearer "+makeToken(t, secret, "regular-user", nil))
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})
}
