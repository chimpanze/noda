package upload

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
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
	_, _ = part.Write(content)
	_ = writer.Close()

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

// --- mimeAllowed unit tests ---

func TestMimeAllowed_StarWildcard(t *testing.T) {
	assert.True(t, mimeAllowed("image/png", []string{"*"}))
	assert.True(t, mimeAllowed("text/plain", []string{"*"}))
	assert.True(t, mimeAllowed("application/octet-stream", []string{"*"}))
}

func TestMimeAllowed_PrefixWildcard(t *testing.T) {
	assert.True(t, mimeAllowed("image/png", []string{"image/*"}))
	assert.True(t, mimeAllowed("image/jpeg", []string{"image/*"}))
	assert.False(t, mimeAllowed("text/plain", []string{"image/*"}))
}

func TestMimeAllowed_NoMatch(t *testing.T) {
	assert.False(t, mimeAllowed("text/plain", []string{"image/png", "image/jpeg"}))
	assert.False(t, mimeAllowed("application/json", []string{"text/plain"}))
}

func TestMimeAllowed_EmptyAllowed(t *testing.T) {
	assert.False(t, mimeAllowed("text/plain", nil))
	assert.False(t, mimeAllowed("text/plain", []string{}))
}

func TestMimeAllowed_ExactMatch(t *testing.T) {
	assert.True(t, mimeAllowed("text/plain", []string{"text/plain"}))
}

// --- parseStringSlice unit tests ---

func TestParseStringSlice_Nil(t *testing.T) {
	result := parseStringSlice(nil)
	assert.Nil(t, result)
}

func TestParseStringSlice_NonArrayInput(t *testing.T) {
	assert.Nil(t, parseStringSlice("not an array"))
	assert.Nil(t, parseStringSlice(42))
	assert.Nil(t, parseStringSlice(true))
}

func TestParseStringSlice_ValidArray(t *testing.T) {
	result := parseStringSlice([]any{"image/png", "text/plain"})
	assert.Equal(t, []string{"image/png", "text/plain"}, result)
}

func TestParseStringSlice_MixedArray(t *testing.T) {
	// Non-string items should be skipped
	result := parseStringSlice([]any{"image/png", 42, "text/plain", true})
	assert.Equal(t, []string{"image/png", "text/plain"}, result)
}

