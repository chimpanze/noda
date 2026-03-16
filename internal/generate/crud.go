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

// expr wraps a value in {{ }} expression syntax.
func expr(s string) string {
	return "{{ " + s + " }}"
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
				// Map body fields to workflow input
				inputMap := map[string]any{}
				for _, col := range columns {
					if col.PrimaryKey || col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" {
						continue
					}
					inputMap[col.Name] = expr("body." + col.Name)
				}
				result.Files["routes/"+wfName+".json"] = map[string]any{
					"id":     wfName + "-route",
					"method": "POST",
					"path":   opts.BasePath,
					"trigger": map[string]any{
						"workflow": wfName,
						"input":    inputMap,
					},
					"body": map[string]any{
						"schema": map[string]any{"$ref": fmt.Sprintf("schemas/models/Create%s", capitalize(singular))},
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
					"id":     wfName + "-route",
					"method": "GET",
					"path":   opts.BasePath,
					"query": map[string]any{
						"schema": map[string]any{"$ref": fmt.Sprintf("schemas/models/%sListQuery", capitalize(singular))},
					},
					"trigger": map[string]any{
						"workflow": wfName,
						"input": map[string]any{
							"limit":  expr("query.limit ?? '20'"),
							"offset": expr("query.offset ?? '0'"),
						},
					},
					"summary": fmt.Sprintf("List %s", table),
					"tags":    []any{table},
				}
			}
			if artifactSet["workflows"] {
				result.Files["workflows/"+wfName+".json"] = generateListWorkflow(table, columns, opts)
			}

		case "get":
			wfName := "get-" + singular
			if artifactSet["routes"] {
				result.Files["routes/"+wfName+".json"] = map[string]any{
					"id":     wfName + "-route",
					"method": "GET",
					"path":   opts.BasePath + "/:id",
					"params": map[string]any{
						"schema": map[string]any{"$ref": fmt.Sprintf("schemas/models/%sIdParams", capitalize(singular))},
					},
					"trigger": map[string]any{
						"workflow": wfName,
						"input": map[string]any{
							"id": expr("params.id"),
						},
					},
					"summary": fmt.Sprintf("Get a %s by ID", singular),
					"tags":    []any{table},
				}
			}
			if artifactSet["workflows"] {
				result.Files["workflows/"+wfName+".json"] = generateGetWorkflow(table, singular, opts)
			}

		case "update":
			wfName := "update-" + singular
			if artifactSet["routes"] {
				// Map body fields + path param to workflow input
				inputMap := map[string]any{
					"id": expr("params.id"),
				}
				for _, col := range columns {
					if col.PrimaryKey || col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" {
						continue
					}
					inputMap[col.Name] = expr("body." + col.Name)
				}
				result.Files["routes/"+wfName+".json"] = map[string]any{
					"id":     wfName + "-route",
					"method": "PUT",
					"path":   opts.BasePath + "/:id",
					"params": map[string]any{
						"schema": map[string]any{"$ref": fmt.Sprintf("schemas/models/%sIdParams", capitalize(singular))},
					},
					"trigger": map[string]any{
						"workflow": wfName,
						"input":    inputMap,
					},
					"body": map[string]any{
						"schema": map[string]any{"$ref": fmt.Sprintf("schemas/models/Update%s", capitalize(singular))},
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
					"id":     wfName + "-route",
					"method": "DELETE",
					"path":   opts.BasePath + "/:id",
					"params": map[string]any{
						"schema": map[string]any{"$ref": fmt.Sprintf("schemas/models/%sIdParams", capitalize(singular))},
					},
					"trigger": map[string]any{
						"workflow": wfName,
						"input": map[string]any{
							"id": expr("params.id"),
						},
					},
					"summary": fmt.Sprintf("Delete a %s", singular),
					"tags":    []any{table},
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
		capitalize(singular) + "IdParams": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "minLength": 1},
			},
			"required": []any{"id"},
		},
		capitalize(singular) + "ListQuery": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "string"},
				"offset": map[string]any{"type": "string"},
			},
		},
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
				"fields": map[string]any{
					"id": expr("$uuid()"),
				},
			},
		}
		prevNode = "gen_id"
	}

	// Build data mapping
	data := map[string]any{}
	for _, col := range columns {
		if col.PrimaryKey && hasUUIDPK {
			data["id"] = expr("nodes.gen_id.id")
			continue
		}
		if col.PrimaryKey {
			continue
		}
		if col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" {
			continue
		}
		data[col.Name] = expr(fmt.Sprintf("input.%s", col.Name))
	}
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		data[opts.ScopeCol] = expr(fmt.Sprintf("input.%s", opts.ScopeParam))
	}

	createConfig := map[string]any{
		"table": table,
		"data":  data,
	}

	nodes["create"] = map[string]any{
		"type":     "db.create",
		"config":   createConfig,
		"services": map[string]any{"database": opts.Service},
	}

	if prevNode != "" {
		edges = append(edges, map[string]any{"from": prevNode, "to": "create", "output": "success"})
	}

	nodes["respond"] = map[string]any{
		"type": "response.json",
		"config": map[string]any{
			"status": 201,
			"body":   expr("nodes.create"),
		},
	}
	edges = append(edges, map[string]any{"from": "create", "to": "respond", "output": "success"})

	return map[string]any{
		"id":    "create-" + singular,
		"nodes": nodes,
		"edges": edges,
	}
}

