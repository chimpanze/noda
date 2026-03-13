package server

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/gofiber/fiber/v3"
)

// listNodes returns all registered node types with their descriptors.
func (e *EditorAPI) listNodes(c fiber.Ctx) error {
	types := e.nodes.AllTypes()
	sort.Strings(types)

	nodes := make([]map[string]any, 0, len(types))
	for _, t := range types {
		desc, ok := e.nodes.GetDescriptor(t)
		if !ok {
			continue
		}

		// Get outputs by creating a temporary executor
		outputs, _ := e.nodes.OutputsForType(t)

		entry := map[string]any{
			"type":        t,
			"name":        desc.Name(),
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

	return c.JSON(map[string]any{"nodes": nodes})
}

// getNodeSchema returns the JSON Schema for a node type's config.
func (e *EditorAPI) getNodeSchema(c fiber.Ctx) error {
	nodeType := c.Params("type")
	desc, ok := e.nodes.GetDescriptor(nodeType)
	if !ok {
		return c.Status(404).JSON(map[string]any{"error": fmt.Sprintf("node type %q not found", nodeType)})
	}

	schema := desc.ConfigSchema()
	if schema == nil {
		return c.JSON(map[string]any{})
	}
	return c.JSON(schema)
}

// computeOutputs creates a temporary executor and returns its outputs.
func (e *EditorAPI) computeOutputs(c fiber.Ctx) error {
	nodeType := c.Params("type")
	factory, ok := e.nodes.GetFactory(nodeType)
	if !ok {
		return c.Status(404).JSON(map[string]any{"error": fmt.Sprintf("node type %q not found", nodeType)})
	}

	var cfg map[string]any
	if err := c.Bind().JSON(&cfg); err != nil {
		cfg = nil
	}

	executor := factory(cfg)
	return c.JSON(map[string]any{"outputs": executor.Outputs()})
}

// listServices returns all configured service instances.
func (e *EditorAPI) listServices(c fiber.Ctx) error {
	all := e.services.All()
	services := make([]map[string]any, 0, len(all))

	for name, svc := range all {
		prefix, _ := e.services.GetPrefix(name)
		entry := map[string]any{
			"name":   name,
			"prefix": prefix,
		}

		// Check health
		if checker, ok := svc.(interface{ Ping() error }); ok {
			if err := checker.Ping(); err != nil {
				entry["health"] = "unhealthy"
				entry["error"] = err.Error()
			} else {
				entry["health"] = "healthy"
			}
		} else {
			entry["health"] = "unknown"
		}

		services = append(services, entry)
	}

	sort.Slice(services, func(i, j int) bool {
		return services[i]["name"].(string) < services[j]["name"].(string)
	})

	return c.JSON(map[string]any{"services": services})
}

// pluginDescriptions maps plugin prefixes to human-readable descriptions.
var pluginDescriptions = map[string]string{
	"db":      "PostgreSQL database — CRUD, raw queries, transactions",
	"cache":   "Redis key-value cache — get, set, delete, exists",
	"stream":  "Redis Streams — durable event streaming",
	"pubsub":  "Redis Pub/Sub — real-time message broadcasting",
	"storage": "File storage — read, write, list, delete",
	"image":   "Image processing — resize, crop, watermark, convert, thumbnail",
	"http":    "Outbound HTTP client — GET, POST, and custom requests",
	"email":   "Email sending via SMTP",
}

// listPlugins returns all loaded plugins with their prefixes and node counts.
func (e *EditorAPI) listPlugins(c fiber.Ctx) error {
	all := e.plugins.All()
	plugins := make([]map[string]any, 0, len(all))

	for _, p := range all {
		entry := map[string]any{
			"name":         p.Name(),
			"prefix":       p.Prefix(),
			"has_services": p.HasServices(),
			"node_count":   len(p.Nodes()),
		}
		if desc, ok := pluginDescriptions[p.Prefix()]; ok {
			entry["description"] = desc
		}
		plugins = append(plugins, entry)
	}

	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i]["prefix"].(string) < plugins[j]["prefix"].(string)
	})

	return c.JSON(map[string]any{"plugins": plugins})
}

// listSchemas returns all shared schema definitions.
func (e *EditorAPI) listSchemas(c fiber.Ctx) error {
	rc := e.resolvedConfig()

	schemas := make([]map[string]any, 0, len(rc.Schemas))
	for path, schema := range rc.Schemas {
		schemas = append(schemas, map[string]any{
			"path":   relPath(e.configDir, path),
			"schema": schema,
		})
	}

	sort.Slice(schemas, func(i, j int) bool {
		return schemas[i]["path"].(string) < schemas[j]["path"].(string)
	})

	return c.JSON(map[string]any{"schemas": schemas})
}

