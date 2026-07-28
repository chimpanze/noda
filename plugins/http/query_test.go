package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeQuery(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want string
	}{
		{"nil input", nil, ""},
		{"empty map", map[string]any{}, ""},
		{"single string", map[string]any{"q": "hello"}, "q=hello"},
		{"number stringified", map[string]any{"limit": 10}, "limit=10"},
		{"float stringified", map[string]any{"ratio": 1.5}, "ratio=1.5"},
		{"bool stringified", map[string]any{"active": true}, "active=true"},
		// Sorted by key: url.Values.Encode sorts, so output is deterministic.
		{"keys sorted", map[string]any{"b": "2", "a": "1", "c": "3"}, "a=1&b=2&c=3"},
		// The bug this field exists to remove: hand-concatenation does not encode.
		{"value with reserved chars", map[string]any{"note": "a&b=c d"}, "note=a%26b%3Dc+d"},
		{"key needing encoding", map[string]any{"a b": "1"}, "a+b=1"},
		// Nulls are omitted entirely, so an optional param needs no conditional.
		{"null omitted", map[string]any{"cursor": nil, "limit": 10}, "limit=10"},
		{"all-null map encodes empty", map[string]any{"cursor": nil}, ""},
		// Arrays become repeated params, in element order.
		{"array repeated", map[string]any{"tag": []any{"new", "sale"}}, "tag=new&tag=sale"},
		{"array element order preserved", map[string]any{"t": []any{"z", "a"}}, "t=z&t=a"},
		{"empty array contributes nothing", map[string]any{"tag": []any{}}, ""},
		{"nil element skipped", map[string]any{"tag": []any{"new", nil, "sale"}}, "tag=new&tag=sale"},
		{"all-nil array contributes nothing", map[string]any{"tag": []any{nil}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeQuery(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEncodeQuery_Errors(t *testing.T) {
	tests := []struct {
		name    string
		raw     any
		wantMsg string
	}{
		{"not a map", "limit=5", "must resolve to an object"},
		{"number instead of map", 42, "must resolve to an object"},
		{"nested object value", map[string]any{"f": map[string]any{"a": 1}}, `query value for "f" must be a scalar`},
		{"nested array value", map[string]any{"f": []any{[]any{"x"}}}, `query value for "f" must be a scalar`},
		{"object inside array", map[string]any{"f": []any{"ok", map[string]any{"a": 1}}}, `query value for "f" must be a scalar`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encodeQuery(tt.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// Determinism matters here for the same reason it did in #450 and #460:
// url.Values.Encode sorts keys, so repeated calls must agree exactly.
func TestEncodeQuery_Deterministic(t *testing.T) {
	raw := map[string]any{"d": "4", "a": "1", "c": "3", "b": "2", "e": "5"}
	first, err := encodeQuery(raw)
	require.NoError(t, err)
	for range 50 {
		got, err := encodeQuery(raw)
		require.NoError(t, err)
		require.Equal(t, first, got)
	}
}

// The query field must reach the wire URL-encoded. httptest gives us the raw
// RequestURI, which is the only place the encoding is observable.
func TestDoRequest_QueryReachesTheWire(t *testing.T) {
	var gotRawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"cursor": nil}))
	config := map[string]any{
		"url": ts.URL + "/items",
		"query": map[string]any{
			"limit":  10,
			"note":   "a&b c",
			"cursor": "{{ input.cursor }}",
		},
	}

	output, _, err := doRequest(context.Background(), execCtx, config, newTestService(), "GET")
	require.NoError(t, err)
	assert.Equal(t, api.OutputSuccess, output)
	// Sorted, encoded, and cursor omitted because it resolved to nil.
	assert.Equal(t, "limit=10&note=a%26b+c", gotRawQuery)
}

// A whole-map expression is the wholesale-forwarding shape the proxy cookbook
// documents: the route maps {{ query }} into input, the node reads input.*.
func TestDoRequest_QueryFromWholeMapExpression(t *testing.T) {
	var gotRawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"q": map[string]any{"sort": "name", "limit": "5"},
	}))
	config := map[string]any{"url": ts.URL + "/items", "query": "{{ input.q }}"}

	_, _, err := doRequest(context.Background(), execCtx, config, newTestService(), "GET")
	require.NoError(t, err)
	assert.Equal(t, "limit=5&sort=name", gotRawQuery)
}

// The merge rule: a url that already carries a query string is rejected rather
// than merged, so there is no precedence to guess at.
func TestDoRequest_QueryConflictsWithURLQueryString(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	config := map[string]any{
		"url":   "http://example.invalid/items?sort=name",
		"query": map[string]any{"limit": 10},
	}

	_, _, err := doRequest(context.Background(), execCtx, config, newTestService(), "GET")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has a query string")
	assert.Contains(t, err.Error(), "sort=name", "the error must name the offending url")
}

// An empty query must NOT trigger the conflict rule — it appends nothing, so
// there is nothing to conflict with.
func TestDoRequest_EmptyQueryDoesNotConflict(t *testing.T) {
	var gotRawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	config := map[string]any{
		"url":   ts.URL + "/items?sort=name",
		"query": map[string]any{"cursor": nil},
	}

	_, _, err := doRequest(context.Background(), execCtx, config, newTestService(), "GET")
	require.NoError(t, err, "an all-null query contributes nothing and must not conflict")
	assert.Equal(t, "sort=name", gotRawQuery)
}

// No query field at all must leave the url untouched, including its own "?".
func TestDoRequest_NoQueryFieldLeavesURLAlone(t *testing.T) {
	var gotRawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	config := map[string]any{"url": ts.URL + "/items?sort=name"}

	_, _, err := doRequest(context.Background(), execCtx, config, newTestService(), "GET")
	require.NoError(t, err)
	assert.Equal(t, "sort=name", gotRawQuery)
}

// The conflict check runs after base_url joining, so a base_url carrying its
// own query string is caught instead of producing a malformed URL.
func TestDoRequest_QueryConflictsWithBaseURLQueryString(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	svc := newTestService()
	svc.baseURL = "http://example.invalid/v1?apikey=abc"
	config := map[string]any{"url": "/items", "query": map[string]any{"limit": 10}}

	_, _, err := doRequest(context.Background(), execCtx, config, svc, "GET")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has a query string")
	assert.Contains(t, err.Error(), "apikey=abc", "the joined url must be named, not just the url field")
}
