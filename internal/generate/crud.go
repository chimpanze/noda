package generate

import (
	"fmt"
	"strings"
)

// CRUDOptions configures what to generate.
type CRUDOptions struct {
	Service    string   // db service name (default: "db")
	BasePath   string   // URL prefix (default: "/api/{table}")
	Operations []string // which ops: create, list, get, update, delete
	Artifacts  []string // which types: routes, workflows, schemas
	ScopeCol   string   // tenant scope column
	ScopeParam string   // URL param for scope
}

// CRUDResult holds generated file content.
type CRUDResult struct {
	Files map[string]map[string]any // relative path -> JSON content
}

// GenerateCRUD produces route, workflow, and schema files for a model.
func GenerateCRUD(model map[string]any, opts CRUDOptions) CRUDResult {
	table, _ := model["table"].(string)
	if table == "" {
		return CRUDResult{Files: map[string]map[string]any{}}
	}

	// Defaults
	if opts.Service == "" {
		opts.Service = "db"
	}
	if opts.BasePath == "" {
		opts.BasePath = "/api/" + table
	}
	if len(opts.Operations) == 0 {
		opts.Operations = []string{"create", "list", "get", "update", "delete"}
	}
	if len(opts.Artifacts) == 0 {
		opts.Artifacts = []string{"routes", "workflows", "schemas"}
	}

	// Parse columns
	columns := parseColumns(model)
	singular := singularize(table)

	result := CRUDResult{Files: make(map[string]map[string]any)}

	artifactSet := make(map[string]bool)
	for _, a := range opts.Artifacts {
		artifactSet[a] = true
	}
	opSet := make(map[string]bool)
	for _, o := range opts.Operations {
		opSet[o] = true
	}

	// Generate schemas
	if artifactSet["schemas"] {
		result.Files[fmt.Sprintf("schemas/models/%s.json", capitalize(singular))] = generateModelSchemas(singular, columns)
	}

	// Generate routes and workflows
	for _, op := range opts.Operations {
		if !opSet[op] {
			continue
		}

		switch op {
		case "create":
			wfName := "create-" + singular
			if artifactSet["routes"] {
				result.Files["routes/"+wfName+".json"] = map[string]any{
					"method":   "POST",
					"path":     opts.BasePath,
					"workflow": wfName,
					"body": map[string]any{
						"schema": map[string]any{"$ref": fmt.Sprintf("schemas/models/%s.json#/Create%s", capitalize(singular), capitalize(singular))},
					},
					"summary": fmt.Sprintf("Create a new %s", singular),
					"tags":    []any{table},
				}
			}
			if artifactSet["workflows"] {
				result.Files["workflows/"+wfName+".json"] = generateCreateWorkflow(table, singular, columns, opts)
			}

		case "list":
			wfName := "list-" + table
			if artifactSet["routes"] {
				result.Files["routes/"+wfName+".json"] = map[string]any{
					"method":   "GET",
					"path":     opts.BasePath,
					"workflow": wfName,
					"summary":  fmt.Sprintf("List %s", table),
					"tags":     []any{table},
				}
			}
			if artifactSet["workflows"] {
				result.Files["workflows/"+wfName+".json"] = generateListWorkflow(table, columns, opts)
			}

		case "get":
			wfName := "get-" + singular
			if artifactSet["routes"] {
				result.Files["routes/"+wfName+".json"] = map[string]any{
					"method":   "GET",
					"path":     opts.BasePath + "/:id",
					"workflow": wfName,
					"summary":  fmt.Sprintf("Get a %s by ID", singular),
					"tags":     []any{table},
				}
			}
			if artifactSet["workflows"] {
				result.Files["workflows/"+wfName+".json"] = generateGetWorkflow(table, singular, opts)
			}

		case "update":
			wfName := "update-" + singular
			if artifactSet["routes"] {
				result.Files["routes/"+wfName+".json"] = map[string]any{
					"method":   "PUT",
					"path":     opts.BasePath + "/:id",
					"workflow": wfName,
					"body": map[string]any{
						"schema": map[string]any{"$ref": fmt.Sprintf("schemas/models/%s.json#/Update%s", capitalize(singular), capitalize(singular))},
					},
					"summary": fmt.Sprintf("Update a %s", singular),
					"tags":    []any{table},
				}
			}
			if artifactSet["workflows"] {
				result.Files["workflows/"+wfName+".json"] = generateUpdateWorkflow(table, singular, opts)
			}

		case "delete":
			wfName := "delete-" + singular
			if artifactSet["routes"] {
				result.Files["routes/"+wfName+".json"] = map[string]any{
					"method":   "DELETE",
					"path":     opts.BasePath + "/:id",
					"workflow": wfName,
					"summary":  fmt.Sprintf("Delete a %s", singular),
					"tags":     []any{table},
				}
			}
			if artifactSet["workflows"] {
				result.Files["workflows/"+wfName+".json"] = generateDeleteWorkflow(table, singular, opts)
			}
		}
	}

	return result
}

