package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeCallToolRequest(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

func TestListNodesHandler(t *testing.T) {
	nodeReg := buildNodeRegistry()
	handler := listNodesHandler(nodeReg)

	t.Run("all nodes", func(t *testing.T) {
		req := makeCallToolRequest("noda_list_nodes", nil)
		result, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)

		data := parseTextResult(t, result)

		nodes := data["nodes"].([]any)
		assert.Greater(t, len(nodes), 30, "expected at least 30 node types")

		count := data["count"].(float64)
		assert.Equal(t, float64(len(nodes)), count)

		// Verify that at least some nodes have output_data
		hasOutputData := 0
		for _, n := range nodes {
			node := n.(map[string]any)
			if _, ok := node["output_data"]; ok {
				hasOutputData++
			}
		}
		assert.Greater(t, hasOutputData, 0, "expected at least some nodes to have output_data")
	})

	t.Run("filter by category", func(t *testing.T) {
		req := makeCallToolRequest("noda_list_nodes", map[string]any{"category": "db"})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		nodes := data["nodes"].([]any)
		assert.Greater(t, len(nodes), 0)
		for _, n := range nodes {
			node := n.(map[string]any)
			assert.Contains(t, node["type"].(string), "db.")
		}
	})

	t.Run("filter returns empty for unknown category", func(t *testing.T) {
		req := makeCallToolRequest("noda_list_nodes", map[string]any{"category": "nonexistent"})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		nodes := data["nodes"].([]any)
		assert.Empty(t, nodes)
	})
}

func TestGetNodeSchemaHandler(t *testing.T) {
	nodeReg := buildNodeRegistry()
	handler := getNodeSchemaHandler(nodeReg)

	t.Run("valid node type", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_node_schema", map[string]any{"node_type": "db.query"})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data := parseTextResult(t, result)
		assert.Equal(t, "db.query", data["node_type"])
		assert.NotEmpty(t, data["description"])
		assert.NotNil(t, data["outputs"])
		assert.NotNil(t, data["output_data"], "expected output_data for db.query")
		outputData := data["output_data"].(map[string]any)
		assert.NotEmpty(t, outputData["success"])
		assert.NotEmpty(t, outputData["error"])
	})

	t.Run("unknown node type", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_node_schema", map[string]any{"node_type": "nonexistent"})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestListFunctionsHandler(t *testing.T) {
	req := makeCallToolRequest("noda_list_functions", nil)
	result, err := listFunctionsHandler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	data := parseTextResult(t, result)
	count := int(data["count"].(float64))
	functions := data["functions"].([]any)
	assert.Equal(t, len(functions), count)

	// Should have Noda built-in (13) + expr-lang built-in (19) = 32
	assert.Equal(t, 32, count)

	// Build a name set and check a few expected functions
	nameSet := make(map[string]bool, len(functions))
	for _, f := range functions {
		fn := f.(map[string]any)
		name := fn["name"].(string)
		nameSet[name] = true
		// Every function should have all three fields
		assert.NotEmpty(t, fn["name"], "function missing name")
		assert.NotEmpty(t, fn["signature"], "function %s missing signature", name)
		assert.NotEmpty(t, fn["description"], "function %s missing description", name)
	}

	// Noda functions
	assert.True(t, nameSet["$uuid"], "missing $uuid")
	assert.True(t, nameSet["sha256"], "missing sha256")
	assert.True(t, nameSet["bcrypt_hash"], "missing bcrypt_hash")

	// expr-lang built-in functions
	assert.True(t, nameSet["len"], "missing len")
	assert.True(t, nameSet["contains"], "missing contains")
	assert.True(t, nameSet["filter"], "missing filter")
	assert.True(t, nameSet["keys"], "missing keys")

	// Verify sorted order
	for i := 1; i < len(functions); i++ {
		prev := functions[i-1].(map[string]any)["name"].(string)
		curr := functions[i].(map[string]any)["name"].(string)
		assert.True(t, prev <= curr, "functions not sorted: %q > %q", prev, curr)
	}
}

