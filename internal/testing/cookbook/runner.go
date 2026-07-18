package cookbook

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/fasthttp/websocket"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/migrate"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/server"
	"github.com/chimpanze/noda/internal/wasm"
	"github.com/chimpanze/noda/internal/worker"
	"github.com/chimpanze/noda/pkg/api"
)

// Options configures a RunProject invocation. Env pairs are exported via
// t.Setenv before config load; MailpitAPI is the base URL mail steps poll.
type Options struct {
	Env        map[string]string
	MailpitAPI string
	// Vars pre-seeds the step-loop's variable map before the first step
	// runs (e.g. an auth code obtained by a walker dep). Step captures may
	// later overwrite a seeded key; that's fine.
	Vars map[string]string
}

// runContext carries the state prepareEnv establishes before config load —
// the resolved Options, the listener reserved for listen-mode suites (nil
// otherwise), its base URL, and whether the caller originally supplied Env
// (the deps guard's signal). It is threaded through instead of adding more
// positional parameters.
type runContext struct {
	opt     Options
	ln      net.Listener
	baseURL string
	hadEnv  bool
}

// RunProject loads the cookbook project at dir through the production config
// pipeline, boots the real server in-process (or over a real TCP listener
// for listen-mode suites), and replays the project's verify.json steps. Any
// failure fails t with the step name in the message.
//
// opts is variadic so pre-tranche-2 call sites (which pass no Options) keep
// compiling unchanged; at most one Options value is honored.
func RunProject(t *testing.T, dir string, plugins []api.Plugin, opts ...Options) {
	t.Helper()
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	rctx, err := prepareEnv(t, dir, opt)
	if err != nil {
		t.Fatalf("cookbook %s: %v", filepath.Base(dir), err)
	}
	if rctx.ln != nil {
		defer func() { _ = rctx.ln.Close() }()
	}
	if err := runProject(dir, plugins, rctx); err != nil {
		t.Fatalf("cookbook %s: %v", filepath.Base(dir), err)
	}
}

// runProjectRecorded runs the project and reports whether it failed, without
// failing t, along with the error message (empty on success). Used to test
// the runner itself against a project expected to be rejected (e.g.
// non-empty deps without Options.Env).
//
// runProject returns a plain error rather than calling t.Errorf/t.Fatalf so
// that this helper can observe failure without a t.Run sub-test: a failing
// sub-test marks the parent (and package) failed regardless of any recorded
// return value, which would make the expected-failure tests show up as a
// real `go test` failure.
func runProjectRecorded(t *testing.T, dir string, plugins []api.Plugin, opts ...Options) (bool, string) {
	t.Helper()
	var opt Options
	if len(opts) > 0 {
		opt = opts[0]
	}
	rctx, err := prepareEnv(t, dir, opt)
	if err != nil {
		t.Logf("cookbook %s: %v", filepath.Base(dir), err)
		return true, err.Error()
	}
	if rctx.ln != nil {
		defer func() { _ = rctx.ln.Close() }()
	}
	if err := runProject(dir, plugins, rctx); err != nil {
		t.Logf("cookbook %s: %v", filepath.Base(dir), err)
		return true, err.Error()
	}
	return false, ""
}

