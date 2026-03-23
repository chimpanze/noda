package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gofiber/fiber/v3"
)

// GenerateOpenAPI creates an OpenAPI 3.1 spec from the resolved config.
func GenerateOpenAPI(rc *config.ResolvedConfig) (*openapi3.T, error) {
	doc := &openapi3.T{
		OpenAPI: "3.1.0",
		Info: &openapi3.Info{
			Title:   getStringFromRoot(rc.Root, "name", "Noda API"),
			Version: getStringFromRoot(rc.Root, "version", "1.0.0"),
		},
		Paths: openapi3.NewPaths(),
	}

	// Add server if configured
	if serverCfg, ok := rc.Root["server"].(map[string]any); ok {
		if port, ok := serverCfg["port"].(float64); ok {
			doc.Servers = openapi3.Servers{
				{URL: fmt.Sprintf("http://localhost:%d", int(port))},
			}
		}
	}

	// Build component schemas from config schemas
	if len(rc.Schemas) > 0 {
		doc.Components = &openapi3.Components{
			Schemas: make(openapi3.Schemas),
		}
		for name, schema := range rc.Schemas {
			schemaRef, err := convertSchema(schema)
			if err != nil {
				continue // skip invalid schemas
			}
			doc.Components.Schemas[name] = schemaRef
		}
	}

	// Add JWT security scheme if configured
	if sec, ok := rc.Root["security"].(map[string]any); ok {
		if _, ok := sec["jwt"].(map[string]any); ok {
			if doc.Components == nil {
				doc.Components = &openapi3.Components{}
			}
			doc.Components.SecuritySchemes = openapi3.SecuritySchemes{
				"bearerAuth": &openapi3.SecuritySchemeRef{
					Value: &openapi3.SecurityScheme{
						Type:         "http",
						Scheme:       "bearer",
						BearerFormat: "JWT",
					},
				},
			}
		}
	}

	// Build paths from routes
	for _, route := range rc.Routes {
		method, _ := route["method"].(string)
		path, _ := route["path"].(string)
		if method == "" || path == "" {
			continue
		}

		// Convert Fiber path params (:id) to OpenAPI ({id})
		oaPath := fiberToOpenAPIPath(path)

		operation := &openapi3.Operation{
			OperationID: routeToOperationID(route),
		}

		// Summary
		if summary, ok := route["summary"].(string); ok {
			operation.Summary = summary
		}

		// Tags
		if tags, ok := route["tags"].([]any); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok {
					operation.Tags = append(operation.Tags, s)
				}
			}
		}

		// Path parameters
		addPathParams(operation, oaPath)

		// Query parameters
		if queryDef, ok := route["query"].(map[string]any); ok {
			addQueryParams(operation, queryDef)
		}

		// Request body
		if bodyDef, ok := route["body"].(map[string]any); ok {
			addRequestBody(operation, bodyDef, rc)
		}

		// Response definitions
		addResponses(operation, route, rc)

		// Security (if route uses JWT middleware)
		if hasJWTMiddleware(route) {
			operation.Security = &openapi3.SecurityRequirements{
				{"bearerAuth": {}},
			}
		}

		// Add operation to path
		pathItem := doc.Paths.Find(oaPath)
		if pathItem == nil {
			pathItem = &openapi3.PathItem{}
			doc.Paths.Set(oaPath, pathItem)
		}

		switch strings.ToUpper(method) {
		case "GET":
			pathItem.Get = operation
		case "POST":
			pathItem.Post = operation
		case "PUT":
			pathItem.Put = operation
		case "PATCH":
			pathItem.Patch = operation
		case "DELETE":
			pathItem.Delete = operation
		}
	}

	return doc, nil
}

// RegisterOpenAPIRoutes adds /openapi.json and /docs endpoints.
func (s *Server) RegisterOpenAPIRoutes() error {
	doc, err := GenerateOpenAPI(s.config)
	if err != nil {
		return err
	}

	specBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal openapi spec: %w", err)
	}

	// Serve raw spec
	s.app.Get("/openapi.json", func(c fiber.Ctx) error {
		c.Set("Content-Type", "application/json")
		return c.Send(specBytes)
	})

	// Serve Scalar UI
	s.app.Get("/docs", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html")
		return c.SendString(scalarHTML())
	})

	return nil
}

func fiberToOpenAPIPath(path string) string {
	// Convert :param to {param}
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			name := strings.TrimPrefix(p, ":")
			name = strings.TrimSuffix(name, "?") // optional params
			parts[i] = "{" + name + "}"
		}
	}
	return strings.Join(parts, "/")
}

func routeToOperationID(route map[string]any) string {
	if id, ok := route["id"].(string); ok {
		return id
	}
	method, _ := route["method"].(string)
	path, _ := route["path"].(string)
	return strings.ToLower(method) + "_" + strings.ReplaceAll(strings.Trim(path, "/"), "/", "_")
}