func TestGetConfigSchemaHandler(t *testing.T) {
	t.Run("valid type", func(t *testing.T) {
		for _, configType := range []string{"root", "route", "workflow", "worker", "schedule", "connections", "test"} {
			req := makeCallToolRequest("noda_get_config_schema", map[string]any{"config_type": configType})
			result, err := getConfigSchemaHandler(context.Background(), req)
			require.NoError(t, err, "config type: %s", configType)
			require.False(t, result.IsError, "config type: %s", configType)

			data := parseTextResult(t, result)
			assert.NotNil(t, data["schema"], "expected schema for %s", configType)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_config_schema", map[string]any{"config_type": "bogus"})
		result, err := getConfigSchemaHandler(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestGetServiceSchemaHandler(t *testing.T) {
	t.Run("auth plugin", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_service_schema", map[string]any{"plugin": "auth"})
		result, err := getServiceSchemaHandler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "\"database\"")
		assert.Contains(t, text, "\"required\"")
	})

	t.Run("db plugin resolved by prefix", func(t *testing.T) {
		// noda.json's "plugin" field accepts either the plugin's Name()
		// ("postgres") or its Prefix() ("db") — registry.GetByName falls back
		// to prefix lookup (internal/registry/plugins.go). This tool must
		// mirror that fallback so "db" (the prefix) resolves the same schema
		// as "postgres" (the name).
		req := makeCallToolRequest("noda_get_service_schema", map[string]any{"plugin": "db"})
		result, err := getServiceSchemaHandler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data := parseTextResult(t, result)
		assert.Equal(t, "postgres", data["name"], "response should name the canonical plugin name")
		assert.Equal(t, "db", data["prefix"])
		assert.NotNil(t, data["config_schema"])
	})

	t.Run("livekit plugin", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_service_schema", map[string]any{"plugin": "livekit"})
		result, err := getServiceSchemaHandler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "\"api_secret\"")
	})

	t.Run("all plugins", func(t *testing.T) {
		for _, args := range []map[string]any{nil, {"plugin": "all"}} {
			req := makeCallToolRequest("noda_get_service_schema", args)
			result, err := getServiceSchemaHandler(context.Background(), req)
			require.NoError(t, err)
			require.False(t, result.IsError)

			data := parseTextResult(t, result)
			plugins := data["plugins"].([]any)
			assert.GreaterOrEqual(t, len(plugins), 9, "expected at least 9 service-bearing plugins")
			for _, p := range plugins {
				entry := p.(map[string]any)
				assert.NotEmpty(t, entry["name"], "entry missing name")
				assert.NotEmpty(t, entry["prefix"], "entry missing prefix")
				assert.NotNil(t, entry["config_schema"], "entry %v missing config_schema", entry["name"])
			}
		}
	})

	t.Run("unknown plugin lists valid names", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_service_schema", map[string]any{"plugin": "nope"})
		result, err := getServiceSchemaHandler(context.Background(), req)
		require.NoError(t, err)
		require.True(t, result.IsError)

		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "nope")
		assert.Contains(t, text, "auth")
		assert.Contains(t, text, "livekit")
	})

	t.Run("service-less plugin returns helpful error", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_service_schema", map[string]any{"plugin": "core.control"})
		result, err := getServiceSchemaHandler(context.Background(), req)
		require.NoError(t, err)
		require.True(t, result.IsError)

		text := result.Content[0].(mcp.TextContent).Text
		assert.Contains(t, text, "no services")
	})
}

