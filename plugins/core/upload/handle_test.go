package upload

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/textproto"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMemStorage(t *testing.T) *storageplugin.Service {
	t.Helper()
	p := &storageplugin.Plugin{}
	rawSvc, err := p.CreateService(map[string]any{"backend": "memory"})
	require.NoError(t, err)
	return rawSvc.(*storageplugin.Service)
}

// makeFileHeader creates a fake *multipart.FileHeader for testing.
func makeFileHeader(filename string, contentType string, content []byte) *multipart.FileHeader {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", contentType)

	part, _ := writer.CreatePart(h)
	part.Write(content)
	writer.Close()

	boundary := writer.Boundary()
	form, _ := multipart.NewReader(body, boundary).ReadForm(int64(len(content)) + 4096)
	files := form.File["file"]
	if len(files) > 0 {
		return files[0]
	}
	return nil
}

func execUpload(t *testing.T, svc *storageplugin.Service, fh *multipart.FileHeader, config map[string]any) (string, any, error) {
	t.Helper()
	ctx := context.Background()
	services := map[string]any{"destination": svc}
	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{"file": fh}),
	)
	e := newHandleExecutor(nil)
	return e.Execute(ctx, execCtx, config, services)
}

func TestUploadHandle_ValidFile(t *testing.T) {
	svc := newMemStorage(t)
	fh := makeFileHeader("hello.txt", "text/plain", []byte("hello world"))
	require.NotNil(t, fh)

	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"text/plain; charset=utf-8"},
		"path":          "uploads/hello.txt",
	}

	output, result, err := execUpload(t, svc, fh, config)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	rm := result.(map[string]any)
	assert.Equal(t, "uploads/hello.txt", rm["path"])
	assert.Equal(t, "hello.txt", rm["filename"])
	assert.NotEmpty(t, rm["content_type"])

	// Verify data stored
	ctx := context.Background()
	data, err := svc.Read(ctx, "uploads/hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestUploadHandle_OversizedFile(t *testing.T) {
	svc := newMemStorage(t)
	content := make([]byte, 2048)
	fh := makeFileHeader("big.bin", "application/octet-stream", content)
	require.NotNil(t, fh)

	config := map[string]any{
		"max_size":      float64(100), // only 100 bytes allowed
		"allowed_types": []any{"application/octet-stream"},
		"path":          "uploads/big.bin",
	}

	_, _, err := execUpload(t, svc, fh, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds max size")
}

func TestUploadHandle_WrongMIMEType(t *testing.T) {
	svc := newMemStorage(t)
	// Create content that will be detected as plain text
	fh := makeFileHeader("hack.txt", "text/plain", []byte("plain text content here"))
	require.NotNil(t, fh)

	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"image/png", "image/jpeg"},
		"path":          "uploads/hack.txt",
	}

	_, _, err := execUpload(t, svc, fh, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestUploadHandle_AllowedWildcard(t *testing.T) {
	svc := newMemStorage(t)
	fh := makeFileHeader("doc.txt", "text/plain", []byte("some text"))
	require.NotNil(t, fh)

	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"text/*"},
		"path":          "uploads/doc.txt",
	}

	output, _, err := execUpload(t, svc, fh, config)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestUploadHandle_PathExpression(t *testing.T) {
	svc := newMemStorage(t)
	fh := makeFileHeader("avatar.png", "image/png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) // PNG header
	require.NotNil(t, fh)

	ctx := context.Background()
	services := map[string]any{"destination": svc}
	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{
			"file":   fh,
			"userID": "user123",
		}),
	)
	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"image/png", "application/octet-stream"},
		"path":          "avatars/{{ input.userID }}/avatar.png",
	}

	e := newHandleExecutor(nil)
	output, result, err := e.Execute(ctx, execCtx, config, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	rm := result.(map[string]any)
	assert.Equal(t, "avatars/user123/avatar.png", rm["path"])
}
