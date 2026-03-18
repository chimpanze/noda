package secrets

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staticProvider is a test helper that returns fixed key-value pairs.
type staticProvider struct {
	name   string
	values map[string]string
	err    error
}

func (p *staticProvider) Name() string { return p.name }
func (p *staticProvider) Load(_ context.Context) (map[string]string, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.values, nil
}

func TestManager_GetHasKeys(t *testing.T) {
	m := New(&staticProvider{
		name:   "test",
		values: map[string]string{"API_KEY": "secret123", "DB_URL": "postgres://localhost"},
	})
	require.NoError(t, m.Load(context.Background()))

	v, ok := m.Get("API_KEY")
	assert.True(t, ok)
	assert.Equal(t, "secret123", v)

	assert.True(t, m.Has("DB_URL"))
	assert.False(t, m.Has("MISSING"))

	keys := m.Keys()
	assert.Equal(t, []string{"API_KEY", "DB_URL"}, keys)
}

func TestManager_Empty(t *testing.T) {
	m := New()
	require.NoError(t, m.Load(context.Background()))

	_, ok := m.Get("ANYTHING")
	assert.False(t, ok)
	assert.False(t, m.Has("ANYTHING"))
	assert.Empty(t, m.Keys())
}

func TestManager_MultipleProviders_LaterWins(t *testing.T) {
	m := New(
		&staticProvider{name: "first", values: map[string]string{"KEY": "from-first", "ONLY_FIRST": "yes"}},
		&staticProvider{name: "second", values: map[string]string{"KEY": "from-second", "ONLY_SECOND": "yes"}},
	)
	require.NoError(t, m.Load(context.Background()))

	v, _ := m.Get("KEY")
	assert.Equal(t, "from-second", v, "later provider should win")

	assert.True(t, m.Has("ONLY_FIRST"))
	assert.True(t, m.Has("ONLY_SECOND"))
}

func TestManager_ProviderError(t *testing.T) {
	m := New(
		&staticProvider{name: "bad", err: fmt.Errorf("connection refused")},
	)
	err := m.Load(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestManager_ExpressionContext(t *testing.T) {
	m := New(&staticProvider{
		name:   "test",
		values: map[string]string{"A": "1", "B": "2"},
	})
	require.NoError(t, m.Load(context.Background()))

	ctx := m.ExpressionContext()
	assert.Equal(t, "1", ctx["A"])
	assert.Equal(t, "2", ctx["B"])
	assert.Len(t, ctx, 2)
}