// listMiddleware returns middleware metadata, presets, and current config.
func (e *EditorAPI) listMiddleware(c fiber.Ctx) error {
	type configField struct {
		Key         string   `json:"key"`
		Type        string   `json:"type"`
		Description string   `json:"description,omitempty"`
		Required    bool     `json:"required,omitempty"`
		Default     any      `json:"default,omitempty"`
		Options     []string `json:"options,omitempty"`
		Placeholder string   `json:"placeholder,omitempty"`
	}
	type descriptor struct {
		Name         string        `json:"name"`
		Description  string        `json:"description,omitempty"`
		ConfigFields []configField `json:"config_fields"`
	}

	descriptors := []descriptor{
		{Name: "auth.jwt", Description: "JWT authentication using Bearer tokens", ConfigFields: []configField{
			{Key: "secret", Type: "string", Description: "JWT signing secret or key", Required: true},
			{Key: "algorithm", Type: "select", Description: "Signing algorithm", Options: []string{"HS256", "HS384", "HS512"}, Default: "HS256"},
		}},
		{Name: "security.cors", Description: "Cross-Origin Resource Sharing headers", ConfigFields: []configField{
			{Key: "allow_origins", Type: "string", Description: "Allowed origins (comma-separated or *)"},
			{Key: "allow_methods", Type: "string", Description: "Allowed HTTP methods"},
			{Key: "allow_headers", Type: "string", Description: "Allowed request headers"},
			{Key: "allow_credentials", Type: "boolean", Description: "Allow credentials (cookies, auth headers)"},
		}},
		{Name: "security.headers", Description: "Secure HTTP response headers (X-Frame-Options, CSP, etc.)", ConfigFields: []configField{}},
		{Name: "security.csrf", Description: "Cross-Site Request Forgery protection", ConfigFields: []configField{
			{Key: "cookie_name", Type: "string", Description: "Name of the CSRF cookie"},
			{Key: "cookie_secure", Type: "boolean", Description: "Set Secure flag on cookie"},
			{Key: "cookie_http_only", Type: "boolean", Description: "Set HttpOnly flag on cookie"},
			{Key: "cookie_same_site", Type: "string", Description: "SameSite attribute (Strict, Lax, None)"},
			{Key: "cookie_session_only", Type: "boolean", Description: "Cookie expires when browser closes"},
			{Key: "single_use_token", Type: "boolean", Description: "Generate a new token after each request"},
		}},
		{Name: "limiter", Description: "Rate limiting per IP address", ConfigFields: []configField{
			{Key: "max", Type: "number", Description: "Maximum requests per window"},
			{Key: "expiration", Type: "string", Description: "Time window duration", Placeholder: "1m"},
		}},
		{Name: "timeout", Description: "Request timeout enforcement", ConfigFields: []configField{
			{Key: "duration", Type: "string", Description: "Maximum request duration", Placeholder: "30s"},
		}},
		{Name: "casbin.enforce", Description: "Role-based access control using Casbin policies", ConfigFields: []configField{
			{Key: "model", Type: "text", Description: "Casbin model definition (PERM format)", Required: true},
			{Key: "policies", Type: "text", Description: "Policy rules (one per line, CSV format: p, sub, obj, act)", Required: true},
			{Key: "tenant_param", Type: "string", Description: "URL parameter for tenant isolation"},
		}},
		{Name: "recover", Description: "Panic recovery — catches panics and returns 500", ConfigFields: []configField{}},
		{Name: "logger", Description: "Request logging with method, path, status, and latency", ConfigFields: []configField{}},
		{Name: "requestid", Description: "Generates a unique X-Request-ID for each request", ConfigFields: []configField{}},
		{Name: "compress", Description: "Response compression (gzip, deflate, brotli)", ConfigFields: []configField{}},
		{Name: "etag", Description: "ETag-based response caching", ConfigFields: []configField{}},
	}

	// Extract presets from resolved config
	presets := make(map[string][]string)
	rc := e.resolvedConfig()
	if rc != nil {
		if mp, ok := rc.Root["middleware_presets"].(map[string]any); ok {
			for name, v := range mp {
				if arr, ok := v.([]any); ok {
					mws := make([]string, 0, len(arr))
					for _, item := range arr {
						if s, ok := item.(string); ok {
							mws = append(mws, s)
						}
					}
					presets[name] = mws
				}
			}
		}
	}

	// Extract current config for each middleware
	mwConfig := make(map[string]any)
	if rc != nil {
		for _, d := range descriptors {
			if len(d.ConfigFields) == 0 {
				continue
			}
			cfg := extractMiddlewareConfig(d.Name, rc.Root)
			if cfg != nil {
				mwConfig[d.Name] = cfg
			}
		}
	}

	// Extract middleware instances
	instances := make(map[string]any)
	if rc != nil {
		if mi, ok := rc.Root["middleware_instances"].(map[string]any); ok {
			instances = mi
		}
	}

	return c.JSON(map[string]any{
		"middleware": descriptors,
		"presets":    presets,
		"config":     mwConfig,
		"instances":  instances,
	})
}

