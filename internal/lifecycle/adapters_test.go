package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdapterNamesAreUnique(t *testing.T) {
	// Instantiate each adapter with nil to verify Name() returns distinct values.
	// We can't call Start/Stop (nil receiver would panic), but Name() is safe.
	adapters := []Component{
		&serverComponent{},
		&workerComponent{},
		&schedulerComponent{},
		&wasmComponent{},
		&connManagerComponent{},
		&serviceRegistryComponent{},
		&tracerComponent{},
		&watcherComponent{},
	}

	seen := make(map[string]bool)
	for _, a := range adapters {
		name := a.Name()
		assert.NotEmpty(t, name, "adapter name must not be empty")
		assert.False(t, seen[name], "duplicate adapter name: %s", name)
		seen[name] = true
	}
	assert.Len(t, seen, 8)
}
