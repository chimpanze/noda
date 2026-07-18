//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/migrate"
	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/internal/registry"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/chimpanze/noda/pkg/api"
	corecontrol "github.com/chimpanze/noda/plugins/core/control"
	coreresponse "github.com/chimpanze/noda/plugins/core/response"
	coretransform "github.com/chimpanze/noda/plugins/core/transform"
	coreutil "github.com/chimpanze/noda/plugins/core/util"
	coreworkflow "github.com/chimpanze/noda/plugins/core/workflow"
	dbplugin "github.com/chimpanze/noda/plugins/db"
	emailplugin "github.com/chimpanze/noda/plugins/email"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fixtureDir is the scaffolded testdata/auth project (committed fixture,
// generated once via `go run ./cmd/noda auth init --dir testdata/auth`).
const fixtureDir = "../../testdata/auth"

// --- email.send mock -------------------------------------------------------
//
// The register / request-password-reset workflows send verification and
// reset tokens by email. Rather than requiring a live Mailpit container,
// the "mailer" node type is overridden with a mock executor that captures
// the resolved to/subject/body so tests can recover the raw token (the
// database only ever stores its hash — see auth.create_token).

type capturedEmail struct {
	To      string
	Subject string
	Body    string
}

type mailbox struct {
	mu   sync.Mutex
	sent []capturedEmail
}

func (m *mailbox) factory(_ map[string]any) api.NodeExecutor {
	return &mockEmailExecutor{mailbox: m}
}

func (m *mailbox) last() capturedEmail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[len(m.sent)-1]
}

type mockEmailExecutor struct{ mailbox *mailbox }

func (e *mockEmailExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *mockEmailExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	to, err := plugin.ResolveString(nCtx, config, "to")
	if err != nil {
		return "", nil, err
	}
	subject, err := plugin.ResolveString(nCtx, config, "subject")
	if err != nil {
		return "", nil, err
	}
	body, err := plugin.ResolveString(nCtx, config, "body")
	if err != nil {
		return "", nil, err
	}
	e.mailbox.mu.Lock()
	e.mailbox.sent = append(e.mailbox.sent, capturedEmail{To: to, Subject: subject, Body: body})
	e.mailbox.mu.Unlock()
	return api.OutputSuccess, map[string]any{"message_id": "mock"}, nil
}

var tokenRE = regexp.MustCompile(`<strong>([^<]+)</strong>`)

func extractToken(t *testing.T, body string) string {
	t.Helper()
	m := tokenRE.FindStringSubmatch(body)
	require.Len(t, m, 2, "could not find <strong>token</strong> in email body: %q", body)
	return m[1]
}

// --- setup -------------------------------------------------------------

// setupAuthEngine mirrors plugins/db/engine_e2e_integration_test.go's
// bootstrap style (build registries by hand, no full config.Bootstrap): a
// real sqlite database migrated from the scaffolded fixture, a real auth
// service, and the email service replaced by the mock above so no SMTP
// server is required.
func setupAuthEngine(t *testing.T) (*registry.ServiceRegistry, *registry.NodeRegistry, *gorm.DB, *mailbox) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "auth-e2e.db")
	dbSvc, err := (&dbplugin.Plugin{}).CreateService(map[string]any{"driver": "sqlite", "path": dbPath})
	require.NoError(t, err)
	gdb := dbSvc.(*gorm.DB)

	migrationsDir := filepath.Join(fixtureDir, "migrations")
	_, err = migrate.Up(gdb, migrationsDir)
	require.NoError(t, err)

	authSvc, err := (&Plugin{}).CreateService(map[string]any{"database": "main-db"})
	require.NoError(t, err)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("main-db", gdb, nil))
	require.NoError(t, svcReg.Register("auth", authSvc, nil))
	// Placeholder: the real email plugin service is never created (no SMTP
	// connection), but dispatch.go resolves the "mailer" slot by service
	// name before the node factory runs, so the name must still exist.
	require.NoError(t, svcReg.Register("email", struct{}{}, nil))

	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))
	require.NoError(t, nodeReg.RegisterFromPlugin(&dbplugin.Plugin{}))
	require.NoError(t, nodeReg.RegisterFromPlugin(&coreresponse.Plugin{}))
	require.NoError(t, nodeReg.RegisterFromPlugin(&emailplugin.Plugin{}))
	// util.timestamp/util.delay: the constant-time pad chains (#289/#290)
	// in login, request-password-reset, and resend-verification need these —
	// the pre-regeneration fixture predated the pads, so the old registry
	// never had to know about them.
	require.NoError(t, nodeReg.RegisterFromPlugin(&coreutil.Plugin{}))

	mb := &mailbox{}
	// Override just the factory (keep the real descriptor registered above,
	// which is harmless since dispatch only consults the factory).
	nodeReg.RegisterFactory("email.send", mb.factory)

	return svcReg, nodeReg, gdb, mb
}

