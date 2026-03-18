package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService() *Service {
	return &Service{
		client:         &http.Client{Timeout: 10 * time.Second},
		defaultTimeout: 10 * time.Second,
	}
}

func newTestServiceWithHeaders(headers map[string]string) *Service {
	return &Service{
		client:         &http.Client{Timeout: 10 * time.Second},
		defaultHeaders: headers,
		defaultTimeout: 10 * time.Second,
	}
}

func TestGET_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "hello"})
	}))
	defer ts.Close()

	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newGetExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url": ts.URL,
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	data := result.(map[string]any)
	assert.Equal(t, 200, data["status"])
	body := data["body"].(map[string]any)
	assert.Equal(t, "hello", body["message"])
}

func TestPOST_JSONBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		bodyBytes, _ := io.ReadAll(r.Body)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(bodyBytes, &payload))
		assert.Equal(t, "bar", payload["foo"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"received": true})
	}))
	defer ts.Close()

	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"payload": map[string]any{"foo": "bar"},
	}))

	e := newPostExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url":  ts.URL,
		"body": "{{ input.payload }}",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	data := result.(map[string]any)
	assert.Equal(t, 200, data["status"])
	body := data["body"].(map[string]any)
	assert.Equal(t, true, body["received"])
}

func TestCustomHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "my-token", r.Header.Get("X-Api-Key"))
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"token": "my-token",
	}))

	e := newGetExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url": ts.URL,
		"headers": map[string]any{
			"X-Api-Key": "{{ input.token }}",
		},
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestDefaultHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer xyz", r.Header.Get("Authorization"))
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := newTestServiceWithHeaders(map[string]string{
		"Authorization": "Bearer xyz",
	})
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newGetExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url": ts.URL,
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newGetExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url":     ts.URL,
		"timeout": "50ms",
	}, services)
	require.Error(t, err)
	// Should be a TimeoutError
	_, ok := err.(*api.TimeoutError)
	assert.True(t, ok, "expected TimeoutError, got %T: %v", err, err)
}

func TestNon200_StillSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte("not found"))
	}))
	defer ts.Close()

	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newGetExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url": ts.URL,
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	data := result.(map[string]any)
	assert.Equal(t, 404, data["status"])
	assert.Equal(t, "not found", data["body"])
}

func TestConnectionError(t *testing.T) {
	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newGetExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url": "http://127.0.0.1:1", // should fail to connect
	}, services)
	require.Error(t, err)
}

func TestRequest_AllMethods(t *testing.T) {
	for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH"} {
		t.Run(method, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, method, r.Method)
				w.WriteHeader(200)
			}))
			defer ts.Close()

			svc := newTestService()
			services := map[string]any{"client": svc}
			execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

			e := newRequestExecutor(nil)
			output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
				"method": method,
				"url":    ts.URL,
			}, services)
			require.NoError(t, err)
			assert.Equal(t, "success", output)
		})
	}
}

func TestBaseURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/users", r.URL.Path)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := &Service{
		client:         &http.Client{Timeout: 10 * time.Second},
		baseURL:        ts.URL,
		defaultTimeout: 10 * time.Second,
	}
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newGetExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url": "/api/users",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestStringResponseBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newGetExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url": ts.URL,
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	data := result.(map[string]any)
	assert.Equal(t, "hello world", data["body"])
}

// --- Plugin metadata tests ---

func TestPlugin_Name(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "http", p.Name())
}

func TestPlugin_Prefix(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "http", p.Prefix())
}

func TestPlugin_HasServices(t *testing.T) {
	p := &Plugin{}
	assert.True(t, p.HasServices())
}

func TestPlugin_Nodes(t *testing.T) {
	p := &Plugin{}
	nodes := p.Nodes()
	require.Len(t, nodes, 3)
	assert.Equal(t, "request", nodes[0].Descriptor.Name())
	assert.Equal(t, "get", nodes[1].Descriptor.Name())
	assert.Equal(t, "post", nodes[2].Descriptor.Name())
}

// --- CreateService tests ---

func TestCreateService_DefaultConfig(t *testing.T) {
	p := &Plugin{}
	svcAny, err := p.CreateService(map[string]any{})
	require.NoError(t, err)
	svc, ok := svcAny.(*Service)
	require.True(t, ok)
	assert.Equal(t, 30*time.Second, svc.defaultTimeout)
	assert.Empty(t, svc.baseURL)
	assert.Nil(t, svc.defaultHeaders)
}

func TestCreateService_WithStringTimeout(t *testing.T) {
	p := &Plugin{}
	svcAny, err := p.CreateService(map[string]any{
		"timeout": "5s",
	})
	require.NoError(t, err)
	svc := svcAny.(*Service)
	assert.Equal(t, 5*time.Second, svc.defaultTimeout)
}

func TestCreateService_WithFloat64Timeout(t *testing.T) {
	p := &Plugin{}
	svcAny, err := p.CreateService(map[string]any{
		"timeout": float64(15),
	})
	require.NoError(t, err)
	svc := svcAny.(*Service)
	assert.Equal(t, 15*time.Second, svc.defaultTimeout)
}

