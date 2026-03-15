package api_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock implementations ---

type mockPlugin struct{}

func (m *mockPlugin) Name() string                                { return "test" }
func (m *mockPlugin) Prefix() string                              { return "test" }
func (m *mockPlugin) Nodes() []api.NodeRegistration               { return nil }
func (m *mockPlugin) HasServices() bool                           { return false }
func (m *mockPlugin) CreateService(_ map[string]any) (any, error) { return nil, nil }
func (m *mockPlugin) HealthCheck(_ any) error                     { return nil }
func (m *mockPlugin) Shutdown(_ any) error                        { return nil }

type mockDescriptor struct{}

func (m *mockDescriptor) Name() string                           { return "test.node" }
func (m *mockDescriptor) Description() string                    { return "" }
func (m *mockDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (m *mockDescriptor) ConfigSchema() map[string]any           { return nil }
func (m *mockDescriptor) OutputDescriptions() map[string]string  { return nil }

type mockExecutor struct{}

func (m *mockExecutor) Outputs() []string { return []string{"ok", "error"} }
func (m *mockExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return "ok", map[string]any{"result": true}, nil
}

type mockExecutionContext struct{}

func (m *mockExecutionContext) Input() any          { return map[string]any{"key": "value"} }
func (m *mockExecutionContext) Auth() *api.AuthData { return nil }
func (m *mockExecutionContext) Trigger() api.TriggerData {
	return api.TriggerData{Type: "http", Timestamp: time.Now(), TraceID: "abc"}
}
func (m *mockExecutionContext) Resolve(_ string) (any, error) { return "resolved", nil }
func (m *mockExecutionContext) ResolveWithVars(_ string, _ map[string]any) (any, error) {
	return "resolved", nil
}
func (m *mockExecutionContext) Log(_ string, _ string, _ map[string]any) {}

type mockStorageService struct{}

func (m *mockStorageService) Read(_ context.Context, _ string) ([]byte, error)   { return nil, nil }
func (m *mockStorageService) Write(_ context.Context, _ string, _ []byte) error  { return nil }
func (m *mockStorageService) Delete(_ context.Context, _ string) error           { return nil }
func (m *mockStorageService) List(_ context.Context, _ string) ([]string, error) { return nil, nil }

type mockCacheService struct{}

func (m *mockCacheService) Get(_ context.Context, _ string) (any, error)        { return nil, nil }
func (m *mockCacheService) Set(_ context.Context, _ string, _ any, _ int) error { return nil }
func (m *mockCacheService) Del(_ context.Context, _ string) error               { return nil }
func (m *mockCacheService) Exists(_ context.Context, _ string) (bool, error)    { return false, nil }

type mockConnectionService struct{}

func (m *mockConnectionService) Send(_ context.Context, _ string, _ any) error { return nil }
func (m *mockConnectionService) SendSSE(_ context.Context, _ string, _ string, _ any, _ string) error {
	return nil
}

// --- Tests ---

func TestPluginInterface(t *testing.T) {
	var p api.Plugin = &mockPlugin{}
	assert.Equal(t, "test", p.Name())
	assert.Equal(t, "test", p.Prefix())
	assert.Nil(t, p.Nodes())
	assert.False(t, p.HasServices())
}

func TestNodeDescriptorInterface(t *testing.T) {
	var d api.NodeDescriptor = &mockDescriptor{}
	assert.Equal(t, "test.node", d.Name())
	assert.Nil(t, d.ServiceDeps())
	assert.Nil(t, d.ConfigSchema())
}

func TestNodeRegistration(t *testing.T) {
	reg := api.NodeRegistration{
		Descriptor: &mockDescriptor{},
		Factory: func(_ map[string]any) api.NodeExecutor {
			return &mockExecutor{}
		},
	}
	executor := reg.Factory(nil)
	assert.Equal(t, []string{"ok", "error"}, executor.Outputs())
}

func TestNodeExecutor(t *testing.T) {
	var exec api.NodeExecutor = &mockExecutor{}
	ctx := context.Background()
	nCtx := &mockExecutionContext{}

	output, data, err := exec.Execute(ctx, nCtx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", output)
	assert.Equal(t, map[string]any{"result": true}, data)
}

func TestExecutionContext(t *testing.T) {
	var nCtx api.ExecutionContext = &mockExecutionContext{}

	assert.NotNil(t, nCtx.Input())
	assert.Nil(t, nCtx.Auth())
	assert.Equal(t, "http", nCtx.Trigger().Type)

	val, err := nCtx.Resolve("$.input.key")
	require.NoError(t, err)
	assert.Equal(t, "resolved", val)

	// Log should not panic
	nCtx.Log("info", "test message", map[string]any{"key": "value"})
}

func TestHTTPResponse(t *testing.T) {
	resp := api.HTTPResponse{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Cookies: []api.Cookie{
			{Name: "session", Value: "abc", HTTPOnly: true, SameSite: "Strict"},
		},
		Body: map[string]any{"ok": true},
	}
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, "application/json", resp.Headers["Content-Type"])
	assert.Len(t, resp.Cookies, 1)
	assert.Equal(t, "session", resp.Cookies[0].Name)
	assert.True(t, resp.Cookies[0].HTTPOnly)
}

func TestServiceInterfaces(t *testing.T) {
	var _ api.StorageService = &mockStorageService{}
	var _ api.CacheService = &mockCacheService{}
	var _ api.ConnectionService = &mockConnectionService{}
}

func TestErrors(t *testing.T) {
	cause := errors.New("connection refused")

	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{
			"ServiceUnavailableError",
			&api.ServiceUnavailableError{Service: "postgres", Cause: cause},
			"service unavailable: postgres: connection refused",
		},
		{
			"ValidationError",
			&api.ValidationError{Field: "email", Message: "invalid format", Value: "bad"},
			`validation error on field "email": invalid format`,
		},
		{
			"TimeoutError",
			&api.TimeoutError{Duration: 5 * time.Second, Operation: "db query"},
			"timeout after 5s: db query",
		},
		{
			"NotFoundError",
			&api.NotFoundError{Resource: "user", ID: "123"},
			"user not found: 123",
		},
		{
			"ConflictError",
			&api.ConflictError{Resource: "order", Reason: "already processed"},
			"conflict on order: already processed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.msg, tt.err.Error())
			assert.Implements(t, (*error)(nil), tt.err)
		})
	}
}

func TestServiceUnavailableErrorUnwrap(t *testing.T) {
	cause := errors.New("connection refused")
	err := &api.ServiceUnavailableError{Service: "redis", Cause: cause}
	assert.True(t, errors.Is(err, cause))
}

func TestErrorTypeAssertions(t *testing.T) {
	var err error = &api.NotFoundError{Resource: "user", ID: "42"}

	var notFound *api.NotFoundError
	assert.True(t, errors.As(err, &notFound))
	assert.Equal(t, "user", notFound.Resource)
	assert.Equal(t, "42", notFound.ID)

	var validation *api.ValidationError
	assert.False(t, errors.As(err, &validation))
}

func TestErrorData(t *testing.T) {
	ed := api.ErrorData{
		Code:     "NOT_FOUND",
		Message:  "User not found",
		NodeID:   "node-1",
		NodeType: "db.findOne",
		Details:  map[string]any{"table": "users"},
	}
	assert.Equal(t, "NOT_FOUND", ed.Code)
	assert.Equal(t, "node-1", ed.NodeID)
}
