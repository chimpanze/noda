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
	imageplugin "github.com/chimpanze/noda/plugins/image"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	"github.com/h2non/bimg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newImageTestServer creates a test server with storage and image plugins.
func newImageTestServer(t *testing.T, routes map[string]map[string]any, workflows map[string]map[string]any) (*Server, *storageplugin.Service) {
	t.Helper()

	sp := &storageplugin.Plugin{}
	rawSvc, err := sp.CreateService(map[string]any{"backend": "memory"})
	require.NoError(t, err)
	svc := rawSvc.(*storageplugin.Service)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("main-storage", rawSvc, sp))

	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&corestorage.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&upload.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&imageplugin.Plugin{})

	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Routes:    routes,
		Workflows: workflows,
		Schemas:   map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	return srv, svc
}

// makeTestPNG creates a 200x200 PNG for testing.
func makeTestPNG(t *testing.T) []byte {
	t.Helper()
	base := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc,
		0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
	buf, err := bimg.NewImage(base).Process(bimg.Options{
		Width: 200, Height: 200, Enlarge: true,
	})
	require.NoError(t, err)
	return buf
}

// TestE2E_UploadAndResize tests the full pipeline: upload → resize → read thumbnail.
func TestE2E_UploadAndResize(t *testing.T) {
	srv, svc := newImageTestServer(t,
		map[string]map[string]any{
			"upload-and-resize": {
				"method": "POST",
				"path":   "/api/images",
				"trigger": map[string]any{
					"workflow": "upload-resize-wf",
					"files":    []any{"file"},
					"input": map[string]any{
						"file": "file",
					},
				},
			},
		},
		map[string]map[string]any{
			"upload-resize-wf": {
				"nodes": map[string]any{
					"upload": map[string]any{
						"type":     "upload.handle",
						"services": map[string]any{"destination": "main-storage"},
						"config": map[string]any{
							"max_size":      float64(10 * 1024 * 1024),
							"allowed_types": []any{"image/png", "application/octet-stream"},
							"path":          "originals/photo.png",
						},
					},
					"resize": map[string]any{
						"type": "image.resize",
						"services": map[string]any{
							"source": "main-storage",
							"target": "main-storage",
						},
						"config": map[string]any{
							"input":  "originals/photo.png",
							"output": "thumbnails/photo_100x100.png",
							"width":  float64(100),
							"height": float64(100),
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.resize }}"},
					},
				},
				"edges": []any{
					map[string]any{"from": "upload", "to": "resize"},
					map[string]any{"from": "resize", "to": "respond"},
				},
			},
		},
	)

	// Build multipart request with a real PNG image
	img := makeTestPNG(t)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="photo.png"`)
	h.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write(img)
	require.NoError(t, err)
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/api/images", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, "thumbnails/photo_100x100.png", result["path"])

	// Verify thumbnail exists and has correct dimensions
	thumbData, err := svc.Read(context.Background(), "thumbnails/photo_100x100.png")
	require.NoError(t, err)
	sz, err := bimg.NewImage(thumbData).Size()
	require.NoError(t, err)
	assert.LessOrEqual(t, sz.Width, 100)
	assert.LessOrEqual(t, sz.Height, 100)
}

// TestE2E_ConvertPNGtoWEBP tests format conversion via API.
func TestE2E_ConvertPNGtoWEBP(t *testing.T) {
	srv, svc := newImageTestServer(t,
		map[string]map[string]any{
			"convert-image": {
				"method": "POST",
				"path":   "/api/convert",
				"trigger": map[string]any{
					"workflow": "convert-wf",
					"files":    []any{"file"},
					"input": map[string]any{
						"file": "file",
					},
				},
			},
		},
		map[string]map[string]any{
			"convert-wf": {
				"nodes": map[string]any{
					"upload": map[string]any{
						"type":     "upload.handle",
						"services": map[string]any{"destination": "main-storage"},
						"config": map[string]any{
							"max_size":      float64(10 * 1024 * 1024),
							"allowed_types": []any{"image/png", "application/octet-stream"},
							"path":          "uploads/original.png",
						},
					},
					"convert": map[string]any{
						"type": "image.convert",
						"services": map[string]any{
							"source": "main-storage",
							"target": "main-storage",
						},
						"config": map[string]any{
							"input":  "uploads/original.png",
							"output": "converted/output.webp",
							"format": "webp",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.convert }}"},
					},
				},
				"edges": []any{
					map[string]any{"from": "upload", "to": "convert"},
					map[string]any{"from": "convert", "to": "respond"},
				},
			},
		},
	)

	img := makeTestPNG(t)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="test.png"`)
	h.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write(img)
	require.NoError(t, err)
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/api/convert", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, "converted/output.webp", result["path"])

	// Verify WEBP format
	webpData, err := svc.Read(context.Background(), "converted/output.webp")
	require.NoError(t, err)
	assert.Equal(t, bimg.WEBP, bimg.DetermineImageType(webpData))
}