func addPathParams(op *openapi3.Operation, path string) {
	parts := strings.Split(path, "/")
	for _, p := range parts {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			name := strings.Trim(p, "{}")
			op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
				Value: &openapi3.Parameter{
					Name:     name,
					In:       "path",
					Required: true,
					Schema: &openapi3.SchemaRef{
						Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
					},
				},
			})
		}
	}
}

func addQueryParams(op *openapi3.Operation, queryDef map[string]any) {
	if schema, ok := queryDef["schema"].(map[string]any); ok {
		if props, ok := schema["properties"].(map[string]any); ok {
			for name, propRaw := range props {
				prop, _ := propRaw.(map[string]any)
				paramSchema := &openapi3.Schema{Type: &openapi3.Types{"string"}}
				if t, ok := prop["type"].(string); ok {
					paramSchema.Type = &openapi3.Types{t}
				}
				op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
					Value: &openapi3.Parameter{
						Name: name,
						In:   "query",
						Schema: &openapi3.SchemaRef{
							Value: paramSchema,
						},
					},
				})
			}
		}
	}
}

func addRequestBody(op *openapi3.Operation, bodyDef map[string]any, _ *config.ResolvedConfig) {
	contentType := "application/json"
	if ct, ok := bodyDef["content_type"].(string); ok {
		contentType = ct
	}

	var schemaRef *openapi3.SchemaRef
	if schemaDef, ok := bodyDef["schema"].(map[string]any); ok {
		if ref, ok := schemaDef["$ref"].(string); ok {
			// Reference to a named schema
			schemaName := strings.TrimPrefix(ref, "schemas/")
			schemaRef = &openapi3.SchemaRef{
				Ref: "#/components/schemas/" + schemaName,
			}
		} else {
			s, _ := convertSchema(schemaDef)
			schemaRef = s
		}
	}

	if schemaRef == nil {
		schemaRef = &openapi3.SchemaRef{
			Value: &openapi3.Schema{Type: &openapi3.Types{"object"}},
		}
	}

	op.RequestBody = &openapi3.RequestBodyRef{
		Value: &openapi3.RequestBody{
			Required: true,
			Content: openapi3.Content{
				contentType: &openapi3.MediaType{
					Schema: schemaRef,
				},
			},
		},
	}
}

func addResponses(op *openapi3.Operation, route map[string]any, _ *config.ResolvedConfig) {
	op.Responses = openapi3.NewResponses()

	if respDef, ok := route["response"].(map[string]any); ok {
		for status, def := range respDef {
			respMap, ok := def.(map[string]any)
			if !ok {
				continue
			}
			desc := "Response"
			if d, ok := respMap["description"].(string); ok {
				desc = d
			}

			resp := &openapi3.Response{
				Description: &desc,
			}

			if schemaDef, ok := respMap["schema"].(map[string]any); ok {
				var schemaRef *openapi3.SchemaRef
				if ref, ok := schemaDef["$ref"].(string); ok {
					schemaName := strings.TrimPrefix(ref, "schemas/")
					schemaRef = &openapi3.SchemaRef{
						Ref: "#/components/schemas/" + schemaName,
					}
				} else {
					schemaRef, _ = convertSchema(schemaDef)
				}

				if schemaRef != nil {
					resp.Content = openapi3.Content{
						"application/json": &openapi3.MediaType{
							Schema: schemaRef,
						},
					}
				}
			}

			op.Responses.Set(status, &openapi3.ResponseRef{Value: resp})
		}
	}

	// Add default error response if not defined
	if op.Responses.Len() == 0 {
		desc := "Success"
		op.Responses.Set("200", &openapi3.ResponseRef{
			Value: &openapi3.Response{Description: &desc},
		})
	}
}

func hasJWTMiddleware(route map[string]any) bool {
	if mw, ok := route["middleware"].([]any); ok {
		for _, m := range mw {
			if s, ok := m.(string); ok && s == "auth.jwt" {
				return true
			}
		}
	}
	if preset, ok := route["middleware_preset"].(string); ok {
		return strings.Contains(preset, "auth")
	}
	return false
}

func convertSchema(schema map[string]any) (*openapi3.SchemaRef, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	s := &openapi3.Schema{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return &openapi3.SchemaRef{Value: s}, nil
}

func getStringFromRoot(root map[string]any, key, defaultVal string) string {
	if v, ok := root[key].(string); ok {
		return v
	}
	return defaultVal
}

func scalarHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
  <title>API Documentation</title>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <style>body { margin: 0; }</style>
</head>
<body>
  <script id="api-reference" data-url="/openapi.json"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`
}