type colInfo struct {
	Name       string
	Type       string
	PrimaryKey bool
	NotNull    bool
	Default    string
	Enum       []string
}

func parseColumns(model map[string]any) []colInfo {
	cols, _ := model["columns"].(map[string]any)
	var result []colInfo
	for name, v := range cols {
		def, _ := v.(map[string]any)
		if def == nil {
			continue
		}
		ci := colInfo{Name: name}
		ci.Type, _ = def["type"].(string)
		ci.PrimaryKey, _ = def["primary_key"].(bool)
		ci.NotNull, _ = def["not_null"].(bool)
		ci.Default, _ = def["default"].(string)
		if enumArr, ok := def["enum"].([]any); ok {
			for _, e := range enumArr {
				if s, ok := e.(string); ok {
					ci.Enum = append(ci.Enum, s)
				}
			}
		}
		result = append(result, ci)
	}
	return result
}

func generateModelSchemas(singular string, columns []colInfo) map[string]any {
	createProps := make(map[string]any)
	updateProps := make(map[string]any)
	var createRequired []any

	for _, col := range columns {
		if col.PrimaryKey {
			continue // PK is auto-generated
		}
		if col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" {
			continue
		}

		prop := map[string]any{"type": jsonSchemaType(col.Type)}
		if len(col.Enum) > 0 {
			enumVals := make([]any, len(col.Enum))
			for i, e := range col.Enum {
				enumVals[i] = e
			}
			prop["enum"] = enumVals
		}

		createProps[col.Name] = prop
		updateProps[col.Name] = prop

		if col.NotNull && col.Default == "" {
			createRequired = append(createRequired, col.Name)
		}
	}

	createSchema := map[string]any{
		"type":       "object",
		"properties": createProps,
	}
	if len(createRequired) > 0 {
		createSchema["required"] = createRequired
	}

	updateSchema := map[string]any{
		"type":       "object",
		"properties": updateProps,
	}

	return map[string]any{
		"Create" + capitalize(singular): createSchema,
		"Update" + capitalize(singular): updateSchema,
	}
}

func generateCreateWorkflow(table, singular string, columns []colInfo, opts CRUDOptions) map[string]any {
	nodes := map[string]any{}
	edges := []any{}

	// Check if there's a UUID PK
	hasUUIDPK := false
	for _, col := range columns {
		if col.PrimaryKey && col.Type == "uuid" {
			hasUUIDPK = true
			break
		}
	}

	prevNode := ""

	if hasUUIDPK {
		nodes["gen_id"] = map[string]any{
			"type": "transform.set",
			"config": map[string]any{
				"values": map[string]any{
					"id": "$uuid()",
				},
			},
		}
		prevNode = "gen_id"
	}

	// Build data mapping
	data := map[string]any{}
	for _, col := range columns {
		if col.PrimaryKey && hasUUIDPK {
			data["id"] = "nodes.gen_id.id"
			continue
		}
		if col.PrimaryKey {
			continue
		}
		if col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" {
			continue
		}
		data[col.Name] = fmt.Sprintf("input.%s", col.Name)
	}
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		data[opts.ScopeCol] = fmt.Sprintf("trigger.params.%s", opts.ScopeParam)
	}

	createConfig := map[string]any{
		"table": table,
		"data":  data,
	}

	nodes["create"] = map[string]any{
		"type":     "db.create",
		"config":   createConfig,
		"services": map[string]any{"db": opts.Service},
	}

	if prevNode != "" {
		edges = append(edges, map[string]any{"from": prevNode, "to": "create"})
	}

	nodes["respond"] = map[string]any{
		"type": "response.json",
		"config": map[string]any{
			"status": 201,
			"body":   "nodes.create",
		},
	}
	edges = append(edges, map[string]any{"from": "create", "to": "respond"})

	return map[string]any{
		"name":  "create-" + singular,
		"nodes": nodes,
		"edges": edges,
	}
}

