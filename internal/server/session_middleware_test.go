package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

// fakeSessionAuth implements api.SessionAuthenticator without a real DB.
type fakeSessionAuth struct {
	validToken string
	err        error // when set, AuthenticateSession fails with this
}

func (f *fakeSessionAuth) AuthenticateSession(_ context.Context, _ any, tok string) (*api.AuthData, error) {
	if f.err != nil {
		return nil, f.err
	}
	if tok == f.validToken {
		return &api.AuthData{
			UserID: "user-1",
			Roles:  []string{"user"},
			Claims: map[string]any{"sub": "user-1", "email": "a@b.c", "session_id": "sess-1", "roles": []string{"user"}},
		}, nil
	}
	return nil, nil
}
func (f *fakeSessionAuth) DatabaseServiceName() string { return "db" }
func (f *fakeSessionAuth) SessionCookieName() string   { return "noda_session" }

// newTestServerWithServices builds a test Server (mirroring newTestServer in
// editor_nodes.go) whose ServiceRegistry is pre-populated with the given
// name→instance pairs, for tests that need server-scoped middleware to
// resolve services at request time.
func newTestServerWithServices(t *testing.T, services map[string]any) *Server {
	t.Helper()
	reg := registry.NewServiceRegistry()
	for name, instance := range services {
		require.NoError(t, reg.Register(name, instance, nil))
	}
	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Routes:    map[string]map[string]any{},
		Workflows: map[string]map[string]any{},
		Schemas:   map[string]map[string]any{},
	}
	srv, err := NewServer(rc, reg, buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	return srv
}

func TestSessionMiddleware(t *testing.T) {
	// Build a Server whose ServiceRegistry contains "auth" → &fakeSessionAuth{validToken: "tok123"}
	// and "db" → struct{}{} (never dereferenced by the fake), using the same
	// construction helpers as the other middleware tests in this package.
	s := newTestServerWithServices(t, map[string]any{
		"auth": &fakeSessionAuth{validToken: "tok123"},
		"db":   struct{}{},
	})

	h, err := s.buildMiddleware("auth.session")
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Use(h)
	app.Get("/x", func(c fiber.Ctx) error {
		if c.Locals(api.LocalJWTUserID) != "user-1" {
			t.Error("user id local not set")
		}
		claims, _ := c.Locals(api.LocalJWTClaims).(map[string]any)
		if claims["session_id"] != "sess-1" {
			t.Error("claims local not set")
		}
		return c.SendString("ok")
	})

	// bearer
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer tok123")
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("bearer: status %d", resp.StatusCode)
	}

	// cookie
	req = httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(&http.Cookie{Name: "noda_session", Value: "tok123"})
	resp, _ = app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("cookie: status %d", resp.StatusCode)
	}

	// invalid / missing → 401
	for _, setup := range []func(*http.Request){
		func(r *http.Request) {},
		func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") },
		func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "noda_session", Value: "wrong"}) },
	} {
		req := httptest.NewRequest("GET", "/x", nil)
		setup(req)
		resp, _ := app.Test(req)
		if resp.StatusCode != 401 {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	}
}

func TestSessionMiddlewareOrdering(t *testing.T) {
	if err := ValidateMiddlewareOrder([]string{"auth.session", "casbin.enforce"}); err != nil {
		t.Fatalf("auth.session before casbin must be valid: %v", err)
	}
	if err := ValidateMiddlewareOrder([]string{"casbin.enforce", "auth.session"}); err == nil {
		t.Fatal("casbin before auth.session must be rejected")
	}
}

// A database outage during session validation is infrastructure, not a
// server bug: it must surface as 503/504 rather than a blanket 500. 409 and
// 422 stay deliberately unmapped — a session lookup is a SELECT on a hashed
// token, so neither is reachable by a caller.
func TestSessionMiddlewareHonorsTypedErrors(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		want       int
		expectCode string
		absent     []string
	}{
		{"unavailable", &api.ServiceUnavailableError{
			Service: "database",
			Cause:   errors.New("pq: connection refused 10.0.0.5:5432"),
		}, 503, "SERVICE_UNAVAILABLE", []string{"connection refused", "10.0.0.5"}},
		{"timeout", &api.TimeoutError{
			Operation: "database query",
			Cause:     errors.New("pq: canceling statement due to statement timeout"),
		}, 504, "TIMEOUT", []string{"canceling statement"}},
		{"conflict stays 500", &api.ConflictError{Resource: "session"}, 500, "HTTP_ERROR", nil},
		{"validation stays 500", &api.ValidationError{Message: "nope"}, 500, "HTTP_ERROR", nil},
		{"unmapped stays 500", errors.New("boom"), 500, "HTTP_ERROR", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServerWithServices(t, map[string]any{
				"auth": &fakeSessionAuth{validToken: "tok123", err: tc.err},
				"db":   struct{}{},
			})
			h, err := s.buildMiddleware("auth.session")
			require.NoError(t, err)

			app := fiber.New(fiber.Config{ErrorHandler: s.errorHandler})
			app.Use(h)
			app.Get("/x", func(c fiber.Ctx) error { return c.SendString("ok") })

			req := httptest.NewRequest("GET", "/x", nil)
			req.Header.Set("Authorization", "Bearer tok123")
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tc.want, resp.StatusCode)

			bodyBytes, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			body := string(bodyBytes)
			for _, frag := range tc.absent {
				require.NotContains(t, body, frag,
					"middleware must not render Cause detail on this ungated path")
			}
			if tc.expectCode != "" {
				require.Contains(t, body, tc.expectCode)
			}
		})
	}
}
