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