func generateListWorkflow(table string, columns []colInfo, opts CRUDOptions) map[string]any {
	nodes := map[string]any{}
	edges := []any{}

	where := map[string]any{}
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		where[opts.ScopeCol] = fmt.Sprintf("trigger.params.%s", opts.ScopeParam)
	}

	findConfig := map[string]any{
		"table":  table,
		"limit":  "input.limit ?? 20",
		"offset": "input.offset ?? 0",
	}
	if len(where) > 0 {
		findConfig["where"] = where
	}

	nodes["find"] = map[string]any{
		"type":     "db.find",
		"config":   findConfig,
		"services": map[string]any{"db": opts.Service},
	}

	countConfig := map[string]any{
		"table": table,
	}
	if len(where) > 0 {
		countConfig["where"] = where
	}

	nodes["count"] = map[string]any{
		"type":     "db.count",
		"config":   countConfig,
		"services": map[string]any{"db": opts.Service},
	}

	nodes["respond"] = map[string]any{
		"type": "response.json",
		"config": map[string]any{
			"body": map[string]any{
				"data":  "nodes.find",
				"total": "nodes.count.count",
			},
		},
	}

	edges = append(edges, map[string]any{"from": "find", "to": "respond"})
	edges = append(edges, map[string]any{"from": "count", "to": "respond"})

	return map[string]any{
		"name":  "list-" + table,
		"nodes": nodes,
		"edges": edges,
	}
}

func generateGetWorkflow(table, singular string, opts CRUDOptions) map[string]any {
	nodes := map[string]any{}
	edges := []any{}

	where := map[string]any{
		"id": "trigger.params.id",
	}
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		where[opts.ScopeCol] = fmt.Sprintf("trigger.params.%s", opts.ScopeParam)
	}

	nodes["find_one"] = map[string]any{
		"type": "db.findOne",
		"config": map[string]any{
			"table":    table,
			"where":    where,
			"required": true,
		},
		"services": map[string]any{"db": opts.Service},
	}

	nodes["respond"] = map[string]any{
		"type": "response.json",
		"config": map[string]any{
			"body": "nodes.find_one",
		},
	}

	edges = append(edges, map[string]any{"from": "find_one", "to": "respond"})

	return map[string]any{
		"name":  "get-" + singular,
		"nodes": nodes,
		"edges": edges,
	}
}

func generateUpdateWorkflow(table, singular string, opts CRUDOptions) map[string]any {
	nodes := map[string]any{}
	edges := []any{}

	where := map[string]any{
		"id": "trigger.params.id",
	}
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		where[opts.ScopeCol] = fmt.Sprintf("trigger.params.%s", opts.ScopeParam)
	}

	nodes["find_one"] = map[string]any{
		"type": "db.findOne",
		"config": map[string]any{
			"table":    table,
			"where":    where,
			"required": true,
		},
		"services": map[string]any{"db": opts.Service},
	}

	nodes["update"] = map[string]any{
		"type": "db.update",
		"config": map[string]any{
			"table": table,
			"where": where,
			"data":  "input",
		},
		"services": map[string]any{"db": opts.Service},
	}

	nodes["respond"] = map[string]any{
		"type": "response.json",
		"config": map[string]any{
			"body": "nodes.update",
		},
	}

	edges = append(edges, map[string]any{"from": "find_one", "to": "update"})
	edges = append(edges, map[string]any{"from": "update", "to": "respond"})

	return map[string]any{
		"name":  "update-" + singular,
		"nodes": nodes,
		"edges": edges,
	}
}

func generateDeleteWorkflow(table, singular string, opts CRUDOptions) map[string]any {
	nodes := map[string]any{}
	edges := []any{}

	where := map[string]any{
		"id": "trigger.params.id",
	}
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		where[opts.ScopeCol] = fmt.Sprintf("trigger.params.%s", opts.ScopeParam)
	}

	nodes["delete"] = map[string]any{
		"type": "db.delete",
		"config": map[string]any{
			"table": table,
			"where": where,
		},
		"services": map[string]any{"db": opts.Service},
	}

	nodes["respond"] = map[string]any{
		"type": "response.json",
		"config": map[string]any{
			"status": 204,
		},
	}

	edges = append(edges, map[string]any{"from": "delete", "to": "respond"})

	return map[string]any{
		"name":  "delete-" + singular,
		"nodes": nodes,
		"edges": edges,
	}
}

func jsonSchemaType(colType string) string {
	switch colType {
	case "integer", "int", "bigint", "serial":
		return "integer"
	case "decimal":
		return "number"
	case "boolean", "bool":
		return "boolean"
	default:
		return "string"
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "zzes") {
		return s[:len(s)-3]
	}
	if strings.HasSuffix(s, "ches") || strings.HasSuffix(s, "shes") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "ses") || strings.HasSuffix(s, "xes") || strings.HasSuffix(s, "zes") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "us") || strings.HasSuffix(s, "ss") {
		return s
	}
	if strings.HasSuffix(s, "s") {
		return s[:len(s)-1]
	}
	return s
}
