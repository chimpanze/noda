package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeneratedRouteTriggersUseCanonicalNamespace ensures the scaffold and
// example generators emit canonical top-level params/body in route trigger
// inputs, rather than the legacy `request.*` alias. WebSocket connection
// `channels.pattern` fields are a different runtime context where `request.*`
// remains legitimately valid and must NOT be touched by this check.
func TestGeneratedRouteTriggersUseCanonicalNamespace(t *testing.T) {
	require.NotContains(t, scaffoldSampleRoute, "request.",
		"scaffold route trigger must use canonical params/body")

	for name, config := range examplePatterns {
		for key, value := range config {
			if key == "connections" {
				// Connection configs (e.g. channels.pattern) legitimately use
				// request.* in a different runtime context; skip them.
				continue
			}
			if strings.Contains(value, "request.") {
				t.Errorf("example %q field %q must not reference request.* (found in generated route/workflow content): %s", name, key, value)
			}
		}
	}

	// Sanity: confirm the WS channels.pattern (the one legitimate exception)
	// still uses request.* so we know the exemption above is doing real work.
	wsConnections := examplePatterns["websocket"]["connections"]
	assert.Contains(t, wsConnections, "request.",
		"websocket channels.pattern is expected to retain request.* (different runtime context)")
}

// TestExamples_JWTSignAndRefAreValid ensures the auth example's jwt_sign node
// config matches the real util.jwt_sign runtime contract (required secret,
// "expiry" key not "expires_in"), and the crud example's schema ref uses the
// real object form rather than the nonexistent "$ref(...)" string form.
func TestExamples_JWTSignAndRefAreValid(t *testing.T) {
	// The hand-rolled JWT sign_token node now lives under the "alternative_workflow"
	// key (see #377/#378): the primary "workflow" key leads with the built-in auth
	// plugin (auth.verify_credentials / auth.create_session), which has no jwt_sign
	// node at all.
	authWorkflow := examplePatterns["auth"]["alternative_workflow"]
	require.NotContains(t, authWorkflow, `"expires_in"`, "jwt_sign uses 'expiry', not 'expires_in'")
	require.Contains(t, authWorkflow, `"expiry"`)
	require.Contains(t, authWorkflow, "secrets.JWT_SECRET")

	crudWorkflow := examplePatterns["crud"]["workflow"]
	require.NotContains(t, crudWorkflow, "$ref(", "schema refs must use the object form {\"$ref\": \"schemas/Name\"}, not the $ref(...) string form")
}

// exampleWorkflowEdge/exampleWorkflow mirror just enough of the real workflow
// JSON shape (nodes keyed by type, edges with from/to/output) to walk every
// embedded example for structural checks without depending on the real
// internal/config types.
type exampleWorkflowEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Output string `json:"output"`
}

type exampleWorkflow struct {
	Nodes map[string]struct {
		Type string `json:"type"`
	} `json:"nodes"`
	Edges []exampleWorkflowEdge `json:"edges"`
}

// TestExamples_NoBooleanControlIfEdges is the #378 regression test: control.if's
// real outputs are "then"/"else"/"error" (see docs/03-nodes/control.if.md), not
// "true"/"false". The new edge-validation on this branch rejects "true"/"false"
// outputs outright, so any embedded example still using them would fail to
// validate. Walk every JSON-shaped string value across every example pattern,
// unmarshal it as a workflow, and assert no edge output is literally "true" or
// "false".
func TestExamples_NoBooleanControlIfEdges(t *testing.T) {
	for name, config := range examplePatterns {
		for key, value := range config {
			trimmed := strings.TrimSpace(value)
			if !strings.HasPrefix(trimmed, "{") {
				continue // not JSON (e.g. SQL migrations, prose descriptions)
			}
			var wf exampleWorkflow
			if err := json.Unmarshal([]byte(trimmed), &wf); err != nil {
				continue // not a workflow-shaped document (e.g. noda.json/route/connections snippets)
			}
			if len(wf.Edges) == 0 {
				continue
			}
			for _, edge := range wf.Edges {
				if edge.Output == "true" || edge.Output == "false" {
					t.Errorf("example %q field %q has edge %s->%s with boolean output %q; control.if outputs are then/else/error",
						name, key, edge.From, edge.To, edge.Output)
				}
			}
		}
	}
}

// TestExamples_AllJSONFieldsUnmarshal is the Step 4 sanity check: every
// JSON-shaped embedded example string must at least unmarshal as valid JSON.
func TestExamples_AllJSONFieldsUnmarshal(t *testing.T) {
	for name, config := range examplePatterns {
		for key, value := range config {
			trimmed := strings.TrimSpace(value)
			if !strings.HasPrefix(trimmed, "{") {
				continue // not JSON (e.g. SQL migrations, prose descriptions)
			}
			var v any
			require.NoErrorf(t, json.Unmarshal([]byte(trimmed), &v),
				"example %q field %q must be valid JSON", name, key)
		}
	}
}

// TestExamples_AuthPrimaryLeadsWithPlugin is the #377 regression test: the
// auth example must lead with the built-in auth plugin, not a hand-rolled JWT
// implementation with its own users table.
func TestExamples_AuthPrimaryLeadsWithPlugin(t *testing.T) {
	auth := examplePatterns["auth"]
	all := strings.Join([]string{
		auth["description"],
		auth["service_config"],
		auth["route"],
		auth["workflow"],
		auth["register_route"],
		auth["register_workflow"],
	}, "\n")

	require.Contains(t, all, "services.auth", "must show the auth service config key")
	require.Contains(t, all, `"plugin": "auth"`, "must configure the built-in auth plugin")
	require.Contains(t, all, "database", "must point the auth service at a database service")
	require.Contains(t, all, "noda auth init", "must point users at the scaffolding command")
	require.Contains(t, all, "auth_users", "must name the table the plugin owns")

	require.Contains(t, auth["workflow"], "auth.verify_credentials",
		"primary login workflow must use the built-in verify_credentials node")
	require.Contains(t, auth["register_workflow"], "auth.create_user",
		"primary register workflow must use the built-in create_user node")
}

// TestExamples_AuthAlternativeIsLabeled is the #377 regression test for the
// hand-rolled JWT variant: it must be clearly marked as an alternative,
// incompatible pattern rather than presented as equally-valid guidance.
func TestExamples_AuthAlternativeIsLabeled(t *testing.T) {
	auth := examplePatterns["auth"]
	label := auth["alternative_description"]
	require.Contains(t, label, "ALTERNATIVE")
	require.Contains(t, label, "incompatible")
	require.Contains(t, label, "#377")
}