func loadWorkflow(t *testing.T, file, id string) engine.WorkflowConfig {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, "workflows", file))
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	wf, err := engine.ParseWorkflowFromMap(id, raw)
	require.NoError(t, err)
	return wf
}

func runWorkflow(
	t *testing.T,
	svcReg *registry.ServiceRegistry,
	nodeReg *registry.NodeRegistry,
	wf engine.WorkflowConfig,
	input map[string]any,
) *engine.ExecutionContextImpl {
	t.Helper()
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(input))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
	return execCtx
}

func httpOutput(t *testing.T, execCtx *engine.ExecutionContextImpl, nodeID string) *api.HTTPResponse {
	t.Helper()
	out, ok := execCtx.GetOutput(nodeID)
	require.True(t, ok, "no output recorded for node %q", nodeID)
	resp, ok := out.(*api.HTTPResponse)
	require.True(t, ok, "node %q output is %T, not *api.HTTPResponse", nodeID, out)
	return resp
}

// TestEngineE2E_AuthFlows drives the scaffolded auth-* workflows end-to-end
// through the real engine: register, login (success + failure), password
// reset (with the mandatory enumeration check), and logout + revocation.
// It shares one temp sqlite database across ordered subtests, per the task
// brief.
func TestEngineE2E_AuthFlows(t *testing.T) {
	svcReg, nodeReg, gdb, mb := setupAuthEngine(t)

	// Captured by the register subtest; consumed by verify_email. The
	// duplicate-register check below sends a token-less "already registered"
	// notice after the verification email, so mb.last() is no longer it.
	var verifyToken string

	t.Run("register", func(t *testing.T) {
		wf := loadWorkflow(t, "auth.register.json", "auth-register")
		execCtx := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{
			"email":    "alice@example.com",
			"password": "password123",
		})

		// Verification-first (#289): a generic 200, no session, no token.
		resp := httpOutput(t, execCtx, "respond")
		assert.Equal(t, 200, resp.Status)
		body, ok := resp.Body.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Check your email to continue", body["message"])
		assert.NotContains(t, body, "token")

		var userCount, sessionCount int64
		require.NoError(t, gdb.Table("auth_users").Where("email = ?", "alice@example.com").Count(&userCount).Error)
		assert.Equal(t, int64(1), userCount)
		require.NoError(t, gdb.Table("auth_sessions").Count(&sessionCount).Error)
		assert.Equal(t, int64(0), sessionCount, "verification-first register must not create a session")

		// A verification email was "sent" via the mocked mailer node.
		sent := mb.last()
		assert.Equal(t, "alice@example.com", sent.To)
		verifyToken = extractToken(t, sent.Body)
		require.NotEmpty(t, verifyToken)

		// Anti-enumeration: registering the same email again returns the
		// byte-identical body (from respond_exists) and sends the
		// "already registered" notice instead of a second verification.
		execCtx2 := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{
			"email":    "alice@example.com",
			"password": "password123",
		})
		resp2 := httpOutput(t, execCtx2, "respond_exists")
		assert.Equal(t, 200, resp2.Status)
		freshJSON, err := json.Marshal(resp.Body)
		require.NoError(t, err)
		existsJSON, err := json.Marshal(resp2.Body)
		require.NoError(t, err)
		assert.JSONEq(t, string(freshJSON), string(existsJSON))
		assert.Equal(t, "Account already registered", mb.last().Subject)
		require.NoError(t, gdb.Table("auth_users").Where("email = ?", "alice@example.com").Count(&userCount).Error)
		assert.Equal(t, int64(1), userCount, "duplicate register must not create a second user")
	})

	t.Run("verify_email", func(t *testing.T) {
		require.NotEmpty(t, verifyToken, "register subtest must run first")

		wf := loadWorkflow(t, "auth.verify-email.json", "auth-verify-email")
		execCtx := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{"token": verifyToken})
		resp := httpOutput(t, execCtx, "respond")
		assert.Equal(t, 200, resp.Status)

		var row struct{ EmailVerifiedAt *time.Time }
		require.NoError(t, gdb.Table("auth_users").
			Where("email = ?", "alice@example.com").
			Select("email_verified_at").Take(&row).Error)
		require.NotNil(t, row.EmailVerifiedAt)

		// Re-using the same token must fail: single-use, undifferentiated "invalid".
		execCtx2 := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{"token": verifyToken})
		resp2 := httpOutput(t, execCtx2, "respond_invalid")
		assert.Equal(t, 400, resp2.Status)
	})

	t.Run("login", func(t *testing.T) {
		wf := loadWorkflow(t, "auth.login.json", "auth-login")

		t.Run("correct credentials", func(t *testing.T) {
			execCtx := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{
				"email":    "alice@example.com",
				"password": "password123",
			})
			resp := httpOutput(t, execCtx, "respond")
			assert.Equal(t, 200, resp.Status)
			body, ok := resp.Body.(map[string]any)
			require.True(t, ok)
			require.NotEmpty(t, body["token"])
		})

		t.Run("wrong password", func(t *testing.T) {
			execCtx := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{
				"email":    "alice@example.com",
				"password": "wrong-password",
			})
			resp := httpOutput(t, execCtx, "respond_invalid")
			assert.Equal(t, 401, resp.Status)
		})
	})

	t.Run("request_password_reset_enumeration", func(t *testing.T) {
		wf := loadWorkflow(t, "auth.request-password-reset.json", "auth-request-password-reset")

		knownCtx := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{"email": "alice@example.com"})
		knownResp := httpOutput(t, knownCtx, "respond_sent")

		unknownStart := time.Now()
		unknownCtx := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{"email": "nobody@example.com"})
		unknownElapsed := time.Since(unknownStart)
		unknownResp := httpOutput(t, unknownCtx, "respond_unknown")

		// The unknown branch is the fast path the constant-time pad exists
		// to slow down (#289): without util.delay it returns in ~1 ms.
		// Loose lower bound (deadline is ~500 ms) to avoid timer flake.
		assert.GreaterOrEqual(t, unknownElapsed, 400*time.Millisecond,
			"unknown-email branch must be padded to the fixed deadline")

		// Mandatory enumeration check: byte-identical response node output
		// for an existing vs. an unknown email.
		knownJSON, err := json.Marshal(knownResp)
		require.NoError(t, err)
		unknownJSON, err := json.Marshal(unknownResp)
		require.NoError(t, err)
		assert.JSONEq(t, string(unknownJSON), string(knownJSON))
		assert.Equal(t, 200, knownResp.Status)
		assert.Equal(t, 200, unknownResp.Status)

		// A reset email was sent only for the known address.
		sent := mb.last()
		assert.Equal(t, "alice@example.com", sent.To)
	})

	var freshSessionToken string

	t.Run("reset_password", func(t *testing.T) {
		resetToken := extractToken(t, mb.last().Body)

		resetWF := loadWorkflow(t, "auth.reset-password.json", "auth-reset-password")
		execCtx := runWorkflow(t, svcReg, nodeReg, resetWF, map[string]any{
			"token":    resetToken,
			"password": "newpassword456",
		})
		resp := httpOutput(t, execCtx, "respond")
		assert.Equal(t, 200, resp.Status)

		// The token was consumed atomically inside set_password (#290);
		// reuse must hit respond_invalid, single-use and undifferentiated.
		reuseCtx := runWorkflow(t, svcReg, nodeReg, resetWF, map[string]any{
			"token":    resetToken,
			"password": "anotherpassword789",
		})
		assert.Equal(t, 400, httpOutput(t, reuseCtx, "respond_invalid").Status)

		loginWF := loadWorkflow(t, "auth.login.json", "auth-login")

		oldCtx := runWorkflow(t, svcReg, nodeReg, loginWF, map[string]any{
			"email":    "alice@example.com",
			"password": "password123",
		})
		assert.Equal(t, 401, httpOutput(t, oldCtx, "respond_invalid").Status)

		newCtx := runWorkflow(t, svcReg, nodeReg, loginWF, map[string]any{
			"email":    "alice@example.com",
			"password": "newpassword456",
		})
		newResp := httpOutput(t, newCtx, "respond")
		assert.Equal(t, 200, newResp.Status)
		body, ok := newResp.Body.(map[string]any)
		require.True(t, ok)
		freshSessionToken, _ = body["token"].(string)
		require.NotEmpty(t, freshSessionToken)
	})

	t.Run("logout", func(t *testing.T) {
		require.NotEmpty(t, freshSessionToken, "reset_password subtest must run first")

		authSvcAny, _ := svcReg.Get("auth")
		authSvc := authSvcAny.(*Service)

		// The real /auth/logout route resolves session_id from
		// auth.claims.session_id, populated by the auth.session middleware
		// (session_auth.go's AuthenticateSession) — reproduce that lookup
		// directly since this test bypasses the HTTP/middleware layer.
		authData, err := authSvc.AuthenticateSession(context.Background(), gdb, freshSessionToken)
		require.NoError(t, err)
		require.NotNil(t, authData)
		sessionID, _ := authData.Claims["session_id"].(string)
		require.NotEmpty(t, sessionID)

		wf := loadWorkflow(t, "auth.logout.json", "auth-logout")
		execCtx := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{"session_id": sessionID})
		resp := httpOutput(t, execCtx, "respond")
		assert.Equal(t, 204, resp.Status)

		data, err := authSvc.AuthenticateSession(context.Background(), gdb, freshSessionToken)
		require.NoError(t, err)
		assert.Nil(t, data, "revoked session must not authenticate")
	})
}

