package cookbook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSuite(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "verify.json")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestLoadSuite(t *testing.T) {
	p := writeSuite(t, `{
	  "deps": [],
	  "steps": [
	    {
	      "name": "create",
	      "request": {"method": "POST", "path": "/api/things", "body": {"n": 1}},
	      "expect": {"status": 201, "body": [{"path": "id", "exists": true}]},
	      "capture": {"thing_id": "body.id"}
	    }
	  ]
	}`)
	s, err := LoadSuite(p)
	require.NoError(t, err)
	assert.Empty(t, s.Deps)
	require.Len(t, s.Steps, 1)
	assert.Equal(t, "create", s.Steps[0].Name)
	assert.Equal(t, "POST", s.Steps[0].Request.Method)
	assert.Equal(t, 201, s.Steps[0].Expect.Status)
	assert.Equal(t, "body.id", s.Steps[0].Capture["thing_id"])
}

func TestLoadSuiteRejects(t *testing.T) {
	cases := map[string]string{
		"unknown field":   `{"steps": [], "bogus": 1}`,
		"no steps":        `{"deps": []}`,
		"missing name":    `{"steps": [{"request": {"method": "GET", "path": "/x"}, "expect": {"status": 200}}]}`,
		"missing method":  `{"steps": [{"name": "a", "request": {"path": "/x"}, "expect": {"status": 200}}]}`,
		"missing path":    `{"steps": [{"name": "a", "request": {"method": "GET"}, "expect": {"status": 200}}]}`,
		"missing status":  `{"steps": [{"name": "a", "request": {"method": "GET", "path": "/x"}, "expect": {}}]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSuite(writeSuite(t, content))
			assert.Error(t, err)
		})
	}
}