func TestParseStringSlice_EmptyArray(t *testing.T) {
	result := parseStringSlice([]any{})
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

// --- limitedReader unit tests ---

func TestLimitedReader_ExactLimit(t *testing.T) {
	data := "hello" // 5 bytes
	lr := &limitedReader{R: strings.NewReader(data), N: 5}
	result, err := io.ReadAll(lr)
	// When exactly at limit, exceeded is set and EOF returned
	assert.NoError(t, err)
	assert.Equal(t, "hello", string(result))
	assert.True(t, lr.exceeded)
}

func TestLimitedReader_UnderLimit(t *testing.T) {
	data := "hi" // 2 bytes
	lr := &limitedReader{R: strings.NewReader(data), N: 10}
	result, err := io.ReadAll(lr)
	assert.NoError(t, err)
	assert.Equal(t, "hi", string(result))
	assert.False(t, lr.exceeded)
}

func TestLimitedReader_OverLimit(t *testing.T) {
	data := "hello world" // 11 bytes
	lr := &limitedReader{R: strings.NewReader(data), N: 5}
	result, err := io.ReadAll(lr)
	assert.NoError(t, err)
	// Should only read up to N bytes then stop
	assert.Equal(t, "hello", string(result))
	assert.True(t, lr.exceeded)
}

func TestLimitedReader_ZeroLimit(t *testing.T) {
	lr := &limitedReader{R: strings.NewReader("anything"), N: 0}
	result, err := io.ReadAll(lr)
	assert.NoError(t, err)
	assert.Empty(t, result)
	assert.True(t, lr.exceeded)
}

// --- Multiple file upload tests ---

func makeMultiFileHeaders(t *testing.T, files []struct {
	name    string
	ctype   string
	content []byte
}) []*multipart.FileHeader {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for _, f := range files {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="files"; filename="`+f.name+`"`)
		h.Set("Content-Type", f.ctype)
		part, err := writer.CreatePart(h)
		require.NoError(t, err)
		_, err = part.Write(f.content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	form, err := multipart.NewReader(body, writer.Boundary()).ReadForm(1 << 20)
	require.NoError(t, err)
	return form.File["files"]
}

func TestUploadHandle_MultipleFiles(t *testing.T) {
	svc := newMemStorage(t)
	fhs := makeMultiFileHeaders(t, []struct {
		name    string
		ctype   string
		content []byte
	}{
		{"a.txt", "text/plain", []byte("file a")},
		{"b.txt", "text/plain", []byte("file b")},
	})
	require.Len(t, fhs, 2)

	ctx := context.Background()
	services := map[string]any{"destination": svc}
	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{"file": fhs}),
	)
	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"*"},
		"max_files":     float64(5),
		"path":          "uploads/multi",
	}

	e := newHandleExecutor(nil)
	output, result, err := e.Execute(ctx, execCtx, config, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	rm := result.(map[string]any)
	filesArr, ok := rm["files"].([]map[string]any)
	require.True(t, ok, "expected files array in result")
	assert.Len(t, filesArr, 2)
	assert.Equal(t, "uploads/multi_0", filesArr[0]["path"])
	assert.Equal(t, "uploads/multi_1", filesArr[1]["path"])
}

func TestUploadHandle_TooManyFiles(t *testing.T) {
	svc := newMemStorage(t)
	fhs := makeMultiFileHeaders(t, []struct {
		name    string
		ctype   string
		content []byte
	}{
		{"a.txt", "text/plain", []byte("a")},
		{"b.txt", "text/plain", []byte("b")},
		{"c.txt", "text/plain", []byte("c")},
	})
	require.Len(t, fhs, 3)

	ctx := context.Background()
	services := map[string]any{"destination": svc}
	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{"file": fhs}),
	)
	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"*"},
		"max_files":     float64(2), // only allow 2
		"path":          "uploads/multi",
	}

	e := newHandleExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, config, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many files")
}

// --- extractFiles edge case tests ---

func TestUploadHandle_NoFileField(t *testing.T) {
	svc := newMemStorage(t)
	ctx := context.Background()
	services := map[string]any{"destination": svc}
	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{"other": "value"}),
	)
	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"*"},
		"path":          "uploads/test",
	}

	e := newHandleExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, config, services)
	require.Error(t, err)
	var ve *api.ValidationError
	assert.ErrorAs(t, err, &ve)
}

func TestUploadHandle_WrongFieldType(t *testing.T) {
	svc := newMemStorage(t)
	ctx := context.Background()
	services := map[string]any{"destination": svc}
	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{"file": "not-a-file"}),
	)
	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"*"},
		"path":          "uploads/test",
	}

	e := newHandleExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, config, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a file upload")
}

func TestUploadHandle_InputNotMap(t *testing.T) {
	svc := newMemStorage(t)
	ctx := context.Background()
	services := map[string]any{"destination": svc}
	execCtx := engine.NewExecutionContext(
		engine.WithInput("not a map"),
	)
	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"*"},
		"path":          "uploads/test",
	}

	e := newHandleExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, config, services)
	require.Error(t, err)
}

func TestUploadHandle_MissingDestinationService(t *testing.T) {
	ctx := context.Background()
	fh := makeFileHeader("test.txt", "text/plain", []byte("data"))
	require.NotNil(t, fh)
	services := map[string]any{} // no destination
	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{"file": fh}),
	)
	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"*"},
		"path":          "uploads/test.txt",
	}

	e := newHandleExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, config, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestUploadHandle_CustomFieldName(t *testing.T) {
	svc := newMemStorage(t)
	fh := makeFileHeader("doc.txt", "text/plain", []byte("custom field"))
	require.NotNil(t, fh)

	ctx := context.Background()
	services := map[string]any{"destination": svc}
	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{"avatar": fh}),
	)
	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{"*"},
		"path":          "uploads/doc.txt",
		"field":         "avatar",
	}

	e := newHandleExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, config, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

// --- Plugin/descriptor unit tests ---

func TestPlugin_Methods(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "core.upload", p.Name())
	assert.Equal(t, "upload", p.Prefix())
	assert.False(t, p.HasServices())

	svc, err := p.CreateService(nil)
	assert.NoError(t, err)
	assert.Nil(t, svc)

	assert.NoError(t, p.HealthCheck(nil))
	assert.NoError(t, p.Shutdown(nil))

	nodes := p.Nodes()
	require.Len(t, nodes, 1)
}

func TestHandleDescriptor(t *testing.T) {
	d := &handleDescriptor{}
	assert.Equal(t, "handle", d.Name())

	deps := d.ServiceDeps()
	require.Contains(t, deps, "destination")
	assert.True(t, deps["destination"].Required)

	schema := d.ConfigSchema()
	assert.NotNil(t, schema)
}

func TestHandleExecutor_Outputs(t *testing.T) {
	e := newHandleExecutor(nil)
	outputs := e.Outputs()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

func TestUploadHandle_DefaultMaxSize(t *testing.T) {
	// When max_size is not in config, default 10MB should be used
	svc := newMemStorage(t)
	fh := makeFileHeader("small.txt", "text/plain", []byte("tiny"))
	require.NotNil(t, fh)

	config := map[string]any{
		"allowed_types": []any{"*"},
		"path":          "uploads/small.txt",
	}

	output, _, err := execUpload(t, svc, fh, config)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestUploadHandle_EmptyAllowedTypes(t *testing.T) {
	// When allowed_types is empty, MIME check is skipped (len(allowedTypes) == 0)
	svc := newMemStorage(t)
	fh := makeFileHeader("any.txt", "text/plain", []byte("anything goes"))
	require.NotNil(t, fh)

	config := map[string]any{
		"max_size":      float64(1024 * 1024),
		"allowed_types": []any{},
		"path":          "uploads/any.txt",
	}

	output, _, err := execUpload(t, svc, fh, config)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}