// TestEngineE2E_ScaffoldedTestSuites runs the scaffolded tests/*.json test
// files through the project's own workflow test runner (internal/testing),
// the same code path `noda test` uses. These test files ship with every
// scaffolded project and were previously never executed by anything —
// this closes that gap.
func TestEngineE2E_ScaffoldedTestSuites(t *testing.T) {
	sm, err := config.NewSecretsManager(fixtureDir, "")
	require.NoError(t, err)
	rc, errs := config.ValidateAll(fixtureDir, "", sm)
	require.Empty(t, errs, "fixture must validate cleanly")

	suites, err := nodatesting.LoadTests(rc)
	require.NoError(t, err)
	require.NotEmpty(t, suites, "expected scaffolded tests/*.json suites")

	coreNodeReg := registry.NewNodeRegistry()
	for _, p := range []api.Plugin{
		&corecontrol.Plugin{},
		&coretransform.Plugin{},
		&coreutil.Plugin{},
		&coreworkflow.Plugin{},
		&coreresponse.Plugin{},
	} {
		require.NoError(t, coreNodeReg.RegisterFromPlugin(p))
	}

	for _, suite := range suites {
		suite := suite
		t.Run(suite.ID, func(t *testing.T) {
			results := nodatesting.RunTestSuite(suite, rc, coreNodeReg, nil)
			require.NotEmpty(t, results)
			for _, r := range results {
				assert.Truef(t, r.Passed, "case %q failed: %s", r.CaseName, r.Error)
			}
		})
	}
}
