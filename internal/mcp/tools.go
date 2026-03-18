package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	configschemas "github.com/chimpanze/noda/internal/config/schemas"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/pathutil"
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
		mcp.NewTool("noda_list_functions",
			mcp.WithDescription("List all functions available in Noda expressions. Includes both built-in Noda functions and expr-lang built-in functions."),
		),
		listFunctionsHandler,
	)

	s.AddTool(
		mcp.NewTool("noda_validate_expression",
			mcp.WithDescription("Validate and analyze a Noda expression. Returns syntax validity, referenced variables, function calls, and warnings about unknown functions. Expressions use {{ }} delimiters."),
			mcp.WithString("expression",
				mcp.Required(),
				mcp.Description("Expression to validate (e.g. '{{ input.name }}', '{{ len(nodes.fetch) > 0 }}')"),
			),
		),
		validateExpressionHandler,
	)

	s.AddTool(
		mcp.NewTool("noda_explain_workflow",
			mcp.WithDescription("Statically analyze a workflow to understand execution order, data flow between nodes, and expected output shapes. Does not execute the workflow."),
			mcp.WithString("workflow",
				mcp.Required(),
				mcp.Description("Workflow JSON config as a string"),
			),
			mcp.WithString("input",
				mcp.Description("Optional mock input data as JSON string, used to resolve expressions"),
			),
		),
		explainWorkflowHandler(nodeReg),
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

			if outDesc := desc.OutputDescriptions(); len(outDesc) > 0 {
				entry["output_data"] = outDesc
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

		if outDesc := desc.OutputDescriptions(); len(outDesc) > 0 {
			result["output_data"] = outDesc
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

// exprLangBuiltins is the set of functions built into expr-lang.
var exprLangBuiltins = map[string]bool{
	"len": true, "contains": true, "startsWith": true, "endsWith": true,
	"trim": true, "replace": true, "split": true, "join": true,
	"keys": true, "values": true, "has": true, "filter": true,
	"map": true, "count": true, "any": true, "all": true,
	"none": true, "one": true, "first": true, "last": true,
	"abs": true, "ceil": true, "floor": true, "round": true,
	"max": true, "min": true, "sum": true, "mean": true,
	"string": true, "int": true, "float": true, "bool": true,
	"type": true, "toJSON": true, "fromJSON": true, "now": true,
	"date": true, "duration": true,
}

var variableRe = regexp.MustCompile(`\b(input|nodes|auth|trigger|env|request)\.[a-zA-Z0-9_.]+\b`)
var functionCallRe = regexp.MustCompile(`(\$?[a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)

func validateExpressionHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	expression, err := req.RequireString("expression")
	if err != nil {
		return mcp.NewToolResultError("expression is required"), nil
	}

	compiler := expr.NewCompilerWithFunctions()
	_, compileErr := compiler.Compile(expression)

	result := map[string]any{
		"expression": expression,
		"valid":      compileErr == nil,
	}
	if compileErr != nil {
		result["error"] = compileErr.Error()
	}

	// Extract variables, functions, and warnings from expression segments
	variables, functions, warnings := analyzeExpression(expression)
	result["variables"] = variables
	result["functions"] = functions
	result["warnings"] = warnings

	return jsonResult(result)
}

// analyzeExpression parses an expression and extracts referenced variables,
// function calls, and warnings about unknown functions.
func analyzeExpression(expression string) (variables []string, functions []string, warnings []string) {
	variables = []string{}
	functions = []string{}
	warnings = []string{}

	parsed, err := expr.Parse(expression)
	if err != nil || parsed.IsLiteral {
		return
	}

	// Build registered function name set
	reg := expr.NewFunctionRegistry()
	registeredNames := reg.RegisteredNames()
	registeredSet := make(map[string]bool, len(registeredNames))
	for _, name := range registeredNames {
		registeredSet[name] = true
	}

	varsSeen := make(map[string]bool)
	funcsSeen := make(map[string]bool)

	for _, seg := range parsed.Segments {
		if seg.Type != expr.SegmentExpression {
			continue
		}

		// Extract variables
		for _, match := range variableRe.FindAllString(seg.Value, -1) {
			if !varsSeen[match] {
				varsSeen[match] = true
				variables = append(variables, match)
			}
		}

		// Extract function calls
		for _, submatch := range functionCallRe.FindAllStringSubmatch(seg.Value, -1) {
			fname := submatch[1]
			if !funcsSeen[fname] {
				funcsSeen[fname] = true
				functions = append(functions, fname)

				// Check if function is known
				if !registeredSet[fname] && !exprLangBuiltins[fname] {
					// Check if there's a $-prefixed version
					if registeredSet["$"+fname] {
						warnings = append(warnings, fmt.Sprintf("%s is not a registered function, did you mean $%s?", fname, fname))
					} else {
						warnings = append(warnings, fmt.Sprintf("%s is not a registered function", fname))
					}
				}
			}
		}
	}

	return
}

func listFunctionsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	reg := expr.NewFunctionRegistry()
	functions := reg.RegisteredFunctions()

	// Add expr-lang built-in functions
	builtins := []expr.FunctionInfo{
		{Name: "all", Signature: "(array, predicate) bool", Description: "Returns true if all elements satisfy the predicate"},
		{Name: "any", Signature: "(array, predicate) bool", Description: "Returns true if any element satisfies the predicate"},
		{Name: "contains", Signature: "(string, substr string) bool", Description: "Returns true if string contains substr"},
		{Name: "count", Signature: "(array, predicate) int", Description: "Returns count of elements satisfying the predicate"},
		{Name: "endsWith", Signature: "(string, suffix string) bool", Description: "Returns true if string ends with suffix"},
		{Name: "filter", Signature: "(array, predicate) array", Description: "Returns elements that satisfy the predicate"},
		{Name: "first", Signature: "(array, predicate) any", Description: "Returns the first element satisfying the predicate"},
		{Name: "has", Signature: "(map, key string) bool", Description: "Returns true if the map contains the key"},
		{Name: "join", Signature: "(array, separator string) string", Description: "Joins array elements into a string with separator"},
		{Name: "keys", Signature: "(map) array", Description: "Returns the keys of a map"},
		{Name: "len", Signature: "(array | string | map) int", Description: "Returns the length of an array, string, or map"},
		{Name: "map", Signature: "(array, predicate) array", Description: "Transforms each element using the predicate"},
		{Name: "none", Signature: "(array, predicate) bool", Description: "Returns true if no elements satisfy the predicate"},
		{Name: "one", Signature: "(array, predicate) bool", Description: "Returns true if exactly one element satisfies the predicate"},
		{Name: "replace", Signature: "(string, old, new string) string", Description: "Replaces occurrences of old with new in string"},
		{Name: "split", Signature: "(string, separator string) array", Description: "Splits string by separator into an array"},
		{Name: "startsWith", Signature: "(string, prefix string) bool", Description: "Returns true if string starts with prefix"},
		{Name: "trim", Signature: "(string) string", Description: "Removes leading and trailing whitespace"},
		{Name: "values", Signature: "(map) array", Description: "Returns the values of a map"},
	}

	allFunctions := append(functions, builtins...)
	sort.Slice(allFunctions, func(i, j int) bool {
		return allFunctions[i].Name < allFunctions[j].Name
	})

	return jsonResult(map[string]any{
		"functions": allFunctions,
		"count":     len(allFunctions),
	})
}

// --- Explain workflow types and handler ---

type workflowDef struct {
	ID    string                     `json:"id"`
	Nodes map[string]workflowNodeDef `json:"nodes"`
	Edges []workflowEdgeDef          `json:"edges"`
}

type workflowNodeDef struct {
	Type   string         `json:"type"`
	As     string         `json:"as,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

type workflowEdgeDef struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Output string `json:"output"`
}

type nodeAnalysis struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	Alias         string            `json:"alias,omitempty"`
	OutputPath    string            `json:"output_path"`
	OutputData    map[string]string `json:"output_data,omitempty"`
	Config        map[string]any    `json:"config,omitempty"`
	Expressions   []string          `json:"expressions,omitempty"`
	IncomingEdges []string          `json:"incoming_edges,omitempty"`
	OutgoingEdges []string          `json:"outgoing_edges,omitempty"`
}

func explainWorkflowHandler(nodeReg *registry.NodeRegistry) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workflowJSON, err := req.RequireString("workflow")
		if err != nil {
			return mcp.NewToolResultError("workflow is required"), nil
		}

		var wf workflowDef
		if err := json.Unmarshal([]byte(workflowJSON), &wf); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid workflow JSON: %v", err)), nil
		}

		// Parse optional mock input
		var mockInput map[string]any
		inputJSON := req.GetString("input", "")
		if inputJSON != "" {
			if err := json.Unmarshal([]byte(inputJSON), &mockInput); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid input JSON: %v", err)), nil
			}
		}

		// Build adjacency list and find entry/terminal nodes
		incomingMap := make(map[string][]string) // nodeID -> ["fromID.output", ...]
		outgoingMap := make(map[string][]string) // nodeID -> ["output -> toID", ...]
		hasIncoming := make(map[string]bool)

		for _, edge := range wf.Edges {
			incomingMap[edge.To] = append(incomingMap[edge.To], edge.From+"."+edge.Output)
			outgoingMap[edge.From] = append(outgoingMap[edge.From], edge.Output+" -> "+edge.To)
			hasIncoming[edge.To] = true
		}

		// Find entry nodes (no incoming edges)
		var entryNodes []string
		for nodeID := range wf.Nodes {
			if !hasIncoming[nodeID] {
				entryNodes = append(entryNodes, nodeID)
			}
		}
		sort.Strings(entryNodes)

		// Topological sort via BFS (Kahn's algorithm)
		inDegree := make(map[string]int)
		for nodeID := range wf.Nodes {
			inDegree[nodeID] = 0
		}
		for _, edge := range wf.Edges {
			inDegree[edge.To]++
		}

		// Build adjacency for traversal
		adj := make(map[string][]string)
		for _, edge := range wf.Edges {
			adj[edge.From] = append(adj[edge.From], edge.To)
		}

		queue := make([]string, len(entryNodes))
		copy(queue, entryNodes)
		var executionOrder []string

		for len(queue) > 0 {
			sort.Strings(queue)
			node := queue[0]
			queue = queue[1:]
			executionOrder = append(executionOrder, node)

			for _, neighbor := range adj[node] {
				inDegree[neighbor]--
				if inDegree[neighbor] == 0 {
					queue = append(queue, neighbor)
				}
			}
		}

		// If some nodes weren't reached (cycles or disconnected), add them
		if len(executionOrder) < len(wf.Nodes) {
			for nodeID := range wf.Nodes {
				found := false
				for _, id := range executionOrder {
					if id == nodeID {
						found = true
						break
					}
				}
				if !found {
					executionOrder = append(executionOrder, nodeID)
				}
			}
		}

		// Find terminal nodes (no outgoing edges)
		hasOutgoing := make(map[string]bool)
		for _, edge := range wf.Edges {
			hasOutgoing[edge.From] = true
		}
		var terminalNodes []string
		for nodeID := range wf.Nodes {
			if !hasOutgoing[nodeID] {
				terminalNodes = append(terminalNodes, nodeID)
			}
		}
		sort.Strings(terminalNodes)

		// Set up expression compiler for mock input resolution
		var compiler *expr.Compiler
		if mockInput != nil {
			compiler = expr.NewCompilerWithFunctions()
		}

		// Build node analyses
		exprPattern := regexp.MustCompile(`\{\{.*?\}\}`)
		analyses := make([]nodeAnalysis, 0, len(executionOrder))

		for _, nodeID := range executionOrder {
			nodeDef := wf.Nodes[nodeID]

			analysis := nodeAnalysis{
				ID:   nodeID,
				Type: nodeDef.Type,
			}

			if nodeDef.As != "" {
				analysis.Alias = nodeDef.As
				analysis.OutputPath = "nodes." + nodeDef.As
			} else {
				analysis.OutputPath = "nodes." + nodeID
			}

			// Look up output descriptions from registry
			if desc, ok := nodeReg.GetDescriptor(nodeDef.Type); ok {
				if outDesc := desc.OutputDescriptions(); len(outDesc) > 0 {
					analysis.OutputData = outDesc
				}
			}

			// Collect expressions from config
			if nodeDef.Config != nil {
				analysis.Config = nodeDef.Config
				expressions := collectExpressions(nodeDef.Config, exprPattern)
				if len(expressions) > 0 {
					analysis.Expressions = expressions
				}

				// Mock input resolution
				if compiler != nil && mockInput != nil {
					analysis.Config = resolveConfigExpressions(nodeDef.Config, compiler, mockInput, exprPattern)
				}
			}

			if edges := incomingMap[nodeID]; len(edges) > 0 {
				analysis.IncomingEdges = edges
			}
			if edges := outgoingMap[nodeID]; len(edges) > 0 {
				analysis.OutgoingEdges = edges
			}

			analyses = append(analyses, analysis)
		}

		return jsonResult(map[string]any{
			"workflow_id":     wf.ID,
			"execution_order": executionOrder,
			"nodes":           analyses,
			"entry_nodes":     entryNodes,
			"terminal_nodes":  terminalNodes,
		})
	}
}

