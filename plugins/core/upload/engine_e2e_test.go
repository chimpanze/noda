package upload

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadHandle_Engine(t *testing.T) {
	svc := newMemStorage(t) // helper from handle_test.go (same package)
	fh := makeFileHeader("hello.txt", "text/plain", []byte("hello world"))
	require.NotNil(t, fh)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("store", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "upload-wf",
		Nodes: map[string]engine.NodeConfig{
			"up": {
				Type: "upload.handle",
				Config: map[string]any{
					"max_size":      float64(1024 * 1024),
					"allowed_types": []any{"text/plain; charset=utf-8"},
					"path":          "uploads/hello.txt",
				},
				Services: map[string]string{"destination": "store"},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"file": fh}))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))

	out, ok := execCtx.GetOutput("up")
	require.True(t, ok)
	rm := out.(map[string]any)
	assert.Equal(t, "uploads/hello.txt", rm["path"])
	assert.Equal(t, "hello.txt", rm["filename"])

	data, err := svc.Read(context.Background(), "uploads/hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestUploadHandle_Engine_TypeRejected(t *testing.T) {
	svc := newMemStorage(t)
	fh := makeFileHeader("evil.bin", "application/octet-stream", []byte{0x00, 0x01, 0x02})
	require.NotNil(t, fh)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("store", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "upload-wf-err",
		Nodes: map[string]engine.NodeConfig{
			"up": {
				Type: "upload.handle",
				Config: map[string]any{
					"max_size":      float64(1024),
					"allowed_types": []any{"image/png"},
					"path":          "uploads/evil.bin",
				},
				Services: map[string]string{"destination": "store"},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"file": fh}))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err) // disallowed type, no error edge → workflow fails
}
