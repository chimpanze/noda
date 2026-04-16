package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/pkg/api"
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

func TestBuildMiddleware_Logger_IncludesRequestID(t *testing.T) {
	// BuildMiddleware("logger", ...) captures os.Stdout when constructing the
	// handler, so the pipe must be wired into os.Stdout *before* that call.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	ridHandler, err := BuildMiddleware("requestid", nil)
	require.NoError(t, err)
	logHandler, err := BuildMiddleware("logger", nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(ridHandler)
	app.Use(logHandler)
	app.Get("/test", func(c fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	rid := resp.Header.Get("X-Request-Id")
	require.NotEmpty(t, rid, "requestid middleware should set X-Request-Id response header")

	require.NoError(t, w.Close())
	captured, err := io.ReadAll(r)
	require.NoError(t, err)

	assert.Contains(t, string(captured), "request_id="+rid,
		"logger output %q should contain request_id=%s", string(captured), rid)
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
				"secret": "test-secret-key-that-is-at-least-32-bytes-long",
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
	secret := "test-secret-key-that-is-at-least-32-bytes-long"

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
		claims := c.Locals(api.LocalJWTClaims)
		userID := c.Locals(api.LocalJWTUserID)
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

func TestBuildMiddleware_Limiter_RedisStorage(t *testing.T) {
	// When storage is "redis" and redis_url is provided, the middleware should
	// be created successfully (it will use Redis-backed distributed storage).
	// We use miniredis to avoid requiring a real Redis instance.
	mr := miniredis.RunT(t)

	app := fiber.New()
	h, err := BuildMiddleware("limiter", map[string]any{
		"middleware": map[string]any{
			"limiter": map[string]any{
				"max":        float64(2),
				"expiration": "1m",
				"storage":    "redis",
				"redis_url":  "redis://" + mr.Addr(),
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

func TestBuildMiddleware_Limiter_RedisStorage_MissingURL(t *testing.T) {
	_, err := BuildMiddleware("limiter", map[string]any{
		"middleware": map[string]any{
			"limiter": map[string]any{
				"max":     float64(10),
				"storage": "redis",
			},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis_url is required")
}

func TestBuildMiddleware_Limiter_InMemoryDefault(t *testing.T) {
	// Without storage config, limiter should use in-memory storage (default).
	h, err := BuildMiddleware("limiter", map[string]any{
		"middleware": map[string]any{
			"limiter": map[string]any{
				"max":        float64(5),
				"expiration": "30s",
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, h)
}

func rsaPublicKeyPEM(pub *rsa.PublicKey) string {
	der, _ := x509.MarshalPKIXPublicKey(pub)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func ecdsaPublicKeyPEM(pub *ecdsa.PublicKey) string {
	der, _ := x509.MarshalPKIXPublicKey(pub)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func TestBuildMiddleware_JWT_RS256_ValidToken(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pubPEM := rsaPublicKeyPEM(&privKey.PublicKey)

	app := fiber.New()
	h, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"algorithm":  "RS256",
				"public_key": pubPEM,
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/protected", func(c fiber.Ctx) error {
		userID := c.Locals(api.LocalJWTUserID)
		return c.JSON(map[string]any{"user_id": userID})
	})

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "rsa-user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString(privKey)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "rsa-user-1")
}

func TestBuildMiddleware_JWT_ES256_ValidToken(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	pubPEM := ecdsaPublicKeyPEM(&privKey.PublicKey)

	app := fiber.New()
	h, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"algorithm":  "ES256",
				"public_key": pubPEM,
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/protected", func(c fiber.Ctx) error {
		userID := c.Locals(api.LocalJWTUserID)
		return c.JSON(map[string]any{"user_id": userID})
	})

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"sub": "ecdsa-user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString(privKey)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "ecdsa-user-1")
}

func TestBuildMiddleware_JWT_UnsupportedAlgorithm_Asymmetric(t *testing.T) {
	_, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"algorithm":  "PS256",
				"public_key": "not-used",
			},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported algorithm")
}

func TestBuildMiddleware_JWT_RSA_MissingPublicKey(t *testing.T) {
	_, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"algorithm": "RS256",
			},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "public_key or public_key_file is required")
}

func TestBuildMiddleware_JWT_RSA_PublicKeyFile(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pubPEM := rsaPublicKeyPEM(&privKey.PublicKey)
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "pub.pem")
	require.NoError(t, os.WriteFile(keyPath, []byte(pubPEM), 0644))

	app := fiber.New()
	h, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"algorithm":       "RS256",
				"public_key_file": keyPath,
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/protected", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": "file-user",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString(privKey)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestBuildMiddleware_StatusRemap_Instance(t *testing.T) {
	h, err := BuildMiddleware("response.status_remap:public", map[string]any{
		"middleware_instances": map[string]any{
			"response.status_remap:public": map[string]any{
				"type": "response.status_remap",
				"config": map[string]any{
					"map": map[string]any{"403": float64(401)},
				},
			},
		},
	})
	require.NoError(t, err)

	app := fiber.New()
	app.Use(h)
	app.Get("/forbidden", func(c fiber.Ctx) error { return c.Status(403).SendString("nope") })

	req := httptest.NewRequest("GET", "/forbidden", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}
