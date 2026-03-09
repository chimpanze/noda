package server

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCacheTestServer(t *testing.T, routes map[string]map[string]any, workflows map[string]map[string]any) *Server {
	t.Helper()

	mr := miniredis.RunT(t)
	p := &cacheplugin.Plugin{}
	rawSvc, err := p.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)

	svcReg := registry.NewServiceRegistry()
	err = svcReg.Register("main-cache", rawSvc, p)
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Routes:    routes,
		Workflows: workflows,
		Schemas:   map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	return srv
}

// --- E2E: Set a cache key and get it back ---

func TestE2E_Cache_SetAndGet(t *testing.T) {
	srv := newCacheTestServer(t,
		map[string]map[string]any{
			"cache-set": {
				"method": "POST",
				"path":   "/api/cache",
				"trigger": map[string]any{
					"workflow": "cache-set",
					"input": map[string]any{
						"key":   "{{ body.key }}",
						"value": "{{ body.value }}",
					},
				},
			},
			"cache-get": {
				"method": "GET",
				"path":   "/api/cache/:key",
				"trigger": map[string]any{
					"workflow": "cache-get",
					"input": map[string]any{
						"key": "{{ params.key }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"cache-set": {
				"nodes": map[string]any{
					"set": map[string]any{
						"type":     "cache.set",
						"services": map[string]any{"cache": "main-cache"},
						"config": map[string]any{
							"key":   "{{ input.key }}",
							"value": "{{ input.value }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.set }}"},
					},
				},
				"edges": []any{map[string]any{"from": "set", "to": "respond"}},
			},
			"cache-get": {
				"nodes": map[string]any{
					"get": map[string]any{
						"type":     "cache.get",
						"services": map[string]any{"cache": "main-cache"},
						"config": map[string]any{
							"key": "{{ input.key }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.get }}"},
					},
				},
				"edges": []any{map[string]any{"from": "get", "to": "respond"}},
			},
		},
	)

	// Set a value
	body := `{"key": "greeting", "value": "hello world"}`
	req := httptest.NewRequest("POST", "/api/cache", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var setResult map[string]any
	require.NoError(t, json.Unmarshal(respBody, &setResult))
	assert.Equal(t, true, setResult["ok"])

	// Get the value back
	req = httptest.NewRequest("GET", "/api/cache/greeting", nil)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ = io.ReadAll(resp.Body)
	var getResult map[string]any
	require.NoError(t, json.Unmarshal(respBody, &getResult))
	assert.Equal(t, "hello world", getResult["value"])
}

// --- E2E: Delete a cache key ---

func TestE2E_Cache_Delete(t *testing.T) {
	srv := newCacheTestServer(t,
		map[string]map[string]any{
			"cache-set": {
				"method": "POST",
				"path":   "/api/cache",
				"trigger": map[string]any{
					"workflow": "cache-set",
					"input": map[string]any{
						"key":   "{{ body.key }}",
						"value": "{{ body.value }}",
					},
				},
			},
			"cache-del": {
				"method": "DELETE",
				"path":   "/api/cache/:key",
				"trigger": map[string]any{
					"workflow": "cache-del",
					"input": map[string]any{
						"key": "{{ params.key }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"cache-set": {
				"nodes": map[string]any{
					"set": map[string]any{
						"type":     "cache.set",
						"services": map[string]any{"cache": "main-cache"},
						"config": map[string]any{
							"key":   "{{ input.key }}",
							"value": "{{ input.value }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.set }}"},
					},
				},
				"edges": []any{map[string]any{"from": "set", "to": "respond"}},
			},
			"cache-del": {
				"nodes": map[string]any{
					"del": map[string]any{
						"type":     "cache.del",
						"services": map[string]any{"cache": "main-cache"},
						"config": map[string]any{
							"key": "{{ input.key }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.del }}"},
					},
				},
				"edges": []any{map[string]any{"from": "del", "to": "respond"}},
			},
		},
	)

	// Set a value first
	body := `{"key": "temp", "value": "temporary"}`
	req := httptest.NewRequest("POST", "/api/cache", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Delete it
	req = httptest.NewRequest("DELETE", "/api/cache/temp", nil)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, true, result["ok"])
}

// --- E2E: Check if key exists ---

func TestE2E_Cache_Exists(t *testing.T) {
	srv := newCacheTestServer(t,
		map[string]map[string]any{
			"cache-set": {
				"method": "POST",
				"path":   "/api/cache",
				"trigger": map[string]any{
					"workflow": "cache-set",
					"input": map[string]any{
						"key":   "{{ body.key }}",
						"value": "{{ body.value }}",
					},
				},
			},
			"cache-exists": {
				"method": "GET",
				"path":   "/api/cache/:key/exists",
				"trigger": map[string]any{
					"workflow": "cache-exists",
					"input": map[string]any{
						"key": "{{ params.key }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"cache-set": {
				"nodes": map[string]any{
					"set": map[string]any{
						"type":     "cache.set",
						"services": map[string]any{"cache": "main-cache"},
						"config": map[string]any{
							"key":   "{{ input.key }}",
							"value": "{{ input.value }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.set }}"},
					},
				},
				"edges": []any{map[string]any{"from": "set", "to": "respond"}},
			},
			"cache-exists": {
				"nodes": map[string]any{
					"check": map[string]any{
						"type":     "cache.exists",
						"services": map[string]any{"cache": "main-cache"},
						"config": map[string]any{
							"key": "{{ input.key }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.check }}"},
					},
				},
				"edges": []any{map[string]any{"from": "check", "to": "respond"}},
			},
		},
	)

	// Check non-existent key
	req := httptest.NewRequest("GET", "/api/cache/nokey/exists", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, false, result["exists"])

	// Set a key
	body := `{"key": "present", "value": "yes"}`
	req = httptest.NewRequest("POST", "/api/cache", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.App().Test(req)

	// Check it exists now
	req = httptest.NewRequest("GET", "/api/cache/present/exists", nil)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ = io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, true, result["exists"])
}

// --- E2E: Set with TTL ---

func TestE2E_Cache_SetWithTTL(t *testing.T) {
	srv := newCacheTestServer(t,
		map[string]map[string]any{
			"cache-set-ttl": {
				"method": "POST",
				"path":   "/api/cache/ttl",
				"trigger": map[string]any{
					"workflow": "cache-set-ttl",
					"input": map[string]any{
						"key":   "{{ body.key }}",
						"value": "{{ body.value }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"cache-set-ttl": {
				"nodes": map[string]any{
					"set": map[string]any{
						"type":     "cache.set",
						"services": map[string]any{"cache": "main-cache"},
						"config": map[string]any{
							"key":   "{{ input.key }}",
							"value": "{{ input.value }}",
							"ttl":   float64(300),
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.set }}"},
					},
				},
				"edges": []any{map[string]any{"from": "set", "to": "respond"}},
			},
		},
	)

	body := `{"key": "session", "value": "abc123"}`
	req := httptest.NewRequest("POST", "/api/cache/ttl", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, true, result["ok"])
}