func TestCreateService_WithInvalidTimeout(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{
		"timeout": "not-a-duration",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timeout")
}

func TestCreateService_WithBaseURL(t *testing.T) {
	p := &Plugin{}
	svcAny, err := p.CreateService(map[string]any{
		"base_url": "https://api.example.com",
	})
	require.NoError(t, err)
	svc := svcAny.(*Service)
	assert.Equal(t, "https://api.example.com", svc.baseURL)
}

func TestCreateService_WithHeaders(t *testing.T) {
	p := &Plugin{}
	svcAny, err := p.CreateService(map[string]any{
		"headers": map[string]any{
			"Authorization": "Bearer token123",
			"X-Custom":      "value",
			"non-string":    42, // should be skipped
		},
	})
	require.NoError(t, err)
	svc := svcAny.(*Service)
	require.Len(t, svc.defaultHeaders, 2)
	assert.Equal(t, "Bearer token123", svc.defaultHeaders["Authorization"])
	assert.Equal(t, "value", svc.defaultHeaders["X-Custom"])
}

// --- HealthCheck tests ---

func TestHealthCheck_ValidService(t *testing.T) {
	p := &Plugin{}
	svc := newTestService()
	err := p.HealthCheck(svc)
	assert.NoError(t, err)
}

func TestHealthCheck_InvalidService(t *testing.T) {
	p := &Plugin{}
	err := p.HealthCheck("not a service")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid service type")
}

// --- Shutdown tests ---

func TestShutdown_ValidService(t *testing.T) {
	p := &Plugin{}
	svc := newTestService()
	err := p.Shutdown(svc)
	assert.NoError(t, err)
}

func TestShutdown_InvalidService(t *testing.T) {
	p := &Plugin{}
	err := p.Shutdown("not a service")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid service type")
}

// --- Descriptor tests ---

func TestRequestDescriptor(t *testing.T) {
	d := &requestDescriptor{}
	assert.Equal(t, "request", d.Name())
	assert.NotNil(t, d.ServiceDeps())
	assert.Contains(t, d.ServiceDeps(), "client")
	schema := d.ConfigSchema()
	assert.Equal(t, "object", schema["type"])
}

func TestGetDescriptor(t *testing.T) {
	d := &getDescriptor{}
	assert.Equal(t, "get", d.Name())
	assert.NotNil(t, d.ServiceDeps())
	schema := d.ConfigSchema()
	assert.Equal(t, "object", schema["type"])
}

func TestPostDescriptor(t *testing.T) {
	d := &postDescriptor{}
	assert.Equal(t, "post", d.Name())
	assert.NotNil(t, d.ServiceDeps())
	schema := d.ConfigSchema()
	assert.Equal(t, "object", schema["type"])
}

// --- Executor Outputs tests ---

func TestExecutor_Outputs(t *testing.T) {
	re := newRequestExecutor(nil)
	assert.NotEmpty(t, re.Outputs())

	ge := newGetExecutor(nil)
	assert.NotEmpty(t, ge.Outputs())

	pe := newPostExecutor(nil)
	assert.NotEmpty(t, pe.Outputs())
}

// --- Request body type tests ---

func TestRequest_StringBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		assert.Equal(t, "raw string body", string(bodyBytes))
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"data": "raw string body",
	}))

	e := newPostExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url":  ts.URL,
		"body": "{{ input.data }}",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestRequest_ByteSliceBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		assert.Equal(t, []byte("byte data"), bodyBytes)
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"raw": []byte("byte data"),
	}))

	e := newPostExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url":  ts.URL,
		"body": "{{ input.raw }}",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestRequest_InvalidTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newGetExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url":     ts.URL,
		"timeout": "not-valid",
	}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timeout")
}

// --- Multi-value response headers ---

func TestMultiValueResponseHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Multi", "val1")
		w.Header().Add("X-Multi", "val2")
		w.Header().Set("X-Single", "only")
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newGetExecutor(nil)
	_, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url": ts.URL,
	}, services)
	require.NoError(t, err)
	data := result.(map[string]any)
	headers := data["headers"].(map[string]any)

	// Multi-value header should be a slice
	multi, ok := headers["x-multi"].([]string)
	require.True(t, ok, "expected []string for multi-value header, got %T", headers["x-multi"])
	assert.Equal(t, []string{"val1", "val2"}, multi)

	// Single-value header should be a string
	single, ok := headers["x-single"].(string)
	require.True(t, ok)
	assert.Equal(t, "only", single)
}

// --- Missing service tests ---

func TestGetExecutor_MissingService(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newGetExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url": "http://example.com",
	}, map[string]any{})
	require.Error(t, err)
}

func TestPostExecutor_MissingService(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newPostExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url":  "http://example.com",
		"body": "test",
	}, map[string]any{})
	require.Error(t, err)
}

func TestRequestExecutor_MissingService(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newRequestExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"method": "GET",
		"url":    "http://example.com",
	}, map[string]any{})
	require.Error(t, err)
}

func TestRequestExecutor_MethodNotEvaluatedAsExpression(t *testing.T) {
	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"method": "DELETE"}))

	// Method contains expression syntax but should be treated as a static string.
	// Previously this would resolve to "DELETE" via expression evaluation.
	// Now it should be used literally, resulting in an invalid HTTP method error.
	e := newRequestExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"method": "{{ input.method }}",
		"url":    "http://example.com",
	}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid method")
}

func TestRequestExecutor_MissingMethod(t *testing.T) {
	svc := newTestService()
	services := map[string]any{"client": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newRequestExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"url": "http://example.com",
	}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field \"method\"")
}