// listEnvVars scans all config for $env() references and returns their status.
func (e *EditorAPI) listEnvVars(c fiber.Ctx) error {
	rc := e.resolvedConfig()
	if rc == nil {
		return c.Status(500).JSON(map[string]any{"error": "no config available"})
	}

	// Collect all env var references from the raw config data
	envVars := make(map[string]map[string]any) // varName -> {defined, sources}

	// Scan all config maps for $env() references
	configSources := map[string]any{
		"root": rc.Root,
	}
	for path, route := range rc.Routes {
		configSources[relPath(e.configDir, path)] = route
	}
	for path, wf := range rc.Workflows {
		configSources[relPath(e.configDir, path)] = wf
	}
	for path, worker := range rc.Workers {
		configSources[relPath(e.configDir, path)] = worker
	}

	for source, data := range configSources {
		refs := findEnvRefs(data)
		for _, varName := range refs {
			if _, exists := envVars[varName]; !exists {
				_, defined := os.LookupEnv(varName)
				envVars[varName] = map[string]any{
					"name":    varName,
					"defined": defined,
					"sources": []string{},
				}
			}
			sources := envVars[varName]["sources"].([]string)
			// Avoid duplicate sources
			found := false
			for _, s := range sources {
				if s == source {
					found = true
					break
				}
			}
			if !found {
				envVars[varName]["sources"] = append(sources, source)
			}
		}
	}

	result := make([]map[string]any, 0, len(envVars))
	for _, v := range envVars {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i]["name"].(string) < result[j]["name"].(string)
	})

	return c.JSON(map[string]any{"variables": result})
}

// listVars scans all config for $var() references and returns variable info with usages.
func (e *EditorAPI) listVars(c fiber.Ctx) error {
	rc := e.resolvedConfig()
	if rc == nil {
		return c.Status(500).JSON(map[string]any{"error": "no config available"})
	}

	// Load raw vars from discovered files (pre-resolution values)
	discovered, err := config.Discover(e.configDir, e.envFlag)
	if err != nil {
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}

	// Load vars.json to get defined values
	definedVars := make(map[string]string)
	if discovered.Vars != "" {
		raw, loadErrs := config.LoadAll(discovered)
		if len(loadErrs) == 0 && raw.Vars != nil {
			definedVars = raw.Vars
		}
	}

	// Scan all config sections for $var() references
	varUsages := make(map[string][]string) // varName -> []filePath

	configSources := make(map[string]any)
	for path, route := range rc.Routes {
		configSources[relPath(e.configDir, path)] = route
	}
	for path, wf := range rc.Workflows {
		configSources[relPath(e.configDir, path)] = wf
	}
	for path, worker := range rc.Workers {
		configSources[relPath(e.configDir, path)] = worker
	}
	for path, sched := range rc.Schedules {
		configSources[relPath(e.configDir, path)] = sched
	}
	for path, conn := range rc.Connections {
		configSources[relPath(e.configDir, path)] = conn
	}

	for source, data := range configSources {
		refs := findVarRefs(data)
		for _, varName := range refs {
			// Deduplicate sources
			sources := varUsages[varName]
			found := false
			for _, s := range sources {
				if s == source {
					found = true
					break
				}
			}
			if !found {
				varUsages[varName] = append(sources, source)
			}
		}
	}

	// Build result: include all defined vars + any referenced-but-undefined vars
	seen := make(map[string]bool)
	result := make([]map[string]any, 0)

	for name, value := range definedVars {
		seen[name] = true
		usages := varUsages[name]
		if usages == nil {
			usages = []string{}
		}
		result = append(result, map[string]any{
			"name":   name,
			"value":  value,
			"usages": usages,
		})
	}

	for name, usages := range varUsages {
		if seen[name] {
			continue
		}
		result = append(result, map[string]any{
			"name":   name,
			"value":  "",
			"usages": usages,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i]["name"].(string) < result[j]["name"].(string)
	})

	return c.JSON(map[string]any{"variables": result})
}

// findVarRefs recursively searches a config value for $var() references.
func findVarRefs(v any) []string {
	var refs []string
	switch val := v.(type) {
	case string:
		matches := config.VarPattern().FindAllStringSubmatch(val, -1)
		for _, m := range matches {
			if len(m) >= 2 {
				refs = append(refs, m[1])
			}
		}
	case map[string]any:
		for _, child := range val {
			refs = append(refs, findVarRefs(child)...)
		}
	case []any:
		for _, child := range val {
			refs = append(refs, findVarRefs(child)...)
		}
	}
	return refs
}