// collectExpressions recursively walks a config map/slice and collects all {{ }} expressions.
func collectExpressions(v any, pattern *regexp.Regexp) []string {
	var results []string
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			results = append(results, collectExpressions(val[k], pattern)...)
		}
	case []any:
		for _, item := range val {
			results = append(results, collectExpressions(item, pattern)...)
		}
	case string:
		matches := pattern.FindAllString(val, -1)
		results = append(results, matches...)
	}
	return results
}

// resolveConfigExpressions deep-copies a config and attempts to evaluate expressions
// using mock input data. If evaluation fails, the original value is kept.
func resolveConfigExpressions(v any, compiler *expr.Compiler, mockInput map[string]any, pattern *regexp.Regexp) map[string]any {
	ctx := map[string]any{"input": mockInput}
	resolved := resolveValue(v, compiler, ctx, pattern)
	if m, ok := resolved.(map[string]any); ok {
		return m
	}
	return nil
}

func resolveValue(v any, compiler *expr.Compiler, ctx map[string]any, pattern *regexp.Regexp) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, item := range val {
			result[k] = resolveValue(item, compiler, ctx, pattern)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = resolveValue(item, compiler, ctx, pattern)
		}
		return result
	case string:
		if pattern.MatchString(val) {
			compiled, err := compiler.Compile(val)
			if err != nil {
				return val
			}
			evaluated, err := expr.Evaluate(compiled, ctx)
			if err != nil {
				return val
			}
			return evaluated
		}
		return val
	default:
		return val
	}
}

