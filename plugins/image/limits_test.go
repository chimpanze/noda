package image

import (
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/require"
)

func makeImageCtx(t *testing.T) *engine.ExecutionContextImpl {
	t.Helper()
	return engine.NewExecutionContext(engine.WithInput(map[string]any{}))
}

func TestEnforceDimensionLimit(t *testing.T) {
	c := makeImageCtx(t) // minimal execution context
	require.NoError(t, enforceDimensionLimit(c, map[string]any{}, 800, 600))
	require.Error(t, enforceDimensionLimit(c, map[string]any{}, 100000, 600))     // > maxWidth
	require.Error(t, enforceDimensionLimit(c, map[string]any{}, 9000, 9000))      // > maxPixels (81MP)
	// per-node override raises the cap
	require.NoError(t, enforceDimensionLimit(c, map[string]any{"max_width": float64(200000), "max_pixels": float64(1 << 40)}, 100000, 600))
}