// prepareEnv resolves the run context: reserves a listener and exports
// COOKBOOK_BASE_URL for listen-mode suites (before Env is exported, so
// config load can reference it), exports Env + COOKBOOK_DATA_DIR, and
// copies seed files. Ordering matters: the listener must be reserved and
// its URL exported before LoadSuite's caller (runProject) loads config.
func prepareEnv(t *testing.T, dir string, opt Options) (*runContext, error) {
	t.Helper()
	built := &runContext{opt: opt, hadEnv: opt.Env != nil}
	// If anything below fails after the listener is reserved, close it here
	// rather than leaking it — a nil return means the caller never gets a
	// runContext to defer-close.
	ok := false
	defer func() {
		if !ok && built.ln != nil {
			_ = built.ln.Close()
		}
	}()

	if built.opt.Env == nil {
		built.opt.Env = map[string]string{}
	}
	if _, exists := built.opt.Env["COOKBOOK_DATA_DIR"]; !exists {
		built.opt.Env["COOKBOOK_DATA_DIR"] = t.TempDir()
	}

	suite, err := LoadSuite(filepath.Join(dir, "verify.json"))
	if err != nil {
		return nil, err
	}

	if suite.Listen {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, fmt.Errorf("reserve listener: %w", err)
		}
		built.ln = ln
		built.baseURL = "http://" + ln.Addr().String()
		built.opt.Env["COOKBOOK_BASE_URL"] = built.baseURL
	}

	for k, v := range built.opt.Env {
		t.Setenv(k, v)
	}

	dataDir := built.opt.Env["COOKBOOK_DATA_DIR"]
	for dest, src := range suite.Seed {
		content, err := os.ReadFile(filepath.Join(dir, src))
		if err != nil {
			return nil, fmt.Errorf("seed %q: %w", src, err)
		}
		target := filepath.Join(dataDir, dest)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("seed %q: %w", dest, err)
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return nil, fmt.Errorf("seed %q: %w", dest, err)
		}
	}
	ok = true
	return built, nil
}

// testLogger returns a logger that discards output, so worker/wasm runtime
// startup logging doesn't clutter cookbook test output.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// waitReady polls until the server accepts TCP requests (any status).
func waitReady(url string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("server never became ready at %s", url)
}

// resolveWorkerMiddleware mirrors cmd/noda's resolveWorkerMiddleware: use
// the first worker's declared middleware names (resolved to implementations)
// if any worker configures them, else the package default chain.
func resolveWorkerMiddleware(configs []worker.WorkerConfig) []worker.Middleware {
	for _, wc := range configs {
		if len(wc.Middleware) > 0 {
			return worker.ResolveMiddleware(wc.Middleware)
		}
	}
	return worker.DefaultMiddleware()
}

// makeWasmWorkflowRunner mirrors cmd/noda's buildWorkflowRunner: an
// api.WorkflowRunner that executes a workflow by ID against the shared
// cache/services/nodes, used by the wasm runtime's trigger_workflow host
// call.
func makeWasmWorkflowRunner(
	cache *engine.WorkflowCache,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
	compiler *expr.Compiler,
	secretsCtx map[string]any,
) api.WorkflowRunner {
	subRunner := &engine.SubWorkflowRunnerImpl{
		Cache:    cache,
		Services: services,
		Nodes:    nodes,
	}
	return func(ctx context.Context, workflowID string, input map[string]any) error {
		graph, ok := cache.Get(workflowID)
		if !ok {
			return fmt.Errorf("workflow %q not found", workflowID)
		}
		execCtx := engine.NewExecutionContext(
			engine.WithInput(input),
			engine.WithTrigger(api.TriggerData{
				Type:    "wasm",
				TraceID: uuid.New().String(),
			}),
			engine.WithWorkflowID(workflowID),
			engine.WithCompiler(compiler),
			engine.WithSecrets(secretsCtx),
			engine.WithSubWorkflowRunner(subRunner),
		)
		return engine.ExecuteGraph(ctx, graph, execCtx, services, nodes)
	}
}

// parseWasmModuleConfig replicates cmd/noda's parseWasmModuleConfig: it is
// cmd-private, so the minimal field set it extracts from a wasm_runtimes
// entry is duplicated here against the exported wasm.ModuleConfig shape.
func parseWasmModuleConfig(name string, raw any) wasm.ModuleConfig {
	cfg := wasm.ModuleConfig{Name: name}
	m, ok := raw.(map[string]any)
	if !ok {
		return cfg
	}
	if v, ok := m["module"].(string); ok {
		cfg.ModulePath = v
	}
	if v, ok := m["tick_rate"].(float64); ok {
		cfg.TickRate = int(v)
	}
	if v, ok := m["encoding"].(string); ok {
		cfg.Encoding = v
	}
	if v, ok := m["config"].(map[string]any); ok {
		cfg.Config = v
	}
	return cfg
}