// --- Project tool handlers ---

func validateConfigHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configDir, err := req.RequireString("config_dir")
	if err != nil {
		return mcp.NewToolResultError("config_dir is required"), nil
	}

	if !filepath.IsAbs(configDir) {
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

	if !filepath.IsAbs(path) {
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

	if !filepath.IsAbs(configDir) {
		return mcp.NewToolResultError("config_dir must be an absolute path"), nil
	}

	root, err := pathutil.NewRoot(configDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid config_dir: %v", err)), nil
	}

	fullPath, err := root.Resolve(relPath)
	if err != nil {
		return mcp.NewToolResultError("path must be a relative path within the project"), nil
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

	if !filepath.IsAbs(configDir) {
		return mcp.NewToolResultError("config_dir must be an absolute path"), nil
	}

	root, err := pathutil.NewRoot(configDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid config_dir: %v", err)), nil
	}

	discovered, err := config.Discover(root.String(), "")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to discover config files: %v", err)), nil
	}

	result := map[string]any{
		"root": root.Rel(discovered.Root),
	}

	if discovered.Overlay != "" {
		result["overlay"] = root.Rel(discovered.Overlay)
	}
	if discovered.Vars != "" {
		result["vars"] = root.Rel(discovered.Vars)
	}

	addRelPaths := func(key string, paths []string) {
		if len(paths) > 0 {
			rel := make([]string, len(paths))
			for i, p := range paths {
				rel[i] = root.Rel(p)
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
