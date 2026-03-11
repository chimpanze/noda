package server

import (
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMiddlewareName(t *testing.T) {
	tests := []struct {
		name     string
		wantBase string
		wantInst string
	}{
		{"auth.jwt", "auth.jwt", ""},
		{"auth.jwt:v1", "auth.jwt", "v1"},
		{"limiter:strict", "limiter", "strict"},
		{"recover", "recover", ""},
		{"casbin.enforce:tenant", "casbin.enforce", "tenant"},
	}
	for _, tt := range tests {
		base, inst := ParseMiddlewareName(tt.name)
		assert.Equal(t, tt.wantBase, base, "base for %q", tt.name)
		assert.Equal(t, tt.wantInst, inst, "instance for %q", tt.name)
	}
}

func TestBuildMiddleware_Instance(t *testing.T) {
	h, err := BuildMiddleware("limiter:strict", map[string]any{
		"middleware_instances": map[string]any{
			"limiter:strict": map[string]any{
				"type": "limiter",
				"config": map[string]any{
					"max":        float64(100),
					"expiration": "1m",
				},
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, h)
}

func TestBuildMiddleware_Instance_NotFound(t *testing.T) {
	_, err := BuildMiddleware("limiter:missing", map[string]any{
		"middleware_instances": map[string]any{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "middleware instance")
	assert.Contains(t, err.Error(), "not found")
}

func TestBuildMiddleware_Instance_UnknownType(t *testing.T) {
	_, err := BuildMiddleware("nonexistent:foo", map[string]any{
		"middleware_instances": map[string]any{
			"nonexistent:foo": map[string]any{
				"type":   "nonexistent",
				"config": map[string]any{},
			},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown middleware")
}

func TestBuildMiddleware_Unknown(t *testing.T) {
	_, err := BuildMiddleware("nonexistent", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown middleware")
}

func TestBuildMiddleware_Recover(t *testing.T) {
	app := fiber.New()
	h, err := BuildMiddleware("recover", nil)
	require.NoError(t, err)

	app.Use(h)
	app.Get("/panic", func(c fiber.Ctx) error {
		panic("test panic")
	})

	req := httptest.NewRequest("GET", "/panic", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
}

func TestBuildMiddleware_RequestID(t *testing.T) {
	app := fiber.New()
	h, err := BuildMiddleware("requestid", nil)
	require.NoError(t, err)

	app.Use(h)
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Header.Get("X-Request-Id"))
}

func TestBuildMiddleware_CORS(t *testing.T) {
	app := fiber.New()
	h, err := BuildMiddleware("security.cors", map[string]any{
		"security": map[string]any{
			"cors": map[string]any{
				"allow_origins": "*",
				"allow_methods": "GET,POST",
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
}

func TestBuildMiddleware_JWT_MissingConfig(t *testing.T) {
	_, err := BuildMiddleware("auth.jwt", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "security.jwt config is required")
}

func TestBuildMiddleware_JWT_MissingSecret(t *testing.T) {
	_, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret is required")
}

func TestBuildMiddleware_JWT_RejectsInvalidToken(t *testing.T) {
	app := fiber.New()
	h, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"secret": "test-secret-key",
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// No auth header
	req := httptest.NewRequest("GET", "/protected", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	// Bad token
	req = httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestBuildMiddleware_JWT_ValidToken(t *testing.T) {
	secret := "test-secret-key"

	app := fiber.New()
	h, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"secret": secret,
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/protected", func(c fiber.Ctx) error {
		claims := c.Locals(LocalJWTClaims)
		userID := c.Locals(LocalJWTUserID)
		return c.JSON(map[string]any{
			"claims":  claims,
			"user_id": userID,
		})
	})

	// Create valid token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "user-123",
		"email": "test@example.com",
		"roles": []string{"admin"},
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte(secret))
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "user-123")
}

func TestBuildMiddleware_Limiter(t *testing.T) {
	app := fiber.New()
	h, err := BuildMiddleware("limiter", map[string]any{
		"middleware": map[string]any{
			"limiter": map[string]any{
				"max":        float64(2),
				"expiration": "1m",
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	}

	// Third should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)
}