func runProject(dir string, plugins []api.Plugin, rctx *runContext) error {
	suite, err := LoadSuite(filepath.Join(dir, "verify.json"))
	if err != nil {
		return err
	}
	if len(suite.Deps) > 0 && !rctx.hadEnv {
		return fmt.Errorf("deps %v declared but no environment provided (run via the integration walker)", suite.Deps)
	}

	sm, err := config.NewSecretsManager(dir, "")
	if err != nil {
		return fmt.Errorf("secrets manager: %w", err)
	}
	rc, verrs := config.ValidateAll(dir, "", sm)
	if len(verrs) > 0 {
		return fmt.Errorf("config validation: %v", verrs)
	}

	preg := registry.NewPluginRegistry()
	for _, p := range plugins {
		if err := preg.Register(p); err != nil {
			return fmt.Errorf("registering plugin %s: %w", p.Name(), err)
		}
	}
	boot, berrs := registry.Bootstrap(context.Background(), rc, preg)
	if len(berrs) > 0 {
		return fmt.Errorf("bootstrap: %v", berrs)
	}

	if fi, err := os.Stat(filepath.Join(dir, "migrations")); err == nil && fi.IsDir() {
		svc, ok := boot.Services.Get("main-db")
		if !ok {
			return fmt.Errorf("migrations/ present but no main-db service")
		}
		gdb, ok := svc.(*gorm.DB)
		if !ok {
			return fmt.Errorf("main-db service is %T, not *gorm.DB", svc)
		}
		if _, err := migrate.Up(gdb, filepath.Join(dir, "migrations")); err != nil {
			return fmt.Errorf("migrations: %w", err)
		}
	}

	// The cache must be built here and injected via WithWorkflowCache: although
	// srv.Setup() would self-build an identical cache, NewServer only wires the
	// sub-workflow runner (used by workflow.run / control.loop) when the cache
	// is already present at construction time (server.go NewServer).
	wfCache, err := engine.NewWorkflowCache(rc.Workflows, boot.Nodes)
	if err != nil {
		return fmt.Errorf("workflow cache: %w", err)
	}

	secretsCtx := sm.ExpressionContext()
	srv, err := server.NewServer(rc, boot.Services, boot.Nodes,
		server.WithWorkflowCache(wfCache),
		server.WithCompiler(boot.Compiler),
		server.WithSecretsContext(secretsCtx),
	)
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}
	if err := srv.Setup(); err != nil {
		return fmt.Errorf("server setup: %w", err)
	}

	if len(rc.Workers) > 0 {
		workerConfigs := worker.ParseWorkerConfigs(rc.Workers)
		mw := resolveWorkerMiddleware(workerConfigs)
		wr := worker.NewRuntime(workerConfigs, boot.Services, boot.Nodes, rc.Workflows, wfCache,
			mw, boot.Compiler, nil, testLogger(), secretsCtx)
		if err := wr.Start(context.Background()); err != nil {
			return fmt.Errorf("workers: %w", err)
		}
		defer func() { _ = wr.Stop(context.Background()) }()
	}

	if wasmRuntimes, _ := rc.Root["wasm_runtimes"].(map[string]any); len(wasmRuntimes) > 0 {
		workflowRunner := makeWasmWorkflowRunner(wfCache, boot.Services, boot.Nodes, boot.Compiler, secretsCtx)
		wrt := wasm.NewRuntime(boot.Services, workflowRunner, testLogger())
		for name, raw := range wasmRuntimes {
			cfg := parseWasmModuleConfig(name, raw)
			if cfg.ModulePath != "" && !filepath.IsAbs(cfg.ModulePath) {
				cfg.ModulePath = filepath.Join(dir, cfg.ModulePath)
			}
			if _, err := wrt.LoadModule(context.Background(), cfg); err != nil {
				return fmt.Errorf("loading wasm module %q: %w", name, err)
			}
			wasmSvc := wasm.NewWasmService(wrt, name)
			// Intentional divergence from cmd/noda's createWasm, which only
			// warns on registration failure: the test harness fails loud.
			if err := boot.Services.Register(name, wasmSvc, nil); err != nil {
				return fmt.Errorf("registering wasm service %q: %w", name, err)
			}
		}
		if err := wrt.StartAll(context.Background()); err != nil {
			return fmt.Errorf("wasm start: %w", err)
		}
		defer func() { _ = wrt.StopAll(context.Background()) }()
	}

	if suite.Listen {
		go func() { _ = srv.App().Listener(rctx.ln) }()
		// Bound shutdown the same way production does (internal/server/server.go's
		// Stop, via a context deadline): an unbounded Shutdown() waits for every
		// open connection to go idle, and this harness routinely leaves WS/SSE
		// clients open across a suite (they're closed by the wsConns/sseClosers
		// defers below, which — being registered later — run first, but any
		// server-side write buffering or a slow client teardown can still stall
		// an unbounded Shutdown()). Cap it so one suite can't hang the whole run.
		defer func() { _ = srv.App().ShutdownWithTimeout(3 * time.Second) }()
		if err := waitReady(rctx.baseURL + "/"); err != nil {
			return err
		}
	}

	vars := map[string]string{}
	for k, v := range rctx.opt.Vars {
		vars[k] = v
	}
	wsConns := map[string]*websocket.Conn{}
	defer func() {
		for _, c := range wsConns {
			_ = c.Close()
		}
	}()
	sseReaders := map[string]*bufio.Reader{}
	sseClosers := map[string]io.Closer{}
	defer func() {
		for _, c := range sseClosers {
			_ = c.Close()
		}
	}()

	for _, step := range suite.Steps {
		switch {
		case step.Mail != nil:
			if rctx.opt.MailpitAPI == "" {
				return fmt.Errorf("step %q: mail step but no MailpitAPI configured", step.Name)
			}
			if err := checkMail(rctx.opt.MailpitAPI, *step.Mail); err != nil {
				return fmt.Errorf("step %q: %w", step.Name, err)
			}
		case step.WS != nil:
			if err := runWSStep(rctx.baseURL, wsConns, step, vars); err != nil {
				return fmt.Errorf("step %q: %w", step.Name, err)
			}
		case step.SSE != nil:
			if err := runSSEStep(rctx.baseURL, sseReaders, sseClosers, step, vars); err != nil {
				return fmt.Errorf("step %q: %w", step.Name, err)
			}
		case suite.Listen:
			if err := runStepHTTP(rctx.baseURL, step, vars); err != nil {
				return fmt.Errorf("step %q: %w", step.Name, err)
			}
		default:
			if err := runStep(srv, step, vars); err != nil {
				return fmt.Errorf("step %q: %w", step.Name, err)
			}
		}
	}
	return nil
}

