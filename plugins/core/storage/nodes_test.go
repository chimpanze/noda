package storage

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServices(t *testing.T) (map[string]any, *storageplugin.Service) {
	t.Helper()
	p := &storageplugin.Plugin{}
	rawSvc, err := p.CreateService(map[string]any{"backend": "memory"})
	require.NoError(t, err)
	svc := rawSvc.(*storageplugin.Service)
	return map[string]any{"storage": svc}, svc
}

func TestStorageWrite_Read_RoundTrip(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()

	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{}),
	)

	// Write
	we := newWriteExecutor(nil)
	output, result, err := we.Execute(ctx, execCtx, map[string]any{
		"path": "greet.txt",
		"data": "hello world",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	_ = result

	// Read it back
	re := newReadExecutor(nil)
	output, result, err = re.Execute(ctx, execCtx, map[string]any{
		"path": "greet.txt",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	rm := result.(map[string]any)
	assert.Equal(t, []byte("hello world"), rm["data"])
	assert.Equal(t, 11, rm["size"])
	assert.NotEmpty(t, rm["content_type"])
}

func TestStorageRead_MissingFile(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	re := newReadExecutor(nil)
	_, _, err := re.Execute(ctx, execCtx, map[string]any{"path": "nope.txt"}, services)
	require.Error(t, err)
}

func TestStorageDelete_ThenRead(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	require.NoError(t, svc.Write(ctx, "bye.txt", []byte("cya")))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	de := newDeleteExecutor(nil)
	output, _, err := de.Execute(ctx, execCtx, map[string]any{"path": "bye.txt"}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	re := newReadExecutor(nil)
	_, _, err = re.Execute(ctx, execCtx, map[string]any{"path": "bye.txt"}, services)
	require.Error(t, err)
}

func TestStorageList(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	require.NoError(t, svc.Write(ctx, "docs/a.txt", []byte("a")))
	require.NoError(t, svc.Write(ctx, "docs/b.txt", []byte("b")))
	require.NoError(t, svc.Write(ctx, "other/c.txt", []byte("c")))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	le := newListExecutor(nil)
	output, result, err := le.Execute(ctx, execCtx, map[string]any{"prefix": "docs"}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	rm := result.(map[string]any)
	paths := rm["paths"].([]string)
	assert.Len(t, paths, 2)
	assert.Contains(t, paths, "docs/a.txt")
	assert.Contains(t, paths, "docs/b.txt")
}

func TestStorageList_EmptyPrefix(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	le := newListExecutor(nil)
	output, result, err := le.Execute(ctx, execCtx, map[string]any{"prefix": "nonexistent"}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	rm := result.(map[string]any)
	paths := rm["paths"].([]string)
	assert.Empty(t, paths)
}

// --- Missing service errors ---

func TestStorageRead_MissingService(t *testing.T) {
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newReadExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{"path": "x.txt"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestStorageWrite_MissingService(t *testing.T) {
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newWriteExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{"path": "x.txt", "data": "hi"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestStorageDelete_MissingService(t *testing.T) {
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newDeleteExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{"path": "x.txt"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestStorageList_MissingService(t *testing.T) {
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newListExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{"prefix": "x"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

// --- Resolve errors (missing required config fields) ---

func TestStorageRead_MissingPath(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newReadExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage.read")
}

func TestStorageWrite_MissingPath(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newWriteExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{"data": "hi"}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage.write")
}

func TestStorageWrite_MissingData(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newWriteExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{"path": "x.txt"}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage.write")
}

func TestStorageDelete_MissingPath(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newDeleteExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage.delete")
}

func TestStorageList_MissingPrefix(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newListExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage.list")
}

// --- Write: byte data and invalid data type ---

func TestStorageWrite_ByteData(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newWriteExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"path": "binary.bin",
		"data": []byte{0x00, 0x01, 0x02},
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	// Verify it was written correctly
	data, err := svc.Read(ctx, "binary.bin")
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0x01, 0x02}, data)
}

func TestStorageWrite_InvalidDataType(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newWriteExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"path": "file.txt",
		"data": 12345,
	}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage.write: data must be string or bytes")
}

// --- Descriptor tests ---

func TestReadDescriptor(t *testing.T) {
	d := &readDescriptor{}
	assert.Equal(t, "read", d.Name())

	schema := d.ConfigSchema()
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "path")
	required := schema["required"].([]any)
	assert.Contains(t, required, "path")

	deps := d.ServiceDeps()
	assert.Contains(t, deps, "storage")
	assert.True(t, deps["storage"].Required)
}

func TestWriteDescriptor(t *testing.T) {
	d := &writeDescriptor{}
	assert.Equal(t, "write", d.Name())

	schema := d.ConfigSchema()
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "path")
	assert.Contains(t, props, "data")
	assert.Contains(t, props, "content_type")
	required := schema["required"].([]any)
	assert.Contains(t, required, "path")
	assert.Contains(t, required, "data")

	deps := d.ServiceDeps()
	assert.Contains(t, deps, "storage")
	assert.True(t, deps["storage"].Required)
}

func TestDeleteDescriptor(t *testing.T) {
	d := &deleteDescriptor{}
	assert.Equal(t, "delete", d.Name())

	schema := d.ConfigSchema()
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "path")
	required := schema["required"].([]any)
	assert.Contains(t, required, "path")

	deps := d.ServiceDeps()
	assert.Contains(t, deps, "storage")
	assert.True(t, deps["storage"].Required)
}

func TestListDescriptor(t *testing.T) {
	d := &listDescriptor{}
	assert.Equal(t, "list", d.Name())

	schema := d.ConfigSchema()
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "prefix")
	required := schema["required"].([]any)
	assert.Contains(t, required, "prefix")

	deps := d.ServiceDeps()
	assert.Contains(t, deps, "storage")
	assert.True(t, deps["storage"].Required)
}

// --- Executor outputs ---

func TestReadExecutor_Outputs(t *testing.T) {
	e := newReadExecutor(nil)
	outputs := e.Outputs()
	assert.Equal(t, []string{"success", "error"}, outputs)
}

func TestWriteExecutor_Outputs(t *testing.T) {
	e := newWriteExecutor(nil)
	outputs := e.Outputs()
	assert.Equal(t, []string{"success", "error"}, outputs)
}

func TestDeleteExecutor_Outputs(t *testing.T) {
	e := newDeleteExecutor(nil)
	outputs := e.Outputs()
	assert.Equal(t, []string{"success", "error"}, outputs)
}

func TestListExecutor_Outputs(t *testing.T) {
	e := newListExecutor(nil)
	outputs := e.Outputs()
	assert.Equal(t, []string{"success", "error"}, outputs)
}

// --- Plugin tests ---

func TestPlugin_Metadata(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "core.storage", p.Name())
	assert.Equal(t, "storage", p.Prefix())
	assert.False(t, p.HasServices())

	svc, err := p.CreateService(nil)
	assert.NoError(t, err)
	assert.Nil(t, svc)

	assert.NoError(t, p.HealthCheck(nil))
	assert.NoError(t, p.Shutdown(nil))
}

func TestPlugin_Nodes(t *testing.T) {
	p := &Plugin{}
	nodes := p.Nodes()
	require.Len(t, nodes, 4)

	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Descriptor.Name()
	}
	assert.Contains(t, names, "read")
	assert.Contains(t, names, "write")
	assert.Contains(t, names, "delete")
	assert.Contains(t, names, "list")
}
