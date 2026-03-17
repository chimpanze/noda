package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestFactoryFunctions(t *testing.T) {
	tests := []struct {
		name     string
		factory  func() Component
		expected string
	}{
		{"ServerComponent", func() Component { return ServerComponent(nil) }, "http-server"},
		{"WorkerComponent", func() Component { return WorkerComponent(nil) }, "workers"},
		{"SchedulerComponent", func() Component { return SchedulerComponent(nil) }, "scheduler"},
		{"WasmComponent", func() Component { return WasmComponent(nil) }, "wasm"},
		{"ConnManagerComponent", func() Component { return ConnManagerComponent(nil) }, "connections"},
		{"ServiceRegistryComponent", func() Component { return ServiceRegistryComponent(nil) }, "services"},
		{"TracerComponent", func() Component { return TracerComponent(nil) }, "tracer"},
		{"WatcherComponent", func() Component { return WatcherComponent(nil) }, "file-watcher"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.factory()
			require.NotNil(t, c)
			assert.Equal(t, tt.expected, c.Name())
		})
	}
}

func TestNoOpStartMethods(t *testing.T) {
	// These adapters have no-op Start methods (return nil without dereferencing inner value)
	ctx := context.Background()

	t.Run("serverComponent", func(t *testing.T) {
		c := &serverComponent{}
		assert.NoError(t, c.Start(ctx))
	})
	t.Run("connManagerComponent", func(t *testing.T) {
		c := &connManagerComponent{}
		assert.NoError(t, c.Start(ctx))
	})
	t.Run("serviceRegistryComponent", func(t *testing.T) {
		c := &serviceRegistryComponent{}
		assert.NoError(t, c.Start(ctx))
	})
	t.Run("tracerComponent", func(t *testing.T) {
		c := &tracerComponent{}
		assert.NoError(t, c.Start(ctx))
	})
}