// substituteBody replaces ${name} refs in every string leaf of a decoded
// request body, so substituted values are JSON-escaped by the subsequent
// marshal rather than spliced into serialized text.
func substituteBody(v any, vars map[string]string) any {
	switch t := v.(type) {
	case string:
		return Substitute(t, vars)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = substituteBody(val, vars)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = substituteBody(val, vars)
		}
		return out
	default:
		return v
	}
}

// buildRequestBody renders a step's body/multipart spec into a request
// reader and content type, shared by both the in-process and real-transport
// request paths.
func buildRequestBody(step Step, vars map[string]string) (io.Reader, string, bool, error) {
	var bodyReader io.Reader
	hasBody := step.Request.Body != nil
	contentType := ""
	if hasBody {
		raw, err := json.Marshal(substituteBody(step.Request.Body, vars))
		if err != nil {
			return nil, "", false, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
		contentType = "application/json"
	}
	if step.Request.Multipart != nil {
		ct, buf, err := buildMultipart(step.Request.Multipart, vars)
		if err != nil {
			return nil, "", false, fmt.Errorf("multipart: %w", err)
		}
		bodyReader = buf
		contentType = ct
		hasBody = true
	}
	return bodyReader, contentType, hasBody, nil
}

// checkResponse applies a step's status/header/body_text/body/capture
// assertions against a received response. Shared by the in-process
// (runStep) and real-transport (runStepHTTP) request paths.
func checkResponse(status int, header http.Header, raw []byte, step Step, vars map[string]string) error {
	if status != step.Expect.Status {
		return fmt.Errorf("expected status %d, got %d (body: %.500s)", step.Expect.Status, status, raw)
	}
	for k, want := range step.Expect.Headers {
		expected := Substitute(want, vars)
		if got := header.Get(k); got != expected {
			return fmt.Errorf("expected header %s=%q, got %q", k, expected, got)
		}
	}
	if step.Expect.BodyText != nil {
		if string(raw) != *step.Expect.BodyText {
			return fmt.Errorf("expected body_text %q, got %q", *step.Expect.BodyText, raw)
		}
	}

	if len(step.Expect.Body) > 0 || len(step.Capture) > 0 {
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("response is not JSON (%.200s): %w", raw, err)
		}
		for _, a := range step.Expect.Body {
			if err := CheckAssertion(doc, a); err != nil {
				return err
			}
		}
		if err := Capture(doc, step.Capture, vars); err != nil {
			return err
		}
	}
	return nil
}

