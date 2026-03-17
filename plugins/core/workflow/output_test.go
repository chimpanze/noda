package workflow

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutput_ResolvedData(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"name": "Alice"}))
	executor := newOutputExecutor(nil)
	config := map[string]any{
		"name": "success",
		"data": "{{ input.name }}",
	}
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "Alice", data)
}

func TestOutput_NoData(t *testing.T) {
	execCtx := engine.NewExecutionContext()
	executor := newOutputExecutor(nil)
	config := map[string]any{
		"name": "success",
	}
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Nil(t, data)
}

func TestOutput_NameAccessible(t *testing.T) {
	config := map[string]any{"name": "my-output"}
	assert.Equal(t, "my-output", OutputName(config))
}

func TestOutput_MissingName(t *testing.T) {
	execCtx := engine.NewExecutionContext()
	executor := newOutputExecutor(nil)
	config := map[string]any{}
	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestOutput_TerminalNode(t *testing.T) {
	executor := newOutputExecutor(nil)
	assert.Empty(t, executor.Outputs())
}

func TestOutput_ResolveError(t *testing.T) {
	execCtx := engine.NewExecutionContext()
	executor := newOutputExecutor(nil)
	config := map[string]any{
		"name": "success",
		"data": "{{ invalid..expr }}",
	}
	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	assert.Error(t, err)
}

func TestOutputDescriptor_Metadata(t *testing.T) {
	desc := &outputDescriptor{}
	assert.Equal(t, "output", desc.Name())
	assert.Contains(t, desc.Description(), "output")
	assert.Nil(t, desc.ServiceDeps())

	schema := desc.ConfigSchema()
	assert.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	_, hasName := props["name"]
	assert.True(t, hasName)

	outputs := desc.OutputDescriptions()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

func TestOutputName_NilConfig(t *testing.T) {
	assert.Equal(t, "", OutputName(nil))
}

func TestOutputName_EmptyConfig(t *testing.T) {
	assert.Equal(t, "", OutputName(map[string]any{}))
}

func TestPlugin_Metadata(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "core.workflow", p.Name())
	assert.Equal(t, "workflow", p.Prefix())
	assert.False(t, p.HasServices())

	nodes := p.Nodes()
	assert.Len(t, nodes, 2)

	svc, err := p.CreateService(nil)
	assert.NoError(t, err)
	assert.Nil(t, svc)

	assert.NoError(t, p.HealthCheck(nil))
	assert.NoError(t, p.Shutdown(nil))
}
