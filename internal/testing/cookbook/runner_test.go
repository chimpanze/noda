package cookbook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeProject lays a minimal cookbook project into a temp dir.
func writeProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"noda.json": `{"server": {"port": 3000}, "services": {}}`,
		"routes/echo.json": `{
		  "id": "echo",
		  "method": "POST",
		  "path": "/api/echo",
		  "trigger": {"workflow": "echo", "input": {"name": "{{ body.name }}"}}
		}`,
		"workflows/echo.json": `{
		  "id": "echo",
		  "nodes": {
		    "greet": {"type": "transform.set", "config": {"fields": {"greeting": "Hello, {{ input.name }}!"}}},
		    "respond": {"type": "response.json", "config": {"status": 200, "body": {"greeting": "{{ nodes.greet.greeting }}", "id": "{{ $uuid() }}"}}}
		  },
		  "edges": [{"from": "greet", "to": "respond"}]
		}`,
		"verify.json": `{
		  "deps": [],
		  "steps": [
		    {
		      "name": "greets by name",
		      "request": {"method": "POST", "path": "/api/echo", "body": {"name": "World"}},
		      "expect": {"status": 200, "body": [
		        {"path": "greeting", "equals": "Hello, World!"},
		        {"path": "id", "regex": "^[0-9a-f-]{36}$"}
		      ]},
		      "capture": {"echo_id": "body.id"}
		    },
		    {
		      "name": "captured variable substitutes",
		      "request": {"method": "POST", "path": "/api/echo", "body": {"name": "${echo_id}"}},
		      "expect": {"status": 200, "body": [{"path": "greeting", "regex": "^Hello, [0-9a-f-]{36}!$"}]}
		    }
		  ]
		}`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return dir
}

func testPlugins() []api.Plugin {
	return []api.Plugin{&transform.Plugin{}, &response.Plugin{}}
}

func TestRunProject(t *testing.T) {
	RunProject(t, writeProject(t), testPlugins())
}

func TestSubstituteBodyEscapesSpecialCharacters(t *testing.T) {
	vars := map[string]string{"x": "he said \"hi\\\"\nnew line"}
	body := map[string]any{
		"msg":    "${x}",
		"nested": map[string]any{"list": []any{"${x}", float64(3), true, nil}},
	}

	raw, err := json.Marshal(substituteBody(body, vars))
	require.NoError(t, err, "substituted body must marshal to valid JSON")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded), "marshaled body must round-trip")
	require.Equal(t, vars["x"], decoded["msg"], "quote/backslash/newline must survive intact")
	nested := decoded["nested"].(map[string]any)
	list := nested["list"].([]any)
	require.Equal(t, vars["x"], list[0])
	require.Equal(t, float64(3), list[1])
	require.Equal(t, true, list[2])
	require.Nil(t, list[3])

	// The original body must not be mutated.
	require.Equal(t, "${x}", body["msg"])
}

// depsSuite is a verify.json body that declares a non-empty deps list.
const depsSuite = `{
  "deps": ["postgres"],
  "steps": [
    {
      "name": "never runs",
      "request": {"method": "POST", "path": "/api/echo", "body": {"name": "x"}},
      "expect": {"status": 200}
    }
  ]
}`

// TestRunProjectDepsRequireEnv asserts that a suite declaring deps is
// rejected when RunProject is called without Options.Env — the signal that
// distinguishes a direct unit-test invocation from a run through the
// integration walker (which always supplies Env for service-backed
// families). Without that signal, deps-declaring projects would silently
// run without their required services.
func TestRunProjectDepsRequireEnv(t *testing.T) {
	dir := writeProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "verify.json"), []byte(depsSuite), 0o644))

	failed, errMsg := runProjectRecorded(t, dir, testPlugins())
	require.True(t, failed, "non-empty deps without Options.Env must fail")
	assert.Contains(t, errMsg, "declared but no environment provided")
}

