package server

import (
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
)

func (e *EditorAPI) listOutputSchemas(c fiber.Ctx) error {
	types := e.nodes.AllTypes()
	schemas := make(map[string]any, len(types))

	for _, t := range types {
		desc, ok := e.nodes.GetDescriptor(t)
		if !ok {
			continue
		}
		if provider, ok := desc.(api.NodeOutputSchemaProvider); ok {
			schemas[t] = provider.OutputSchema()
		}
	}

	return c.JSON(map[string]any{"schemas": schemas})
}