// withRetry runs fn once when timeout is empty; otherwise it re-runs fn
// (sleeping 150ms between attempts) until fn succeeds or the parsed
// timeout elapses, returning the LAST attempt's error on expiry. Shared by
// both request transports so retry_timeout behaves identically in listen
// and in-process suites.
func withRetry(timeout string, fn func() error) error {
	if timeout == "" {
		return fn()
	}
	d, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("invalid retry_timeout %q: %w", timeout, err)
	}
	deadline := time.Now().Add(d)
	for {
		lastErr := fn()
		if lastErr == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return lastErr
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// doInProcessAttempt performs one in-process round trip through the Fiber
// app's test transport and checks the response, without retrying.
func doInProcessAttempt(srv *server.Server, step Step, vars map[string]string) error {
	path := Substitute(step.Request.Path, vars)

	bodyReader, contentType, hasBody, err := buildRequestBody(step, vars)
	if err != nil {
		return err
	}

	req := httptest.NewRequest(step.Request.Method, path, bodyReader)
	if hasBody {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range step.Request.Headers {
		req.Header.Set(k, Substitute(v, vars))
	}

	resp, err := srv.App().Test(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	return checkResponse(resp.StatusCode, resp.Header, raw, step, vars)
}

// runStep executes a request step in-process against the Fiber app's test
// transport, for non-listen-mode projects, honoring RetryTimeout with the
// same semantics as the real-transport path.
func runStep(srv *server.Server, step Step, vars map[string]string) error {
	return withRetry(step.Request.RetryTimeout, func() error {
		return doInProcessAttempt(srv, step, vars)
	})
}

// doHTTPAttempt performs one real-transport HTTP round trip against
// baseURL+path and checks the response, without retrying.
func doHTTPAttempt(baseURL string, step Step, vars map[string]string) error {
	path := Substitute(step.Request.Path, vars)

	bodyReader, contentType, hasBody, err := buildRequestBody(step, vars)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(step.Request.Method, baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	if hasBody {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range step.Request.Headers {
		req.Header.Set(k, Substitute(v, vars))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	return checkResponse(resp.StatusCode, resp.Header, raw, step, vars)
}

// runStepHTTP executes a request step over a real TCP connection, for
// listen-mode projects, honoring RetryTimeout with the same semantics as
// the in-process path.
func runStepHTTP(baseURL string, step Step, vars map[string]string) error {
	return withRetry(step.Request.RetryTimeout, func() error {
		return doHTTPAttempt(baseURL, step, vars)
	})
}

// runWSStep executes one connect/send/expect action against a named
// WebSocket client, dialing and storing new connections in conns.
func runWSStep(baseURL string, conns map[string]*websocket.Conn, step Step, vars map[string]string) error {
	ws := step.WS
	switch {
	case ws.Connect != "":
		if _, exists := conns[ws.Client]; exists {
			return fmt.Errorf("ws client %q already connected", ws.Client)
		}
		url := "ws" + strings.TrimPrefix(baseURL, "http") + Substitute(ws.Connect, vars)
		// Theoretical race: the dial's 101 response completes on the client
		// before the server has necessarily finished registering the conn
		// with the connmgr Manager, so a broadcast fired immediately after
		// this call returns could in principle miss this client. In
		// practice the window is negligible — verify.json's broadcast
		// steps are always at least a full HTTP request/response (the POST
		// that triggers ws.send) plus this connect's own RTT later, which
		// is orders of magnitude longer than the registration that happens
		// synchronously inside the WS upgrade handler. Under CI load that
		// window can still widen enough to matter (see the realtime
		// cookbook flake fixed by an on_connect welcome broadcast — a
		// deterministic per-client registration gate in verify.json rather
		// than a code fix here, since delivery on this path is
		// unrecoverable/unpollable).
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			return fmt.Errorf("ws connect %q: %w", ws.Client, err)
		}
		conns[ws.Client] = conn
		return nil
	case ws.Send != nil:
		conn, ok := conns[ws.Client]
		if !ok {
			return fmt.Errorf("ws client %q not connected", ws.Client)
		}
		raw, err := json.Marshal(substituteBody(ws.Send, vars))
		if err != nil {
			return fmt.Errorf("marshal ws send: %w", err)
		}
		if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
			return fmt.Errorf("ws send %q: %w", ws.Client, err)
		}
		return nil
	case len(ws.Expect) > 0:
		conn, ok := conns[ws.Client]
		if !ok {
			return fmt.Errorf("ws client %q not connected", ws.Client)
		}
		return expectWSMessage(conn, ws.Expect)
	default:
		return fmt.Errorf("ws client %q: no action", ws.Client)
	}
}

// expectWSMessage reads messages from conn until one matches all
// assertions or the read deadline elapses. The read deadline is set ONCE
// before the loop: a fasthttp/gorilla-style websocket read-deadline timeout
// is sticky, so once it fires the connection is dead — which is fine here,
// since a timeout means the step has already failed.
func expectWSMessage(conn *websocket.Conn, assertions []BodyAssertion) error {
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set read deadline: %w", err)
	}
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("ws expect: %w", err)
		}
		matched, err := matchMessage(raw, assertions)
		if err != nil {
			return err
		}
		if matched {
			return nil
		}
	}
}

// runSSEStep executes one connect/expect action against a named SSE
// client, storing new connections' reader (in readers) and response body
// closer (in closers).
func runSSEStep(baseURL string, readers map[string]*bufio.Reader, closers map[string]io.Closer, step Step, vars map[string]string) error {
	sse := step.SSE
	switch {
	case sse.Connect != "":
		if _, exists := readers[sse.Client]; exists {
			return fmt.Errorf("sse client %q already connected", sse.Client)
		}
		url := baseURL + Substitute(sse.Connect, vars)
		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("sse connect %q: %w", sse.Client, err)
		}
		if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
			_ = resp.Body.Close()
			return fmt.Errorf("sse connect %q: expected text/event-stream, got %q", sse.Client, resp.Header.Get("Content-Type"))
		}
		readers[sse.Client] = bufio.NewReader(resp.Body)
		closers[sse.Client] = resp.Body
		return nil
	case len(sse.Expect) > 0:
		r, ok := readers[sse.Client]
		if !ok {
			return fmt.Errorf("sse client %q not connected", sse.Client)
		}
		return expectSSEEvent(r, sse.Expect)
	default:
		return fmt.Errorf("sse client %q: no action", sse.Client)
	}
}