// TestRunProjectDepsAllowedWithEnv asserts that supplying Options.Env clears
// the deps guard: the (deps-declaring but otherwise service-less) test
// project runs to completion instead of being rejected up front.
func TestRunProjectDepsAllowedWithEnv(t *testing.T) {
	dir := writeProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "verify.json"), []byte(depsSuite), 0o644))

	failed, errMsg := runProjectRecorded(t, dir, testPlugins(), Options{Env: map[string]string{"DUMMY": "1"}})
	assert.False(t, failed, "supplying Env must clear the deps guard: %s", errMsg)
}

func TestRunProjectSeedsAndDataDir(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"noda.json":       `{"server": {"port": 3000}, "services": {}}`,
		"files/hello.txt": "seeded-content",
		"routes/peek.json": `{
		  "id": "peek", "method": "GET", "path": "/api/peek",
		  "trigger": {"workflow": "peek"}
		}`,
		"workflows/peek.json": `{
		  "id": "peek",
		  "nodes": {"respond": {"type": "response.json", "config": {"status": 200, "body": {"ok": true}}}},
		  "edges": []
		}`,
		"verify.json": `{
		  "seed": {"data/hello.txt": "files/hello.txt"},
		  "steps": [{"name": "up", "request": {"method": "GET", "path": "/api/peek"}, "expect": {"status": 200}}]
		}`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}

	RunProject(t, dir, testPlugins())

	dataDir := os.Getenv("COOKBOOK_DATA_DIR")
	require.NotEmpty(t, dataDir)
	got, err := os.ReadFile(filepath.Join(dataDir, "data", "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "seeded-content", string(got))
}

func TestMailStepAgainstStubMailpit(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/messages", r.URL.Path)
		_, _ = w.Write([]byte(`{"total": 1, "messages": [{"Subject": "Welcome", "To": [{"Address": "bob@example.com"}], "Snippet": "hi bob"}]}`))
	}))
	defer stub.Close()

	err := checkMail(stub.URL, MailExpect{To: "bob@example.com", Subject: "Welcome", BodyRegex: "hi"})
	assert.NoError(t, err)

	// Override the poll deadline for the negative case so the unit suite
	// stays fast — the production default (5s) is only exercised by the
	// integration-tagged mailpit tests.
	origDeadline := mailPollDeadline
	mailPollDeadline = 300 * time.Millisecond
	defer func() { mailPollDeadline = origDeadline }()

	err = checkMail(stub.URL, MailExpect{To: "carol@example.com", Subject: "Welcome"})
	assert.Error(t, err)
}

func TestCheckMailInvalidRegexFailsFast(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"total": 0, "messages": []}`))
	}))
	defer stub.Close()

	start := time.Now()
	err := checkMail(stub.URL, MailExpect{To: "bob@example.com", Subject: "Welcome", BodyRegex: "("})
	assert.Error(t, err)
	assert.Less(t, time.Since(start), time.Second, "invalid regex must be reported immediately, not after a poll timeout")
}

// TestRunProjectListenMode proves the listen-mode transport switch: a real
// TCP listener is reserved, the server is dialed over it via runStepHTTP
// (not the in-process Fiber test transport runStep uses), and
// COOKBOOK_BASE_URL is exported before config load.
//
// Note: {{ $env('COOKBOOK_BASE_URL') }} cannot be used inside a workflow
// node's config to prove the export, because $env() resolution is scoped
// to the root config document only (internal/secrets/resolve.go: "Only
// meant for the root config (not routes/workflows)") — workflows are a
// separate config section that never goes through sm.Resolve. So the
// export is instead verified directly against the process environment
// after RunProject returns.
func TestRunProjectListenMode(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"noda.json": `{"server": {"port": 3000}, "services": {}}`,
		"routes/base.json": `{
		  "id": "base", "method": "GET", "path": "/api/base",
		  "trigger": {"workflow": "base"}
		}`,
		"workflows/base.json": `{
		  "id": "base",
		  "nodes": {"respond": {"type": "response.json", "config": {"status": 200, "body": {"ok": true}}}},
		  "edges": []
		}`,
		"verify.json": `{
		  "listen": true,
		  "steps": [{"name": "serves over tcp", "request": {"method": "GET", "path": "/api/base"},
		    "expect": {"status": 200, "body": [{"path": "ok", "equals": true}]}}]
		}`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	RunProject(t, dir, testPlugins())

	baseURL := os.Getenv("COOKBOOK_BASE_URL")
	require.NotEmpty(t, baseURL, "COOKBOOK_BASE_URL must be exported for listen-mode suites")
	assert.Regexp(t, `^http://127\.0\.0\.1:\d+$`, baseURL)
}

