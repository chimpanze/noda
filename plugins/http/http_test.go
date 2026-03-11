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