// expectSSEEvent reads events from r until one matches all assertions or an
// overall 5s timer elapses. Mirrors the goroutine+select shape used in
// ws_sse_integration_test.go, since bufio.Reader has no read-deadline hook.
func expectSSEEvent(r *bufio.Reader, assertions []BodyAssertion) error {
	type result struct {
		data string
		err  error
	}
	for {
		ch := make(chan result, 1)
		go func() {
			data, err := nextSSEData(r)
			ch <- result{data: data, err: err}
		}()
		select {
		case res := <-ch:
			if res.err != nil {
				return fmt.Errorf("sse expect: %w", res.err)
			}
			matched, err := matchMessage([]byte(res.data), assertions)
			if err != nil {
				return err
			}
			if matched {
				return nil
			}
		case <-time.After(5 * time.Second):
			return fmt.Errorf("sse expect: timeout waiting for matching event")
		}
	}
}

// buildMultipart renders a MultipartSpec into a body and content type, with
// ${var} substitution applied to field values and text file contents.
func buildMultipart(spec *MultipartSpec, vars map[string]string) (string, *bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for name, val := range spec.Fields {
		if err := w.WriteField(name, Substitute(val, vars)); err != nil {
			return "", nil, err
		}
	}
	for _, f := range spec.Files {
		field := f.Field
		if field == "" {
			field = "file"
		}
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, f.Filename))
		if f.ContentType != "" {
			hdr.Set("Content-Type", f.ContentType)
		}
		part, err := w.CreatePart(hdr)
		if err != nil {
			return "", nil, err
		}
		var data []byte
		if f.ContentBase64 != "" {
			data, err = base64.StdEncoding.DecodeString(f.ContentBase64)
			if err != nil {
				return "", nil, fmt.Errorf("file %q: %w", f.Filename, err)
			}
		} else {
			data = []byte(Substitute(f.Content, vars))
		}
		if _, err := part.Write(data); err != nil {
			return "", nil, err
		}
	}
	if err := w.Close(); err != nil {
		return "", nil, err
	}
	return w.FormDataContentType(), buf, nil
}

