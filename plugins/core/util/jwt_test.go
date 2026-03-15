package util

import (
	"context"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTSign_BasicToken(t *testing.T) {
	executor := newJWTSignExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"claims": map[string]any{
			"sub":  "user-1",
			"role": "admin",
		},
		"secret": "my-secret",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	// Parse and verify the token
	tokenStr := data.(string)
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
		return []byte("my-secret"), nil
	})
	require.NoError(t, err)
	assert.True(t, token.Valid)

	claims := token.Claims.(jwt.MapClaims)
	assert.Equal(t, "user-1", claims["sub"])
	assert.Equal(t, "admin", claims["role"])
}

func TestJWTSign_WithExpiry(t *testing.T) {
	executor := newJWTSignExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"claims": map[string]any{"sub": "user-1"},
		"secret": "my-secret",
		"expiry": "1h",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	token, err := jwt.Parse(data.(string), func(token *jwt.Token) (any, error) {
		return []byte("my-secret"), nil
	})
	require.NoError(t, err)

	claims := token.Claims.(jwt.MapClaims)
	exp, err := claims.GetExpirationTime()
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(time.Hour), exp.Time, 5*time.Second)
}

func TestJWTSign_DayExpiry(t *testing.T) {
	executor := newJWTSignExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"claims": map[string]any{"sub": "user-1"},
		"secret": "my-secret",
		"expiry": "7d",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	token, err := jwt.Parse(data.(string), func(token *jwt.Token) (any, error) {
		return []byte("my-secret"), nil
	})
	require.NoError(t, err)

	claims := token.Claims.(jwt.MapClaims)
	exp, err := claims.GetExpirationTime()
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), exp.Time, 5*time.Second)
}

func TestJWTSign_HS384(t *testing.T) {
	executor := newJWTSignExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"claims":    map[string]any{"sub": "user-1"},
		"secret":    "my-secret",
		"algorithm": "HS384",
	}

	_, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)

	token, err := jwt.Parse(data.(string), func(token *jwt.Token) (any, error) {
		assert.Equal(t, jwt.SigningMethodHS384, token.Method)
		return []byte("my-secret"), nil
	})
	require.NoError(t, err)
	assert.True(t, token.Valid)
}

func TestJWTSign_HS512(t *testing.T) {
	executor := newJWTSignExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"claims":    map[string]any{"sub": "user-1"},
		"secret":    "my-secret",
		"algorithm": "HS512",
	}

	_, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)

	token, err := jwt.Parse(data.(string), func(token *jwt.Token) (any, error) {
		assert.Equal(t, jwt.SigningMethodHS512, token.Method)
		return []byte("my-secret"), nil
	})
	require.NoError(t, err)
	assert.True(t, token.Valid)
}

func TestJWTSign_UnsupportedAlgorithm(t *testing.T) {
	executor := newJWTSignExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"claims":    map[string]any{"sub": "user-1"},
		"secret":    "my-secret",
		"algorithm": "RS256",
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported algorithm")
}

func TestJWTSign_MissingClaims(t *testing.T) {
	executor := newJWTSignExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"secret": "my-secret",
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "claims is required")
}

func TestJWTSign_InvalidExpiry(t *testing.T) {
	executor := newJWTSignExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"claims": map[string]any{"sub": "user-1"},
		"secret": "my-secret",
		"expiry": "not-a-duration",
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expiry")
}

func TestJWTSign_Descriptor(t *testing.T) {
	d := &jwtSignDescriptor{}
	assert.Equal(t, "jwt_sign", d.Name())
	assert.Nil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	require.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "claims")
	assert.Contains(t, props, "secret")
	assert.Contains(t, props, "algorithm")
	assert.Contains(t, props, "expiry")

	required := schema["required"].([]any)
	assert.Contains(t, required, "claims")
	assert.Contains(t, required, "secret")
}

func TestJWTSign_Outputs(t *testing.T) {
	executor := newJWTSignExecutor(nil)
	assert.Equal(t, []string{"success", "error"}, executor.Outputs())
}

func TestJWTSign_DefaultAlgorithmIsHS256(t *testing.T) {
	executor := newJWTSignExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"claims": map[string]any{"sub": "user-1"},
		"secret": "my-secret",
	}

	_, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)

	token, err := jwt.Parse(data.(string), func(token *jwt.Token) (any, error) {
		assert.Equal(t, jwt.SigningMethodHS256, token.Method)
		return []byte("my-secret"), nil
	})
	require.NoError(t, err)
	assert.True(t, token.Valid)
}
