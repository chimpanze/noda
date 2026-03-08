package testing

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockExecutor_Success(t *testing.T) {
	mc := MockConfig{
		Output: map[string]any{"id": 1, "name": "Alice"},
	}
	executor := NewMockExecutor(mc)

	output, data, err := executor.Execute(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, map[string]any{"id": 1, "name": "Alice"}, data)
}

func TestMockExecutor_Error(t *testing.T) {
	mc := MockConfig{
		Error: &MockError{Message: "database connection failed"},
	}
	executor := NewMockExecutor(mc)

	_, _, err := executor.Execute(context.Background(), nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database connection failed")
}

func TestMockExecutor_OutputName(t *testing.T) {
	mc := MockConfig{
		Output:     map[string]any{"result": "ok"},
		OutputName: "approved",
	}
	executor := NewMockExecutor(mc)

	assert.Contains(t, executor.Outputs(), "approved")

	output, data, err := executor.Execute(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "approved", output)
	assert.Equal(t, map[string]any{"result": "ok"}, data)
}

func TestMockExecutor_NilOutput(t *testing.T) {
	mc := MockConfig{}
	executor := NewMockExecutor(mc)

	output, data, err := executor.Execute(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Nil(t, data)
}

func TestMockExecutor_Interface(t *testing.T) {
	mc := MockConfig{Output: "test"}
	var _ api.NodeExecutor = NewMockExecutor(mc)
}

func TestUnmockedExecutor(t *testing.T) {
	executor := NewUnmockedExecutor("my-node", "db.query")

	_, _, err := executor.Execute(context.Background(), nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "my-node")
	assert.Contains(t, err.Error(), "db.query")
	assert.Contains(t, err.Error(), "no mock")
}