func TestValidateExpressionHandler(t *testing.T) {
	t.Run("valid expression", func(t *testing.T) {
		req := makeCallToolRequest("noda_validate_expression", map[string]any{"expression": "{{ input.name }}"})
		result, err := validateExpressionHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		assert.True(t, data["valid"].(bool))
	})

	t.Run("valid literal", func(t *testing.T) {
		req := makeCallToolRequest("noda_validate_expression", map[string]any{"expression": "hello world"})
		result, err := validateExpressionHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		assert.True(t, data["valid"].(bool))
	})

	t.Run("invalid expression", func(t *testing.T) {
		req := makeCallToolRequest("noda_validate_expression", map[string]any{"expression": "{{ invalid( }}"})
		result, err := validateExpressionHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		assert.False(t, data["valid"].(bool))
		assert.NotEmpty(t, data["error"])
	})

	t.Run("expression with variables", func(t *testing.T) {
		req := makeCallToolRequest("noda_validate_expression", map[string]any{"expression": "{{ input.name }}"})
		result, err := validateExpressionHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		assert.True(t, data["valid"].(bool))
		variables := toStringSlice(data["variables"])
		assert.Contains(t, variables, "input.name")
		functions := toStringSlice(data["functions"])
		assert.Empty(t, functions)
	})

	t.Run("expression with functions", func(t *testing.T) {
		req := makeCallToolRequest("noda_validate_expression", map[string]any{"expression": "{{ $uuid() }}"})
		result, err := validateExpressionHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		assert.True(t, data["valid"].(bool))
		variables := toStringSlice(data["variables"])
		assert.Empty(t, variables)
		functions := toStringSlice(data["functions"])
		assert.Contains(t, functions, "$uuid")
	})

	t.Run("expression with unknown function warning", func(t *testing.T) {
		req := makeCallToolRequest("noda_validate_expression", map[string]any{"expression": "{{ uuid() }}"})
		result, err := validateExpressionHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		warnings := toStringSlice(data["warnings"])
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "did you mean $uuid") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected warning suggesting $uuid, got: %v", warnings)
	})

	t.Run("expression with node references", func(t *testing.T) {
		req := makeCallToolRequest("noda_validate_expression", map[string]any{"expression": "{{ nodes.create.id }}"})
		result, err := validateExpressionHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		assert.True(t, data["valid"].(bool))
		variables := toStringSlice(data["variables"])
		assert.Contains(t, variables, "nodes.create.id")
	})

	t.Run("mixed expression", func(t *testing.T) {
		req := makeCallToolRequest("noda_validate_expression", map[string]any{
			"expression": "{{ bcrypt_verify(input.password, nodes.lookup.hash) }}",
		})
		result, err := validateExpressionHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		assert.True(t, data["valid"].(bool))
		variables := toStringSlice(data["variables"])
		assert.Contains(t, variables, "input.password")
		assert.Contains(t, variables, "nodes.lookup.hash")
		functions := toStringSlice(data["functions"])
		assert.Contains(t, functions, "bcrypt_verify")
	})
}

// toStringSlice converts a JSON array ([]any) to []string.
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, len(arr))
	for i, item := range arr {
		out[i] = item.(string)
	}
	return out
}

