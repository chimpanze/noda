package cookbook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
	err = checkMail(stub.URL, MailExpect{To: "carol@example.com", Subject: "Welcome"})
	assert.Error(t, err)
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
