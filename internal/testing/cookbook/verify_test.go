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
		"unknown field":  `{"steps": [], "bogus": 1}`,
		"no steps":       `{"deps": []}`,
		"missing name":   `{"steps": [{"request": {"method": "GET", "path": "/x"}, "expect": {"status": 200}}]}`,
		"missing method": `{"steps": [{"name": "a", "request": {"path": "/x"}, "expect": {"status": 200}}]}`,
		"missing path":   `{"steps": [{"name": "a", "request": {"method": "GET"}, "expect": {"status": 200}}]}`,
		"missing status": `{"steps": [{"name": "a", "request": {"method": "GET", "path": "/x"}, "expect": {}}]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSuite(writeSuite(t, content))
			assert.Error(t, err)
		})
	}
}

func TestLoadSuiteSeedMultipartMail(t *testing.T) {
	p := writeSuite(t, `{
	  "deps": ["postgres"],
	  "seed": {"images/in.png": "files/in.png"},
	  "steps": [
	    {
	      "name": "upload",
	      "request": {
	        "method": "POST", "path": "/api/upload",
	        "multipart": {
	          "fields": {"note": "hello"},
	          "files": [{"field": "file", "filename": "in.png", "content_type": "image/png", "content_base64": "aGk="}]
	        }
	      },
	      "expect": {"status": 201, "body": [{"path": "path", "exists": true}]}
	    },
	    {
	      "name": "invitation arrives",
	      "mail": {"to": "bob@example.com", "subject": "Welcome", "body_regex": "hi"}
	    }
	  ]
	}`)
	s, err := LoadSuite(p)
	require.NoError(t, err)
	assert.Equal(t, "files/in.png", s.Seed["images/in.png"])
	require.Len(t, s.Steps, 2)
	require.NotNil(t, s.Steps[0].Request.Multipart)
	assert.Equal(t, "aGk=", s.Steps[0].Request.Multipart.Files[0].ContentBase64)
	require.NotNil(t, s.Steps[1].Mail)
	assert.Equal(t, "Welcome", s.Steps[1].Mail.Subject)
}

func TestLoadSuiteRejectsNewShapes(t *testing.T) {
	cases := map[string]string{
		"mail and request together": `{"steps": [{"name": "a", "mail": {"to": "x@y", "subject": "s"}, "request": {"method": "GET", "path": "/x"}, "expect": {"status": 200}}]}`,
		"mail missing subject":      `{"steps": [{"name": "a", "mail": {"to": "x@y"}}]}`,
		"mail missing to":           `{"steps": [{"name": "a", "mail": {"subject": "s"}}]}`,
		"neither mail nor request":  `{"steps": [{"name": "a"}]}`,
		"multipart with body":       `{"steps": [{"name": "a", "request": {"method": "POST", "path": "/x", "body": {"k": 1}, "multipart": {"fields": {"f": "v"}}}, "expect": {"status": 200}}]}`,
		"file content both":         `{"steps": [{"name": "a", "request": {"method": "POST", "path": "/x", "multipart": {"files": [{"filename": "f.txt", "content": "x", "content_base64": "eA=="}]}}, "expect": {"status": 200}}]}`,
		"file content neither":      `{"steps": [{"name": "a", "request": {"method": "POST", "path": "/x", "multipart": {"files": [{"filename": "f.txt"}]}}, "expect": {"status": 200}}]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSuite(writeSuite(t, content))
			assert.Error(t, err)
		})
	}
}
