package mcp

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"
	"regexp"
	"strings"

	nodadocs "github.com/chimpanze/noda/docs"
	configschemas "github.com/chimpanze/noda/internal/config/schemas"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerResources(s *server.MCPServer) {
	// Static doc resources
	docResources := []struct {
		uri, name, desc, file string
	}{
		{"noda://docs/quick-start", "Quick Start Guide", "How to get started with Noda in 5 minutes", "01-getting-started/quick-start.md"},
		{"noda://docs/expressions", "Expression Reference", "Noda expression syntax, context variables, and built-in functions", "01-getting-started/expressions.md"},
		{"noda://docs/data-flow", "Data Flow Guide", "How data flows through workflows: trigger input, node outputs, aliases, and data threading", "01-getting-started/data-flow.md"},
	}

	for _, r := range docResources {
		file := r.file // capture
		s.AddResource(
			mcp.NewResource(r.uri, r.name,
				mcp.WithResourceDescription(r.desc),
				mcp.WithMIMEType("text/markdown"),
			),
			staticDocHandler(file),
		)
	}

	// Node documentation template
	s.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"noda://docs/nodes/{type}",
			"Node Documentation",
			mcp.WithTemplateDescription("Documentation for a specific Noda node type (e.g. db.query, control.if)"),
			mcp.WithTemplateMIMEType("text/markdown"),
		),
		nodeDocHandler,
	)

	// Config schema template
	s.AddResourceTemplate(
		mcp.NewResourceTemplate(
			"noda://schemas/{type}",
			"Config Schema",
			mcp.WithTemplateDescription("JSON Schema for a Noda config file type (root, route, workflow, worker, schedule, connections, test)"),
			mcp.WithTemplateMIMEType("application/json"),
		),
		schemaResourceHandler,
	)
}

func staticDocHandler(file string) func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		data, err := nodadocs.FS.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read doc %s: %w", file, err)
		}
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      req.Params.URI,
				MIMEType: "text/markdown",
				Text:     string(data),
			},
		}, nil
	}
}

var nodeDocPattern = regexp.MustCompile(`noda://docs/nodes/(.+)`)

func nodeDocHandler(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	matches := nodeDocPattern.FindStringSubmatch(req.Params.URI)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid node doc URI: %s", req.Params.URI)
	}
	nodeType := matches[1]

	// Node docs are stored as 03-nodes/{type}.md
	filename := fmt.Sprintf("03-nodes/%s.md", nodeType)
	data, err := nodadocs.FS.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("no documentation found for node type %q", nodeType)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "text/markdown",
			Text:     string(data),
		},
	}, nil
}

var schemaPattern = regexp.MustCompile(`noda://schemas/(.+)`)

func schemaResourceHandler(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	matches := schemaPattern.FindStringSubmatch(req.Params.URI)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid schema URI: %s", req.Params.URI)
	}
	schemaType := matches[1]

	validTypes := map[string]string{
		"root":        "root.json",
		"route":       "route.json",
		"workflow":    "workflow.json",
		"worker":      "worker.json",
		"schedule":    "schedule.json",
		"connections": "connections.json",
		"test":        "test.json",
	}

	filename, ok := validTypes[schemaType]
	if !ok {
		valid := make([]string, 0, len(validTypes))
		for k := range validTypes {
			valid = append(valid, k)
		}
		return nil, fmt.Errorf("unknown schema type %q, valid types: %s",
			schemaType, strings.Join(valid, ", "))
	}

	data, err := configschemas.FS.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema %s: %w", filename, err)
	}

	// Pretty-print the JSON
	var schema any
	if err := json.Unmarshal(data, &schema); err == nil {
		if pretty, err := json.MarshalIndent(schema, "", "  "); err == nil {
			data = pretty
		}
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
