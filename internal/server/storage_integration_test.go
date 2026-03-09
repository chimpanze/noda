package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	corestorage "github.com/chimpanze/noda/plugins/core/storage"
	"github.com/chimpanze/noda/plugins/core/upload"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newStorageTestServer creates a server with a memory storage service registered.
func newStorageTestServer(t *testing.T, routes map[string]map[string]any, workflows map[string]map[string]any) (*Server, *storageplugin.Service) {
	t.Helper()

	sp := &storageplugin.Plugin{}
	rawSvc, err := sp.CreateService(map[string]any{"backend": "memory"})
	require.NoError(t, err)
	storageSvc := rawSvc.(*storageplugin.Service)

	svcReg := registry.NewServiceRegistry()
	err = svcReg.Register("main-storage", rawSvc, sp)
	require.NoError(t, err)

	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&corestorage.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&upload.Plugin{})

	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Routes:    routes,
		Workflows: workflows,
		Schemas:   map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	return srv, storageSvc
}

// buildMultipartBody creates a multipart/form-data body with a single file.
func buildMultipartBody(t *testing.T, fieldName, filename, contentType string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+filename+`"`)
	h.Set("Content-Type", contentType)
	part, err := writer.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	writer.Close()

	return body, writer.FormDataContentType()
}

// TestE2E_UploadHandle_StoresFile tests: HTTP file upload → upload.handle → file exists in storage.
func TestE2E_UploadHandle_StoresFile(t *testing.T) {
	srv, storageSvc := newStorageTestServer(t,
		map[string]map[string]any{
			"upload-file": {
				"method": "POST",
				"path":   "/api/upload",
				"trigger": map[string]any{
					"workflow": "upload-wf",
					"files":    []any{"file"},
					"input": map[string]any{
						"file": "file", // file field (not an expression — handled as file)
					},
				},
			},
		},
		map[string]map[string]any{
			"upload-wf": {
				"nodes": map[string]any{
					"handle": map[string]any{
						"type":     "upload.handle",
						"services": map[string]any{"destination": "main-storage"},
						"config": map[string]any{
							"max_size":      float64(10 * 1024 * 1024),
							"allowed_types": []any{"text/plain; charset=utf-8", "text/plain"},
							"path":          "uploads/test.txt",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ handle }}"},
					},
				},
				"edges": []any{
					map[string]any{"from": "handle", "to": "respond"},
				},
			},
		},
	)

	content := []byte("hello from upload test")
	body, contentType := buildMultipartBody(t, "file", "test.txt", "text/plain", content)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)

	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, "uploads/test.txt", result["path"])
	assert.Equal(t, "test.txt", result["filename"])

	// Verify the file is actually in storage
	data, err := storageSvc.Read(context.Background(), "uploads/test.txt")
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

// TestE2E_UploadHandle_ValidationRejection_WrongMIME tests that a disallowed MIME type
// routes to the error output and returns a 422 via response.error.
func TestE2E_UploadHandle_ValidationRejection_WrongMIME(t *testing.T) {
	srv, storageSvc := newStorageTestServer(t,
		map[string]map[string]any{
			"upload-file": {
				"method": "POST",
				"path":   "/api/upload",
				"trigger": map[string]any{
					"workflow": "upload-wf",
					"files":    []any{"file"},
					"input": map[string]any{
						"file": "file",
					},
				},
			},
		},
		map[string]map[string]any{
			"upload-wf": {
				"nodes": map[string]any{
					"handle": map[string]any{
						"type":     "upload.handle",
						"services": map[string]any{"destination": "main-storage"},
						"config": map[string]any{
							"max_size":      float64(10 * 1024 * 1024),
							"allowed_types": []any{"image/png", "image/jpeg"},
							"path":          "uploads/rejected.txt",
						},
					},
					"err-resp": map[string]any{
						"type": "response.error",
						"config": map[string]any{
							"status":  "422",
							"message": "{{ handle.error }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "handle", "to": "err-resp", "output": "error"},
				},
			},
		},
	)

	// Send a plain text file — should be rejected (not an image)
	content := []byte("this is plain text content")
	body, contentType := buildMultipartBody(t, "file", "text.txt", "text/plain", content)

	req := httptest.NewRequest("POST", "/api/upload", body)
	req.Header.Set("Content-Type", contentType)

	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode)

	// Verify file was NOT stored
	_, readErr := storageSvc.Read(context.Background(), "uploads/rejected.txt")
	assert.Error(t, readErr, "file should not have been stored on rejection")
}

// TestE2E_StorageWriteRead tests the storage.write → storage.read pipeline in a workflow.
func TestE2E_StorageWriteRead(t *testing.T) {
	srv, storageSvc := newStorageTestServer(t,
		map[string]map[string]any{
			"store-data": {
				"method": "POST",
				"path":   "/api/store",
				"trigger": map[string]any{
					"workflow": "store-wf",
					"input": map[string]any{
						"content": "{{ request.body.content }}",
						"path":    "{{ request.body.path }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"store-wf": {
				"nodes": map[string]any{
					"write": map[string]any{
						"type":     "storage.write",
						"services": map[string]any{"storage": "main-storage"},
						"config": map[string]any{
							"path": "{{ input.path }}",
							"data": "{{ input.content }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": map[string]any{"ok": true}},
					},
				},
				"edges": []any{
					map[string]any{"from": "write", "to": "respond"},
				},
			},
		},
	)

	body := bytes.NewBufferString(`{"content": "stored data", "path": "data/record.txt"}`)
	req := httptest.NewRequest("POST", "/api/store", body)
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify data is in storage
	data, err := storageSvc.Read(context.Background(), "data/record.txt")
	require.NoError(t, err)
	assert.Equal(t, "stored data", string(data))
}