func generateListWorkflow(table string, columns []colInfo, opts CRUDOptions) map[string]any {
	nodes := map[string]any{}
	edges := []any{}

	where := map[string]any{}
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		where[opts.ScopeCol] = expr(fmt.Sprintf("input.%s", opts.ScopeParam))
	}

	findConfig := map[string]any{
		"table":  table,
		"limit":  expr("input.limit ?? 20"),
		"offset": expr("input.offset ?? 0"),
	}
	if len(where) > 0 {
		findConfig["where"] = where
	}

	nodes["find"] = map[string]any{
		"type":     "db.find",
		"config":   findConfig,
		"services": map[string]any{"database": opts.Service},
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
		"services": map[string]any{"database": opts.Service},
	}

	nodes["respond"] = map[string]any{
		"type": "response.json",
		"config": map[string]any{
			"body": map[string]any{
				"data":  expr("nodes.find"),
				"total": expr("nodes.count.count"),
			},
		},
	}

	edges = append(edges, map[string]any{"from": "find", "to": "respond", "output": "success"})
	edges = append(edges, map[string]any{"from": "count", "to": "respond", "output": "success"})

	return map[string]any{
		"id":    "list-" + table,
		"nodes": nodes,
		"edges": edges,
	}
}

func generateGetWorkflow(table, singular string, opts CRUDOptions) map[string]any {
	nodes := map[string]any{}
	edges := []any{}

	where := map[string]any{
		"id": expr("input.id"),
	}
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		where[opts.ScopeCol] = expr(fmt.Sprintf("input.%s", opts.ScopeParam))
	}

	nodes["find_one"] = map[string]any{
		"type": "db.findOne",
		"config": map[string]any{
			"table":    table,
			"where":    where,
			"required": true,
		},
		"services": map[string]any{"database": opts.Service},
	}

	nodes["respond"] = map[string]any{
		"type": "response.json",
		"config": map[string]any{
			"body": expr("nodes.find_one"),
		},
	}

	edges = append(edges, map[string]any{"from": "find_one", "to": "respond", "output": "success"})

	return map[string]any{
		"id":    "get-" + singular,
		"nodes": nodes,
		"edges": edges,
	}
}

func generateUpdateWorkflow(table, singular string, opts CRUDOptions) map[string]any {
	nodes := map[string]any{}
	edges := []any{}

	where := map[string]any{
		"id": expr("input.id"),
	}
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		where[opts.ScopeCol] = expr(fmt.Sprintf("input.%s", opts.ScopeParam))
	}

	nodes["find_one"] = map[string]any{
		"type": "db.findOne",
		"config": map[string]any{
			"table":    table,
			"where":    where,
			"required": true,
		},
		"services": map[string]any{"database": opts.Service},
	}

	nodes["update"] = map[string]any{
		"type": "db.update",
		"config": map[string]any{
			"table": table,
			"where": where,
			"data":  expr("input"),
		},
		"services": map[string]any{"database": opts.Service},
	}

	nodes["respond"] = map[string]any{
		"type": "response.json",
		"config": map[string]any{
			"body": expr("nodes.update"),
		},
	}

	edges = append(edges, map[string]any{"from": "find_one", "to": "update", "output": "success"})
	edges = append(edges, map[string]any{"from": "update", "to": "respond", "output": "success"})

	return map[string]any{
		"id":    "update-" + singular,
		"nodes": nodes,
		"edges": edges,
	}
}

func generateDeleteWorkflow(table, singular string, opts CRUDOptions) map[string]any {
	nodes := map[string]any{}
	edges := []any{}

	where := map[string]any{
		"id": expr("input.id"),
	}
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		where[opts.ScopeCol] = expr(fmt.Sprintf("input.%s", opts.ScopeParam))
	}

	nodes["delete"] = map[string]any{
		"type": "db.delete",
		"config": map[string]any{
			"table": table,
			"where": where,
		},
		"services": map[string]any{"database": opts.Service},
	}

	nodes["respond"] = map[string]any{
		"type": "response.json",
		"config": map[string]any{
			"status": 204,
		},
	}

	edges = append(edges, map[string]any{"from": "delete", "to": "respond", "output": "success"})

	return map[string]any{
		"id":    "delete-" + singular,
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