func TestGetExamplesHandler(t *testing.T) {
	t.Run("all patterns", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_examples", map[string]any{"pattern": "all"})
		result, err := getExamplesHandler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data := parseTextResult(t, result)
		assert.NotEmpty(t, data["available"])
		assert.NotEmpty(t, data["patterns"])
	})

	t.Run("specific pattern", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_examples", map[string]any{"pattern": "crud"})
		result, err := getExamplesHandler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data := parseTextResult(t, result)
		assert.Equal(t, "crud", data["pattern"])
	})

	t.Run("unknown pattern", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_examples", map[string]any{"pattern": "nonexistent"})
		result, err := getExamplesHandler(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("every example config snippet is valid JSON", func(t *testing.T) {
		for name, example := range examplePatterns {
			for key, val := range example {
				// "description"/"alternative_description" are prose; migration_*
				// fields are raw SQL, not JSON.
				if key == "description" || key == "alternative_description" || strings.HasPrefix(key, "migration") {
					continue
				}
				var parsed any
				assert.NoErrorf(t, json.Unmarshal([]byte(val), &parsed),
					"example %q field %q must be valid JSON", name, key)
			}
		}
	})

	t.Run("websocket example matches the connections schema shape", func(t *testing.T) {
		// #230: handlers must be workflow-id strings under an endpoints.<name> map,
		// not objects, and not the flat {id,type,path} shape.
		var conn map[string]any
		require.NoError(t, json.Unmarshal([]byte(examplePatterns["websocket"]["connections"]), &conn))

		endpoints, ok := conn["endpoints"].(map[string]any)
		require.True(t, ok, "connections must use an endpoints.<name> map")
		require.NotEmpty(t, endpoints)
		for epName, raw := range endpoints {
			ep := raw.(map[string]any)
			for _, h := range []string{"on_connect", "on_message", "on_disconnect"} {
				if v, present := ep[h]; present {
					_, isStr := v.(string)
					assert.Truef(t, isStr, "endpoint %q handler %q must be a workflow-id string", epName, h)
				}
			}
		}
		// sync.pubsub is required for cross-instance broadcast.
		sync, ok := conn["sync"].(map[string]any)
		require.True(t, ok, "connections must declare sync.pubsub")
		assert.NotEmpty(t, sync["pubsub"])
	})
}

func TestValidateConfigHandler(t *testing.T) {
	t.Run("valid project", func(t *testing.T) {
		// Scaffold a project first so we have known-valid config
		tmpDir := t.TempDir()
		projectPath := filepath.Join(tmpDir, "valid-project")
		scaffoldReq := makeCallToolRequest("noda_scaffold_project", map[string]any{"path": projectPath})
		_, err := scaffoldProjectHandler(context.Background(), scaffoldReq)
		require.NoError(t, err)

		req := makeCallToolRequest("noda_validate_config", map[string]any{"config_dir": projectPath})
		result, err := validateConfigHandler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data := parseTextResult(t, result)
		assert.True(t, data["valid"].(bool))
	})

	t.Run("nonexistent project", func(t *testing.T) {
		req := makeCallToolRequest("noda_validate_config", map[string]any{"config_dir": "/nonexistent/path"})
		result, err := validateConfigHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		assert.False(t, data["valid"].(bool))
	})

	t.Run("relative path rejected", func(t *testing.T) {
		req := makeCallToolRequest("noda_validate_config", map[string]any{"config_dir": "relative/path"})
		result, err := validateConfigHandler(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("errors carry file path and JSON pointer", func(t *testing.T) {
		// Scaffold a valid project, then introduce a localizable schema violation:
		// body.schema must be an object, set it to a string (the #224 repro).
		tmpDir := t.TempDir()
		projectPath := filepath.Join(tmpDir, "bad-project")
		scaffoldReq := makeCallToolRequest("noda_scaffold_project", map[string]any{"path": projectPath})
		_, err := scaffoldProjectHandler(context.Background(), scaffoldReq)
		require.NoError(t, err)

		badRoute := `{
  "id": "hello-route",
  "method": "GET",
  "path": "/api/hello/:name",
  "body": { "schema": "not-an-object" },
  "trigger": { "workflow": "hello", "input": {} }
}`
		require.NoError(t, os.WriteFile(filepath.Join(projectPath, "routes", "api.json"), []byte(badRoute), 0o644))

		req := makeCallToolRequest("noda_validate_config", map[string]any{"config_dir": projectPath})
		result, err := validateConfigHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		assert.False(t, data["valid"].(bool))

		errsRaw, ok := data["errors"].([]any)
		require.True(t, ok, "errors should be a list of objects")
		require.NotEmpty(t, errsRaw)

		first := errsRaw[0].(map[string]any)
		assert.Contains(t, first["file"], "api.json", "error must name the offending file")
		assert.Equal(t, "/body/schema", first["pointer"], "error must carry a JSON pointer to the field")
		assert.NotEmpty(t, first["message"])
		assert.Contains(t, first["error"], "api.json")
		assert.Contains(t, first["error"], "/body/schema")
	})

	t.Run("catches node-config errors that pass file-level schema validation", func(t *testing.T) {
		// A response.error node with an empty config is valid JSON and matches
		// the generic node schema (file-level validation), but the response
		// plugin's audited schema requires "code" and "message" — only the
		// startup dry-run (node/service/expression checks) catches this.
		tmpDir := t.TempDir()
		projectPath := filepath.Join(tmpDir, "dryrun-project")
		scaffoldReq := makeCallToolRequest("noda_scaffold_project", map[string]any{"path": projectPath})
		_, err := scaffoldProjectHandler(context.Background(), scaffoldReq)
		require.NoError(t, err)

		badWorkflow := `{
  "id": "broken",
  "nodes": {
    "fail": { "type": "response.error", "config": {} }
  },
  "edges": []
}`
		require.NoError(t, os.WriteFile(filepath.Join(projectPath, "workflows", "broken.json"), []byte(badWorkflow), 0o644))

		req := makeCallToolRequest("noda_validate_config", map[string]any{"config_dir": projectPath})
		result, err := validateConfigHandler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data := parseTextResult(t, result)
		assert.False(t, data["valid"].(bool))

		errsRaw, ok := data["errors"].([]any)
		require.True(t, ok, "errors should be a list of objects")
		require.NotEmpty(t, errsRaw)

		found := false
		for _, raw := range errsRaw {
			m := raw.(map[string]any)
			if msg, _ := m["message"].(string); strings.Contains(msg, "missing required config field") {
				found = true
			}
		}
		assert.True(t, found, "expected a 'missing required config field' error, got: %+v", errsRaw)
	})

	t.Run("catches service-only plugin schema errors (storage/stream/pubsub)", func(t *testing.T) {
		// #376's ServiceConfigSchema audit runs during the startup dry-run.
		// storage/stream/pubsub provide services but no node types, so they
		// were previously registered in the CLI's dry-run plugin set but not
		// the MCP tool's (corePlugins() only) — meaning a bad storage config
		// validated as clean via MCP while `noda validate` correctly rejected
		// it. Regression test for that parity gap.
		tmpDir := t.TempDir()
		projectPath := filepath.Join(tmpDir, "service-only-project")
		scaffoldReq := makeCallToolRequest("noda_scaffold_project", map[string]any{"path": projectPath})
		_, err := scaffoldProjectHandler(context.Background(), scaffoldReq)
		require.NoError(t, err)

		nodaJSONPath := filepath.Join(projectPath, "noda.json")
		raw, err := os.ReadFile(nodaJSONPath)
		require.NoError(t, err)
		var cfg map[string]any
		require.NoError(t, json.Unmarshal(raw, &cfg))
		cfg["services"] = map[string]any{
			"badstorage": map[string]any{
				"plugin": "storage",
				"config": map[string]any{"backend": "s3"}, // not in enum [local memory]
			},
		}
		updated, err := json.Marshal(cfg)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(nodaJSONPath, updated, 0o644))

		req := makeCallToolRequest("noda_validate_config", map[string]any{"config_dir": projectPath})
		result, err := validateConfigHandler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		assert.False(t, data["valid"].(bool))

		errsRaw, ok := data["errors"].([]any)
		require.True(t, ok, "errors should be a list of objects")
		require.NotEmpty(t, errsRaw)

		found := false
		for _, raw := range errsRaw {
			m := raw.(map[string]any)
			if msg, _ := m["message"].(string); strings.Contains(msg, "badstorage") && strings.Contains(msg, "storage") {
				found = true
			}
		}
		assert.True(t, found, "expected a service-schema error naming the storage service, got: %+v", errsRaw)
	})
}

func TestExplainWorkflowHandler(t *testing.T) {
	nodeReg := buildNodeRegistry()
	handler := explainWorkflowHandler(nodeReg)

	t.Run("basic workflow", func(t *testing.T) {
		workflow := `{
			"id": "test",
			"nodes": {
				"greet": {
					"type": "transform.set",
					"config": {
						"fields": {
							"message": "Hello, {{ input.name }}!"
						}
					}
				},
				"respond": {
					"type": "response.json",
					"config": {
						"status": 200,
						"body": {
							"greeting": "{{ nodes.greet.message }}"
						}
					}
				}
			},
			"edges": [
				{"from": "greet", "to": "respond", "output": "success"}
			]
		}`

		req := makeCallToolRequest("noda_explain_workflow", map[string]any{
			"workflow": workflow,
		})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data := parseTextResult(t, result)
		assert.Equal(t, "test", data["workflow_id"])

		nodes := data["nodes"].([]any)
		assert.Len(t, nodes, 2)

		order := data["execution_order"].([]any)
		assert.Equal(t, "greet", order[0])
		assert.Equal(t, "respond", order[1])

		entryNodes := data["entry_nodes"].([]any)
		assert.Contains(t, entryNodes, "greet")

		terminalNodes := data["terminal_nodes"].([]any)
		assert.Contains(t, terminalNodes, "respond")

		// Verify expressions were collected
		greetNode := nodes[0].(map[string]any)
		assert.Equal(t, "greet", greetNode["id"])
		expressions := greetNode["expressions"].([]any)
		assert.Contains(t, expressions, "{{ input.name }}")

		// Verify edges
		respondNode := nodes[1].(map[string]any)
		assert.Equal(t, "respond", respondNode["id"])
		incoming := respondNode["incoming_edges"].([]any)
		assert.Contains(t, incoming, "greet.success")

		outgoing := greetNode["outgoing_edges"].([]any)
		assert.Contains(t, outgoing, "success -> respond")
	})

	t.Run("workflow with alias", func(t *testing.T) {
		workflow := `{
			"id": "alias-test",
			"nodes": {
				"fetch_user": {
					"type": "db.findOne",
					"as": "user",
					"config": {
						"table": "users",
						"where": {"id": "{{ input.id }}"}
					}
				}
			},
			"edges": []
		}`
		req := makeCallToolRequest("noda_explain_workflow", map[string]any{
			"workflow": workflow,
		})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)

		data := parseTextResult(t, result)
		nodes := data["nodes"].([]any)
		node := nodes[0].(map[string]any)
		assert.Equal(t, "user", node["alias"])
		assert.Equal(t, "nodes.user", node["output_path"])
	})

	t.Run("with mock input", func(t *testing.T) {
		workflow := `{
			"id": "mock-test",
			"nodes": {
				"greet": {
					"type": "transform.set",
					"config": {
						"fields": {
							"message": "Hello, {{ input.name }}!"
						}
					}
				}
			},
			"edges": []
		}`
		req := makeCallToolRequest("noda_explain_workflow", map[string]any{
			"workflow": workflow,
			"input":    `{"name": "World"}`,
		})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)

		data := parseTextResult(t, result)
		nodes := data["nodes"].([]any)
		node := nodes[0].(map[string]any)
		config := node["config"].(map[string]any)
		fields := config["fields"].(map[string]any)
		assert.Equal(t, "Hello, World!", fields["message"])
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := makeCallToolRequest("noda_explain_workflow", map[string]any{
			"workflow": "not json",
		})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("invalid input JSON", func(t *testing.T) {
		workflow := `{"id": "test", "nodes": {}, "edges": []}`
		req := makeCallToolRequest("noda_explain_workflow", map[string]any{
			"workflow": workflow,
			"input":    "not json",
		})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestScaffoldProjectHandler(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "test-project")

	req := makeCallToolRequest("noda_scaffold_project", map[string]any{"path": projectPath})
	result, err := scaffoldProjectHandler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	expectedFiles := []string{
		"noda.json",
		"routes/api.json",
		"workflows/hello.json",
		"schemas/greeting.json",
		"tests/hello.test.json",
		"migrations/20260101000000_create_items.up.sql",
		"migrations/20260101000000_create_items.down.sql",
		".env",
		".env.example",
		"docker-compose.yml",
	}
	for _, f := range expectedFiles {
		_, err := os.Stat(filepath.Join(projectPath, f))
		assert.NoError(t, err, "expected file %s to exist", f)
	}

	// Verify noda.json is valid JSON
	data, err := os.ReadFile(filepath.Join(projectPath, "noda.json"))
	require.NoError(t, err)
	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(data, &parsed))

	// Scaffolded services must match the documented/root-schema shape the runtime
	// actually parses: family/registered plugin name + nested "config", and the
	// canonical {{ $env('VAR') }} interpolation (not ${VAR}, which is unsupported). (#231)
	services, ok := parsed["services"].(map[string]any)
	require.True(t, ok, "noda.json must define services")
	for name, raw := range services {
		svc, ok := raw.(map[string]any)
		require.True(t, ok, "service %q must be an object", name)
		assert.NotEmpty(t, svc["plugin"], "service %q must name a plugin", name)
		cfg, ok := svc["config"].(map[string]any)
		assert.True(t, ok, "service %q connection fields must be nested under config", name)
		for k, v := range cfg {
			if s, isStr := v.(string); isStr {
				assert.NotContains(t, s, "${", "service %q.config.%s uses unsupported ${VAR} env syntax", name, k)
			}
		}
		_, hasDSN := svc["dsn"]
		assert.False(t, hasDSN, "service %q must not put dsn at top level", name)
	}

	// The scaffolded project must pass validation end-to-end.
	vreq := makeCallToolRequest("noda_validate_config", map[string]any{"config_dir": projectPath})
	vres, err := validateConfigHandler(context.Background(), vreq)
	require.NoError(t, err)
	vdata := parseTextResult(t, vres)
	assert.True(t, vdata["valid"].(bool), "scaffolded project should validate")
}

func TestScaffoldProjectHandler_GeneratesEnvWithUniqueJWTSecret(t *testing.T) {
	tmpDir := t.TempDir()
	projectA := filepath.Join(tmpDir, "project-a")
	projectB := filepath.Join(tmpDir, "project-b")

	reqA := makeCallToolRequest("noda_scaffold_project", map[string]any{"path": projectA})
	_, err := scaffoldProjectHandler(context.Background(), reqA)
	require.NoError(t, err)
	reqB := makeCallToolRequest("noda_scaffold_project", map[string]any{"path": projectB})
	_, err = scaffoldProjectHandler(context.Background(), reqB)
	require.NoError(t, err)

	exampleData, err := os.ReadFile(filepath.Join(projectA, ".env.example"))
	require.NoError(t, err)
	assert.Contains(t, string(exampleData), "at least 32 bytes")
	assert.Contains(t, string(exampleData), "replace-with-at-least-32-bytes")

	envA, err := os.ReadFile(filepath.Join(projectA, ".env"))
	require.NoError(t, err)
	envB, err := os.ReadFile(filepath.Join(projectB, ".env"))
	require.NoError(t, err)

	secretA := extractJWTSecret(t, string(envA))
	secretB := extractJWTSecret(t, string(envB))
	assert.Len(t, secretA, 64)
	assert.Len(t, secretB, 64)
	assert.NotEqual(t, secretA, secretB, "each scaffold must generate a unique secret")
	assert.NotContains(t, string(envA), "replace-with-at-least-32-bytes")
	assert.Contains(t, string(envA), "DATABASE_URL=postgres://noda:noda@localhost:5432/noda?sslmode=disable")
}

func extractJWTSecret(t *testing.T, envContent string) string {
	t.Helper()
	for line := range strings.SplitSeq(envContent, "\n") {
		if after, ok := strings.CutPrefix(line, "JWT_SECRET="); ok {
			return after
		}
	}
	t.Fatal("JWT_SECRET= line not found in .env")
	return ""
}

func TestScaffoldProjectHandler_RefusesOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "test-project")

	require.NoError(t, os.MkdirAll(projectPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "noda.json"), []byte("{}"), 0644))

	req := makeCallToolRequest("noda_scaffold_project", map[string]any{"path": projectPath})
	result, err := scaffoldProjectHandler(context.Background(), req)
	require.NoError(t, err)
	require.True(t, result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, text, "noda.json")

	// existing file untouched, no partial scaffold
	data, err := os.ReadFile(filepath.Join(projectPath, "noda.json"))
	require.NoError(t, err)
	assert.Equal(t, "{}", string(data))
	_, err = os.Stat(filepath.Join(projectPath, "docker-compose.yml"))
	assert.True(t, os.IsNotExist(err), "docker-compose.yml should not have been written")
}

func TestReadProjectFileHandler(t *testing.T) {
	configDir, _ := filepath.Abs("../../examples/rest-api")

	t.Run("read JSON file", func(t *testing.T) {
		req := makeCallToolRequest("noda_read_project_file", map[string]any{
			"config_dir": configDir,
			"path":       "noda.json",
		})
		result, err := readProjectFileHandler(context.Background(), req)
		require.NoError(t, err)
		require.False(t, result.IsError)
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		req := makeCallToolRequest("noda_read_project_file", map[string]any{
			"config_dir": configDir,
			"path":       "../../../etc/passwd",
		})
		result, err := readProjectFileHandler(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		req := makeCallToolRequest("noda_read_project_file", map[string]any{
			"config_dir": configDir,
			"path":       "/etc/passwd",
		})
		result, err := readProjectFileHandler(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		req := makeCallToolRequest("noda_read_project_file", map[string]any{
			"config_dir": configDir,
			"path":       "nonexistent.json",
		})
		result, err := readProjectFileHandler(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestListProjectFilesHandler(t *testing.T) {
	configDir, _ := filepath.Abs("../../examples/rest-api")

	req := makeCallToolRequest("noda_list_project_files", map[string]any{"config_dir": configDir})
	result, err := listProjectFilesHandler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	data := parseTextResult(t, result)
	assert.Equal(t, "noda.json", data["root"])
}

// parseTextResult extracts and parses JSON from a CallToolResult.
func parseTextResult(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	require.NotEmpty(t, result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &data))
	return data
}
