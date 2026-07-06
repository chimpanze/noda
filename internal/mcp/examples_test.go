package mcp

import (
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
	authWorkflow := examplePatterns["auth"]["workflow"]
	require.NotContains(t, authWorkflow, `"expires_in"`, "jwt_sign uses 'expiry', not 'expires_in'")
	require.Contains(t, authWorkflow, `"expiry"`)
	require.Contains(t, authWorkflow, "secrets.JWT_SECRET")

	crudWorkflow := examplePatterns["crud"]["workflow"]
	require.NotContains(t, crudWorkflow, "$ref(", "schema refs must use the object form {\"$ref\": \"schemas/Name\"}, not the $ref(...) string form")
}