// TestWithRetry unit-tests the shared retry loop both transports (runStep
// and runStepHTTP) delegate to, so retry_timeout semantics can't diverge
// between listen-mode and in-process suites.
func TestWithRetry(t *testing.T) {
	t.Run("no timeout runs once", func(t *testing.T) {
		calls := 0
		err := withRetry("", func() error {
			calls++
			return assert.AnError
		})
		assert.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, 1, calls, "empty retry_timeout must not retry")
	})

	t.Run("succeeds on third attempt", func(t *testing.T) {
		calls := 0
		err := withRetry("5s", func() error {
			calls++
			if calls < 3 {
				return assert.AnError
			}
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, 3, calls)
	})

	t.Run("times out with last error", func(t *testing.T) {
		attempt := 0
		err := withRetry("1ms", func() error {
			attempt++
			return fmt.Errorf("attempt %d failed", attempt)
		})
		require.Error(t, err)
		assert.Equal(t, fmt.Sprintf("attempt %d failed", attempt), err.Error(),
			"the LAST attempt's error must be reported")
	})

	t.Run("invalid duration", func(t *testing.T) {
		err := withRetry("bogus", func() error { return nil })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid retry_timeout")
	})
}

func TestRetryTimeoutPolls(t *testing.T) {
	// Serve a counter endpoint that returns ready=false twice, then true.
	// Simplest: a workflow can't count — use a stub http server instead to
	// unit-test runStepHTTP's retry directly against a flaky handler.
	hits := 0
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		if hits < 3 {
			_, _ = w.Write([]byte(`{"ready": false}`))
			return
		}
		_, _ = w.Write([]byte(`{"ready": true}`))
	}))
	defer stub.Close()

	step := Step{
		Name:    "poll",
		Request: RequestSpec{Method: "GET", Path: "/", RetryTimeout: "5s"},
		Expect:  ExpectSpec{Status: 200, Body: []BodyAssertion{{Path: "ready", Equals: true}}},
	}
	err := runStepHTTP(stub.URL, step, map[string]string{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, hits, 3)
}

func TestBuildMultipartBody(t *testing.T) {
	spec := &MultipartSpec{
		Fields: map[string]string{"note": "hello ${v}"},
		Files:  []FilePart{{Filename: "a.txt", ContentType: "text/plain", Content: "body-${v}"}},
	}
	contentType, body, err := buildMultipart(spec, map[string]string{"v": "X"})
	require.NoError(t, err)
	assert.Contains(t, contentType, "multipart/form-data; boundary=")
	s := body.String()
	assert.Contains(t, s, `name="note"`)
	assert.Contains(t, s, "hello X")
	assert.Contains(t, s, `filename="a.txt"`)
	assert.Contains(t, s, "body-X")
	assert.Contains(t, s, "Content-Type: text/plain")
}

func TestOptionsVarsPreSeed(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"noda.json": `{"server": {"port": 3000}, "services": {}}`,
		"routes/echo.json": `{
		  "id": "echo", "method": "POST", "path": "/api/echo",
		  "trigger": {"workflow": "echo", "input": {"v": "{{ body.v }}"}}
		}`,
		"workflows/echo.json": `{
		  "id": "echo",
		  "nodes": {"respond": {"type": "response.json", "config": {"status": 200, "body": {"v": "{{ input.v }}"}}}},
		  "edges": []
		}`,
		"verify.json": `{
		  "steps": [{"name": "seeded var substitutes", "request": {"method": "POST", "path": "/api/echo", "body": {"v": "${seeded}"}},
		    "expect": {"status": 200, "body": [{"path": "v", "equals": "from-options"}]}}]
		}`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	RunProject(t, dir, testPlugins(), Options{Vars: map[string]string{"seeded": "from-options"}})
}