// mailPollDeadline bounds how long checkMail polls Mailpit for a matching
// message. A package var (rather than a constant) so the unit suite can
// override it to keep the negative-case test fast; production callers get
// the 5s default.
var mailPollDeadline = 5 * time.Second

// checkMail polls the Mailpit list API for a message matching the
// expectation. Matching is deliberately restricted to the list endpoint
// (GET /api/v1/messages): fetching individual messages would let BodyRegex
// match the full body, but message bodies can contain raw control bytes
// that break JSON decoding of the per-message endpoint, so BodyRegex is
// matched against the list response's Snippet field instead. Mailpit's
// MessageSummary does include Snippet (a plain-text preview), but if a
// given Mailpit version ever omits or empties it, an empty Snippet simply
// never matches a non-empty BodyRegex — treated as "not yet delivered"
// rather than a hard error, so the poll keeps retrying until the deadline.
//
// A transient error fetching or decoding the list response (e.g. Mailpit
// still starting up) does not abort the poll — it is treated the same as
// "not yet delivered" and retried until the deadline. An invalid
// BodyRegex, by contrast, can never succeed on any retry, so it is
// compiled once up front and reported immediately.
func checkMail(apiBase string, want MailExpect) error {
	type message struct {
		Subject string `json:"Subject"`
		Snippet string `json:"Snippet"`
		To      []struct {
			Address string `json:"Address"`
		} `json:"To"`
	}

	var bodyRegex *regexp.Regexp
	if want.BodyRegex != "" {
		re, err := regexp.Compile(want.BodyRegex)
		if err != nil {
			return fmt.Errorf("mail body_regex: %w", err)
		}
		bodyRegex = re
	}

	var last []message
	deadline := time.Now().Add(mailPollDeadline)
	for time.Now().Before(deadline) {
		resp, err := http.Get(apiBase + "/api/v1/messages")
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var out struct {
			Messages []message `json:"messages"`
		}
		err = json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		last = out.Messages
		for _, m := range out.Messages {
			if m.Subject != want.Subject {
				continue
			}
			toMatch := false
			for _, to := range m.To {
				if to.Address == want.To {
					toMatch = true
				}
			}
			if !toMatch {
				continue
			}
			if bodyRegex != nil && !bodyRegex.MatchString(m.Snippet) {
				continue
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("no message to %q with subject %q found (inbox: %v)", want.To, want.Subject, last)
}
