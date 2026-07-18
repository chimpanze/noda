package cookbook

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doc(t *testing.T, s string) any {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(s), &v))
	return v
}

func boolPtr(b bool) *bool { return &b }

func TestLookupPath(t *testing.T) {
	d := doc(t, `{"a": {"b": [{"c": 7}]}, "s": "x"}`)

	v, ok := LookupPath(d, "a.b.0.c")
	require.True(t, ok)
	assert.Equal(t, float64(7), v)

	v, ok = LookupPath(d, "s")
	require.True(t, ok)
	assert.Equal(t, "x", v)

	_, ok = LookupPath(d, "a.missing")
	assert.False(t, ok)

	_, ok = LookupPath(d, "a.b.5")
	assert.False(t, ok)
}

func TestCheckAssertion(t *testing.T) {
	d := doc(t, `{"id": "abc-123", "n": 3, "ok": true, "items": [1, 2], "obj": {"k": 1}, "nil": null}`)

	// equals: JSON-normalized comparison
	assert.NoError(t, CheckAssertion(d, BodyAssertion{Path: "n", Equals: 3}))
	assert.NoError(t, CheckAssertion(d, BodyAssertion{Path: "id", Equals: "abc-123"}))
	assert.Error(t, CheckAssertion(d, BodyAssertion{Path: "n", Equals: 4}))

	// regex
	assert.NoError(t, CheckAssertion(d, BodyAssertion{Path: "id", Regex: `^abc-\d+$`}))
	assert.Error(t, CheckAssertion(d, BodyAssertion{Path: "id", Regex: `^zzz`}))

	// exists
	assert.NoError(t, CheckAssertion(d, BodyAssertion{Path: "id", Exists: boolPtr(true)}))
	assert.NoError(t, CheckAssertion(d, BodyAssertion{Path: "gone", Exists: boolPtr(false)}))
	assert.Error(t, CheckAssertion(d, BodyAssertion{Path: "gone", Exists: boolPtr(true)}))

	// type
	assert.NoError(t, CheckAssertion(d, BodyAssertion{Path: "n", Type: "number"}))
	assert.NoError(t, CheckAssertion(d, BodyAssertion{Path: "items", Type: "array"}))
	assert.NoError(t, CheckAssertion(d, BodyAssertion{Path: "obj", Type: "object"}))
	assert.NoError(t, CheckAssertion(d, BodyAssertion{Path: "ok", Type: "boolean"}))
	assert.NoError(t, CheckAssertion(d, BodyAssertion{Path: "nil", Type: "null"}))
	assert.Error(t, CheckAssertion(d, BodyAssertion{Path: "n", Type: "string"}))

	// exactly one matcher required
	assert.Error(t, CheckAssertion(d, BodyAssertion{Path: "n"}))
	assert.Error(t, CheckAssertion(d, BodyAssertion{Path: "n", Equals: 3, Regex: "3"}))
}

func TestSubstituteAndCapture(t *testing.T) {
	vars := map[string]string{}
	d := doc(t, `{"id": "abc-123", "n": 7}`)

	require.NoError(t, Capture(d, map[string]string{"thing_id": "body.id", "count": "body.n"}, vars))
	assert.Equal(t, "abc-123", vars["thing_id"])
	assert.Equal(t, "7", vars["count"])

	assert.Equal(t, "/api/things/abc-123", Substitute("/api/things/${thing_id}", vars))
	assert.Equal(t, "no vars here", Substitute("no vars here", vars))
	// unknown variables are left intact (visible in failure output)
	assert.Equal(t, "${nope}", Substitute("${nope}", vars))

	assert.Error(t, Capture(d, map[string]string{"x": "body.missing"}, vars))
	assert.Error(t, Capture(d, map[string]string{"x": "header.X"}, vars), "only body.* capture sources exist in this tranche")
}
