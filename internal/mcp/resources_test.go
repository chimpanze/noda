package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeReadResourceRequest(uri string) mcp.ReadResourceRequest {
	return mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: uri,
		},
	}
}

func TestStaticDocHandler(t *testing.T) {
	t.Run("quick-start", func(t *testing.T) {
		handler := staticDocHandler("01-getting-started/quick-start.md")
		req := makeReadResourceRequest("noda://docs/quick-start")
		contents, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, contents, 1)

		text := contents[0].(mcp.TextResourceContents)
		assert.Equal(t, "text/markdown", text.MIMEType)
		assert.NotEmpty(t, text.Text)
	})

	t.Run("expressions", func(t *testing.T) {
		handler := staticDocHandler("01-getting-started/expressions.md")
		req := makeReadResourceRequest("noda://docs/expressions")
		contents, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, contents, 1)
		assert.NotEmpty(t, contents[0].(mcp.TextResourceContents).Text)
	})

	t.Run("expression-cookbook", func(t *testing.T) {
		handler := staticDocHandler("01-getting-started/expression-cookbook.md")
		req := makeReadResourceRequest("noda://docs/expression-cookbook")
		contents, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, contents, 1)
		assert.NotEmpty(t, contents[0].(mcp.TextResourceContents).Text)
	})

	t.Run("services", func(t *testing.T) {
		handler := staticDocHandler("01-getting-started/services.md")
		req := makeReadResourceRequest("noda://docs/services")
		contents, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, contents, 1)
		assert.NotEmpty(t, contents[0].(mcp.TextResourceContents).Text)
	})

	t.Run("workflow-patterns", func(t *testing.T) {
		handler := staticDocHandler("04-guides/workflow-patterns.md")
		req := makeReadResourceRequest("noda://docs/workflow-patterns")
		contents, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, contents, 1)
		assert.NotEmpty(t, contents[0].(mcp.TextResourceContents).Text)
	})

	t.Run("authentication", func(t *testing.T) {
		handler := staticDocHandler("04-guides/authentication.md")
		req := makeReadResourceRequest("noda://docs/authentication")
		contents, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, contents, 1)
		assert.NotEmpty(t, contents[0].(mcp.TextResourceContents).Text)
	})

	t.Run("testing-and-debugging", func(t *testing.T) {
		handler := staticDocHandler("04-guides/testing-and-debugging.md")
		req := makeReadResourceRequest("noda://docs/testing")
		contents, err := handler(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, contents, 1)
		assert.NotEmpty(t, contents[0].(mcp.TextResourceContents).Text)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		handler := staticDocHandler("nonexistent.md")
		req := makeReadResourceRequest("noda://docs/nonexistent")
		_, err := handler(context.Background(), req)
		assert.Error(t, err)
	})
}

func TestNodeDocHandler(t *testing.T) {
	t.Run("valid node type", func(t *testing.T) {
		req := makeReadResourceRequest("noda://docs/nodes/db.query")
		contents, err := nodeDocHandler(context.Background(), req)
		require.NoError(t, err)
		require.Len(t, contents, 1)

		text := contents[0].(mcp.TextResourceContents)
		assert.Equal(t, "text/markdown", text.MIMEType)
		assert.NotEmpty(t, text.Text)
	})

	t.Run("unknown node type", func(t *testing.T) {
		req := makeReadResourceRequest("noda://docs/nodes/nonexistent.thing")
		_, err := nodeDocHandler(context.Background(), req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no documentation found")
	})

	t.Run("invalid URI", func(t *testing.T) {
		req := makeReadResourceRequest("invalid://uri")
		_, err := nodeDocHandler(context.Background(), req)
		assert.Error(t, err)
	})
}

func TestSchemaResourceHandler(t *testing.T) {
	validTypes := []string{"root", "route", "workflow", "worker", "schedule", "connections", "test"}

	for _, schemaType := range validTypes {
		t.Run(schemaType, func(t *testing.T) {
			req := makeReadResourceRequest("noda://schemas/" + schemaType)
			contents, err := schemaResourceHandler(context.Background(), req)
			require.NoError(t, err)
			require.Len(t, contents, 1)

			text := contents[0].(mcp.TextResourceContents)
			assert.Equal(t, "application/json", text.MIMEType)

			var schema map[string]any
			assert.NoError(t, json.Unmarshal([]byte(text.Text), &schema))
		})
	}

	t.Run("unknown type", func(t *testing.T) {
		req := makeReadResourceRequest("noda://schemas/bogus")
		_, err := schemaResourceHandler(context.Background(), req)
		assert.Error(t, err)
	})

	t.Run("invalid URI", func(t *testing.T) {
		req := makeReadResourceRequest("invalid://uri")
		_, err := schemaResourceHandler(context.Background(), req)
		assert.Error(t, err)
	})
}
