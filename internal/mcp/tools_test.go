package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	})

	t.Run("unknown node type", func(t *testing.T) {
		req := makeCallToolRequest("noda_get_node_schema", map[string]any{"node_type": "nonexistent"})
		result, err := handler(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, result.IsError)
	})
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
