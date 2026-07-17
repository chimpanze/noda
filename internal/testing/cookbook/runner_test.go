package cookbook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/chimpanze/noda/plugins/core/transform"
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

func TestRunProjectRejectsDeps(t *testing.T) {
	dir := writeProject(t)
	p := filepath.Join(dir, "verify.json")
	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	patched := strings.Replace(string(raw), `"deps": []`, `"deps": ["postgres"]`, 1)
	require.NotEqual(t, string(raw), patched, "expected to find deps: [] to patch")
	require.NoError(t, os.WriteFile(p, []byte(patched), 0o644))

	failed := runProjectRecorded(t, dir, testPlugins())
	require.True(t, failed, "non-empty deps must fail in this tranche")
}
