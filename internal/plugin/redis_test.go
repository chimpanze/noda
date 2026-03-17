package plugin

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRedisClient_Success(t *testing.T) {
	mr := miniredis.RunT(t)
	config := map[string]any{
		"url": "redis://" + mr.Addr(),
	}
	client, err := NewRedisClient(config, "test")
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close()
}

func TestNewRedisClient_MissingURL(t *testing.T) {
	config := map[string]any{}
	_, err := NewRedisClient(config, "cache")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'url'")
	assert.Contains(t, err.Error(), "cache")
}

func TestNewRedisClient_EmptyURL(t *testing.T) {
	config := map[string]any{"url": ""}
	_, err := NewRedisClient(config, "cache")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'url'")
}

func TestNewRedisClient_NonStringURL(t *testing.T) {
	config := map[string]any{"url": 123}
	_, err := NewRedisClient(config, "cache")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'url'")
}

func TestNewRedisClient_InvalidURL(t *testing.T) {
	config := map[string]any{"url": "not-a-redis-url"}
	_, err := NewRedisClient(config, "cache")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse url")
}

func TestNewRedisClient_WithPoolSize(t *testing.T) {
	mr := miniredis.RunT(t)
	config := map[string]any{
		"url":       "redis://" + mr.Addr(),
		"pool_size": float64(20),
	}
	client, err := NewRedisClient(config, "test")
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close()
	assert.Equal(t, 20, client.Options().PoolSize)
}

func TestNewRedisClient_WithMinIdle(t *testing.T) {
	mr := miniredis.RunT(t)
	config := map[string]any{
		"url":      "redis://" + mr.Addr(),
		"min_idle": float64(5),
	}
	client, err := NewRedisClient(config, "test")
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close()
	assert.Equal(t, 5, client.Options().MinIdleConns)
}

func TestNewRedisClient_WithAllOptions(t *testing.T) {
	mr := miniredis.RunT(t)
	config := map[string]any{
		"url":       "redis://" + mr.Addr(),
		"pool_size": 15,
		"min_idle":  3,
	}
	client, err := NewRedisClient(config, "test")
	require.NoError(t, err)
	require.NotNil(t, client)
	defer client.Close()
	assert.Equal(t, 15, client.Options().PoolSize)
	assert.Equal(t, 3, client.Options().MinIdleConns)
}
