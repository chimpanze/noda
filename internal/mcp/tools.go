package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	configschemas "github.com/chimpanze/noda/internal/config/schemas"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTools(s *server.MCPServer, nodeReg *registry.NodeRegistry) {
	// Metadata tools (no project needed)
	s.AddTool(
		mcp.NewTool("noda_list_nodes",
			mcp.WithDescription("List all Noda node types with descriptions, outputs, and service dependencies. Use to discover what building blocks are available."),
			mcp.WithString("category",
				mcp.Description("Filter by category prefix (e.g. 'db', 'control', 'transform', 'cache', 'response', 'util')"),
			),
		),
		listNodesHandler(nodeReg),
	)

	s.AddTool(
		mcp.NewTool("noda_get_node_schema",
			mcp.WithDescription("Get the JSON Schema for a specific node type's config. Use after noda_list_nodes to understand how to configure a node."),
			mcp.WithString("node_type",
				mcp.Required(),
				mcp.Description("Full node type (e.g. 'db.query', 'control.if', 'transform.set')"),
			),
		),
		getNodeSchemaHandler(nodeReg),
	)

	s.AddTool(
		mcp.NewTool("noda_get_config_schema",
			mcp.WithDescription("Get the JSON Schema for a Noda config file type. Use to understand the structure of config files."),
			mcp.WithString("config_type",
				mcp.Required(),
				mcp.Description("Config file type: root, route, workflow, worker, schedule, connections, or test"),
			),
		),
		getConfigSchemaHandler,
	)

	s.AddTool(
		mcp.NewTool("noda_validate_expression",
			mcp.WithDescription("Validate a Noda expression for syntax errors. Expressions use {{ }} delimiters."),
			mcp.WithString("expression",
				mcp.Required(),
				mcp.Description("Expression to validate (e.g. '{{ input.name }}', '{{ len(nodes.fetch) > 0 }}')"),
			),
		),
		validateExpressionHandler,
	)

	s.AddTool(
		mcp.NewTool("noda_get_examples",
			mcp.WithDescription("Get example config snippets for common patterns. Use to bootstrap new routes and workflows."),
			mcp.WithString("pattern",
				mcp.Description("Pattern name: crud, auth, websocket, file-upload, scheduled-job, or all"),
				mcp.DefaultString("all"),
			),
		),
		getExamplesHandler,
	)

	// Project tools (accept config_dir parameter)
	s.AddTool(
		mcp.NewTool("noda_validate_config",
			mcp.WithDescription("Validate a Noda project's config files. Reports schema errors, missing references, and cross-file issues."),
			mcp.WithString("config_dir",
				mcp.Required(),
				mcp.Description("Absolute path to the Noda project directory containing noda.json"),
			),
		),
		validateConfigHandler,
	)

	s.AddTool(
		mcp.NewTool("noda_scaffold_project",
			mcp.WithDescription("Create a new Noda project with standard directory structure and sample files."),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Absolute path for the new project directory"),
			),
		),
		scaffoldProjectHandler,
	)

	s.AddTool(
		mcp.NewTool("noda_read_project_file",
			mcp.WithDescription("Read a config file from a Noda project. Returns the file contents as JSON."),
			mcp.WithString("config_dir",
				mcp.Required(),
				mcp.Description("Absolute path to the Noda project directory"),
			),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Relative path within the project (e.g. 'routes/api.json', 'workflows/hello.json')"),
			),
		),
		readProjectFileHandler,
	)

	s.AddTool(
		mcp.NewTool("noda_list_project_files",
			mcp.WithDescription("List all config files in a Noda project, categorized by type (routes, workflows, etc.)."),
			mcp.WithString("config_dir",
				mcp.Required(),
				mcp.Description("Absolute path to the Noda project directory"),
			),
		),
		listProjectFilesHandler,
	)
}

// --- Metadata tool handlers ---

func listNodesHandler(nodeReg *registry.NodeRegistry) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		category := req.GetString("category", "")

		types := nodeReg.AllTypes()
		sort.Strings(types)

		nodes := make([]map[string]any, 0, len(types))
		for _, t := range types {
			if category != "" && !strings.HasPrefix(t, category+".") {
				continue
			}

			desc, ok := nodeReg.GetDescriptor(t)
			if !ok {
				continue
			}

			outputs, _ := nodeReg.OutputsForType(t)

			entry := map[string]any{
				"type":        t,
				"description": desc.Description(),
				"outputs":     outputs,
			}

			if deps := desc.ServiceDeps(); len(deps) > 0 {
				depsMap := make(map[string]any, len(deps))
				for slot, dep := range deps {
					depsMap[slot] = map[string]any{
						"prefix":   dep.Prefix,
						"required": dep.Required,
					}
				}
				entry["service_deps"] = depsMap
			}

			if schema := desc.ConfigSchema(); schema != nil {
				entry["has_schema"] = true
			}

			nodes = append(nodes, entry)
		}

		return jsonResult(map[string]any{"nodes": nodes, "count": len(nodes)})
	}
}

