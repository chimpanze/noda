package plugin

import (
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDB struct {
	name string
}

func TestGetService_Success(t *testing.T) {
	db := &mockDB{name: "pg"}
	services := map[string]any{"db": db}

	result, err := GetService[*mockDB](services, "db")
	require.NoError(t, err)
	assert.Equal(t, "pg", result.name)
}

func TestGetService_MissingSlot(t *testing.T) {
	services := map[string]any{}

	_, err := GetService[*mockDB](services, "db")
	require.Error(t, err)

	var svcErr *api.ServiceUnavailableError
	assert.ErrorAs(t, err, &svcErr)
	assert.Equal(t, "db", svcErr.Service)
	assert.Contains(t, svcErr.Error(), "not configured")
}

func TestGetService_WrongType(t *testing.T) {
	services := map[string]any{"db": "not a db"}

	_, err := GetService[*mockDB](services, "db")
	require.Error(t, err)

	var svcErr *api.ServiceUnavailableError
	assert.ErrorAs(t, err, &svcErr)
	assert.Equal(t, "db", svcErr.Service)
	assert.Contains(t, svcErr.Error(), "does not implement")
}

func TestGetService_StringType(t *testing.T) {
	services := map[string]any{"cache": "hello"}
	result, err := GetService[string](services, "cache")
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestGetService_NilValue(t *testing.T) {
	services := map[string]any{"db": (*mockDB)(nil)}

	result, err := GetService[*mockDB](services, "db")
	require.NoError(t, err)
	assert.Nil(t, result)
}