// findEnvRefs recursively searches a config value for $env() references.
func findEnvRefs(v any) []string {
	var refs []string
	switch val := v.(type) {
	case string:
		// Match $env(VAR_NAME) patterns
		s := val
		for {
			idx := strings.Index(s, "$env(")
			if idx < 0 {
				break
			}
			rest := s[idx+5:]
			end := strings.Index(rest, ")")
			if end < 0 {
				break
			}
			varName := strings.TrimSpace(rest[:end])
			if varName != "" {
				refs = append(refs, varName)
			}
			s = rest[end+1:]
		}
	case map[string]any:
		for _, child := range val {
			refs = append(refs, findEnvRefs(child)...)
		}
	case []any:
		for _, child := range val {
			refs = append(refs, findEnvRefs(child)...)
		}
	}
	return refs
}

// expressionContext returns available variables for a node's position in a workflow.
func (e *EditorAPI) expressionContext(c fiber.Ctx) error {
	workflowName := c.Query("workflow")
	nodeID := c.Query("node")

	if workflowName == "" {
		return c.Status(400).JSON(map[string]any{"error": "workflow query parameter required"})
	}

	// Load workflow config
	rc := e.resolvedConfig()
	var wfConfig map[string]any
	if rc != nil {
		for id, wf := range rc.Workflows {
			if id == workflowName {
				wfConfig = wf
				break
			}
		}
	}

	// Build context variables
	vars := make([]map[string]any, 0)

	// Always available: input
	vars = append(vars, map[string]any{
		"name":        "input",
		"type":        "object",
		"description": "Workflow input data from trigger",
	})

	// Always available: trigger
	vars = append(vars, map[string]any{
		"name":        "trigger",
		"type":        "object",
		"description": "Trigger metadata (method, path, trace_id, etc.)",
	})

	// Always available: auth
	vars = append(vars, map[string]any{
		"name":        "auth",
		"type":        "object",
		"description": "Authentication context (user_id, roles, claims)",
	})

	// Built-in functions
	builtins := []map[string]any{
		{"name": "$uuid()", "type": "function", "description": "Generate UUID v4"},
		{"name": "now()", "type": "function", "description": "Current timestamp"},
		{"name": "upper(s)", "type": "function", "description": "Uppercase string"},
		{"name": "lower(s)", "type": "function", "description": "Lowercase string"},
		{"name": "len(v)", "type": "function", "description": "Length of array/string/map"},
		{"name": "toInt(v)", "type": "function", "description": "Convert to integer"},
		{"name": "toFloat(v)", "type": "function", "description": "Convert to float"},
	}

	// If we have a workflow config and node ID, find upstream nodes
	upstreamNodes := make([]map[string]any, 0)
	if wfConfig != nil && nodeID != "" {
		upstreamNodes = e.findUpstreamNodes(wfConfig, nodeID)
	}

	return c.JSON(map[string]any{
		"variables": vars,
		"functions": builtins,
		"upstream":  upstreamNodes,
	})
}

// findUpstreamNodes walks the workflow graph backwards from the given node
// and returns all upstream node IDs with their types.
func (e *EditorAPI) findUpstreamNodes(wfConfig map[string]any, targetNodeID string) []map[string]any {
	// Parse edges
	rawEdges, _ := wfConfig["edges"].([]any)
	type edge struct{ from, output, to string }
	var edges []edge
	for _, re := range rawEdges {
		em, _ := re.(map[string]any)
		if em == nil {
			continue
		}
		edges = append(edges, edge{
			from:   fmt.Sprintf("%v", em["from"]),
			output: fmt.Sprintf("%v", em["output"]),
			to:     fmt.Sprintf("%v", em["to"]),
		})
	}

	// Parse node types
	nodeTypes := make(map[string]string)
	rawNodes, _ := wfConfig["nodes"].(map[string]any)
	for id, v := range rawNodes {
		if nm, ok := v.(map[string]any); ok {
			nodeTypes[id] = fmt.Sprintf("%v", nm["type"])
		}
	}

	// BFS backwards from targetNodeID
	visited := map[string]bool{}
	queue := []string{targetNodeID}
	var result []map[string]any

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		for _, edge := range edges {
			if edge.to == current && !visited[edge.from] {
				queue = append(queue, edge.from)
				result = append(result, map[string]any{
					"node_id":   edge.from,
					"node_type": nodeTypes[edge.from],
					"ref":       fmt.Sprintf("nodes.%s", edge.from),
				})
			}
		}
	}

	return result
}
