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

	// Should have Noda built-in (12) + expr-lang built-in (19) = 31
	assert.Equal(t, 31, count)

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