func getNodeSchemaHandler(nodeReg *registry.NodeRegistry) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeType, err := req.RequireString("node_type")
		if err != nil {
			return mcp.NewToolResultError("node_type is required"), nil
		}

		desc, ok := nodeReg.GetDescriptor(nodeType)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("node type %q not found", nodeType)), nil
		}

		schema := desc.ConfigSchema()
		result := map[string]any{
			"node_type":   nodeType,
			"description": desc.Description(),
		}

		if schema != nil {
			result["config_schema"] = schema
		}

		outputs, _ := nodeReg.OutputsForType(nodeType)
		result["outputs"] = outputs

		if deps := desc.ServiceDeps(); len(deps) > 0 {
			result["service_deps"] = deps
		}

		return jsonResult(result)
	}
}

func getConfigSchemaHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configType, err := req.RequireString("config_type")
	if err != nil {
		return mcp.NewToolResultError("config_type is required"), nil
	}

	validTypes := map[string]string{
		"root":        "root.json",
		"route":       "route.json",
		"workflow":    "workflow.json",
		"worker":      "worker.json",
		"schedule":    "schedule.json",
		"connections": "connections.json",
		"test":        "test.json",
	}

	filename, ok := validTypes[configType]
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("unknown config type %q, valid types: %s",
			configType, strings.Join(sortedKeys(validTypes), ", "))), nil
	}

	data, err := configschemas.FS.ReadFile(filename)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read schema: %v", err)), nil
	}

	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse schema: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"config_type": configType,
		"schema":      schema,
	})
}

func validateExpressionHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	expression, err := req.RequireString("expression")
	if err != nil {
		return mcp.NewToolResultError("expression is required"), nil
	}

	compiler := expr.NewCompiler()
	_, compileErr := compiler.Compile(expression)

	result := map[string]any{
		"expression": expression,
		"valid":      compileErr == nil,
	}
	if compileErr != nil {
		result["error"] = compileErr.Error()
	}

	return jsonResult(result)
}

// --- Project tool handlers ---

func validateConfigHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configDir, err := req.RequireString("config_dir")
	if err != nil {
		return mcp.NewToolResultError("config_dir is required"), nil
	}

	if !isAbsPath(configDir) {
		return mcp.NewToolResultError("config_dir must be an absolute path"), nil
	}

	rc, errs := config.ValidateAll(configDir, "")
	if len(errs) > 0 {
		errMsgs := make([]string, len(errs))
		for i, e := range errs {
			errMsgs[i] = e.Message
		}
		return jsonResult(map[string]any{
			"valid":  false,
			"errors": errMsgs,
		})
	}

	return jsonResult(map[string]any{
		"valid":      true,
		"file_count": rc.FileCount,
		"summary": map[string]int{
			"routes":      len(rc.Routes),
			"workflows":   len(rc.Workflows),
			"workers":     len(rc.Workers),
			"schedules":   len(rc.Schedules),
			"connections": len(rc.Connections),
			"tests":       len(rc.Tests),
		},
	})
}

func scaffoldProjectHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}

	if !isAbsPath(path) {
		return mcp.NewToolResultError("path must be an absolute path"), nil
	}

	// Create project directory structure
	dirs := []string{
		"routes",
		"workflows",
		"schemas",
		"tests",
		"migrations",
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create project directory: %v", err)), nil
	}

	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(path, d), 0755); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to create %s directory: %v", d, err)), nil
		}
	}

	files := map[string]string{
		"noda.json":             scaffoldNodaJSON,
		".env.example":          scaffoldEnvExample,
		"docker-compose.yml":    scaffoldDockerCompose,
		"routes/api.json":       scaffoldSampleRoute,
		"workflows/hello.json":  scaffoldSampleWorkflow,
		"schemas/greeting.json": scaffoldSampleSchema,
		"tests/hello.test.json": scaffoldSampleTest,
	}

	for name, content := range files {
		fullPath := filepath.Join(path, name)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to write %s: %v", name, err)), nil
		}
	}

	created := make([]string, 0, len(files)+len(dirs))
	for _, d := range dirs {
		created = append(created, d+"/")
	}
	for name := range files {
		created = append(created, name)
	}
	sort.Strings(created)

	return jsonResult(map[string]any{
		"created": created,
		"path":    path,
		"message": "Project scaffolded. Next: cd into directory, cp .env.example .env, docker compose up -d, noda dev",
	})
}

func readProjectFileHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configDir, err := req.RequireString("config_dir")
	if err != nil {
		return mcp.NewToolResultError("config_dir is required"), nil
	}
	relPath, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}

	if !isAbsPath(configDir) {
		return mcp.NewToolResultError("config_dir must be an absolute path"), nil
	}

	// Prevent path traversal
	cleaned := filepath.Clean(relPath)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return mcp.NewToolResultError("path must be a relative path within the project"), nil
	}

	fullPath := filepath.Join(configDir, cleaned)

	// Verify the resolved path is still within configDir
	absConfigDir, _ := filepath.Abs(configDir)
	absFullPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absFullPath, absConfigDir+string(filepath.Separator)) {
		return mcp.NewToolResultError("path must be within the project directory"), nil
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.NewToolResultError(fmt.Sprintf("file not found: %s", relPath)), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("failed to read file: %v", err)), nil
	}

	// Try to parse as JSON for pretty output
	var parsed any
	if json.Unmarshal(data, &parsed) == nil {
		return jsonResult(map[string]any{
			"path":    relPath,
			"content": parsed,
		})
	}

	// Return raw content for non-JSON files
	return mcp.NewToolResultText(string(data)), nil
}

func listProjectFilesHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configDir, err := req.RequireString("config_dir")
	if err != nil {
		return mcp.NewToolResultError("config_dir is required"), nil
	}

	if !isAbsPath(configDir) {
		return mcp.NewToolResultError("config_dir must be an absolute path"), nil
	}

	discovered, err := config.Discover(configDir, "")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to discover config files: %v", err)), nil
	}

	result := map[string]any{
		"root": relativeToDir(configDir, discovered.Root),
	}

	if discovered.Overlay != "" {
		result["overlay"] = relativeToDir(configDir, discovered.Overlay)
	}
	if discovered.Vars != "" {
		result["vars"] = relativeToDir(configDir, discovered.Vars)
	}

	addRelPaths := func(key string, paths []string) {
		if len(paths) > 0 {
			rel := make([]string, len(paths))
			for i, p := range paths {
				rel[i] = relativeToDir(configDir, p)
			}
			result[key] = rel
		}
	}

	addRelPaths("schemas", discovered.Schemas)
	addRelPaths("routes", discovered.Routes)
	addRelPaths("workflows", discovered.Workflows)
	addRelPaths("workers", discovered.Workers)
	addRelPaths("schedules", discovered.Schedules)
	addRelPaths("connections", discovered.Connections)
	addRelPaths("tests", discovered.Tests)
	addRelPaths("models", discovered.Models)

	return jsonResult(result)
}

// --- Helpers ---

func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func isAbsPath(p string) bool {
	return filepath.IsAbs(p)
}

func relativeToDir(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- Scaffold templates ---

const scaffoldNodaJSON = `{
  "server": {
    "port": 3000,
    "read_timeout": "30s",
    "write_timeout": "30s",
    "body_limit": 5242880
  },
  "services": {
    "db": {
      "plugin": "postgres",
      "dsn": "${DATABASE_URL}"
    },
    "cache": {
      "plugin": "redis",
      "url": "${REDIS_URL}"
    }
  }
}
`

const scaffoldEnvExample = `# Database
DATABASE_URL=postgres://noda:noda@localhost:5432/noda?sslmode=disable

# Redis
REDIS_URL=redis://localhost:6379/0

# JWT
JWT_SECRET=change-me-in-production
`

const scaffoldDockerCompose = `services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: noda
      POSTGRES_PASSWORD: noda
      POSTGRES_DB: noda
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

volumes:
  pgdata:
`

const scaffoldSampleRoute = `{
  "id": "hello-route",
  "method": "GET",
  "path": "/api/hello/:name",
  "trigger": {
    "workflow": "hello",
    "input": {
      "name": "{{ request.params.name }}"
    }
  }
}
`

const scaffoldSampleWorkflow = `{
  "id": "hello",
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
    { "from": "greet", "to": "respond", "output": "success" }
  ]
}
`

const scaffoldSampleSchema = `{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "minLength": 1,
      "maxLength": 100
    }
  },
  "required": ["name"]
}
`

const scaffoldSampleTest = `{
  "id": "hello-test",
  "workflow": "hello",
  "tests": [
    {
      "name": "greets by name",
      "input": { "name": "World" },
      "expect": {
        "status": "success",
        "output": {
          "greeting": "Hello, World!"
        }
      }
    }
  ]
}
`
