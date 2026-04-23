package registry

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// createTimeout bounds how long InitializeServices waits for a plugin's
// CreateService to return before declaring the service initialisation
// failed. Var (not const) so tests can override it.
var createTimeout = 30 * time.Second

// InitializeServices creates service instances from the root config's "services" map.
// Each service entry must have a "plugin" field referencing a registered plugin prefix.
//
// The supplied ctx bounds the cleanup goroutine spawned on createTimeout —
// when ctx is cancelled (typically at lifecycle shutdown), any cleanup
// goroutine still waiting for a hung CreateService exits, abandoning the
// late result rather than leaking.
func InitializeServices(ctx context.Context, servicesConfig map[string]any, plugins *PluginRegistry) (*ServiceRegistry, []error) {
	registry := NewServiceRegistry()
	var errs []error

	// Sort service names for deterministic initialization order.
	names := make([]string, 0, len(servicesConfig))
	for name := range servicesConfig {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		raw := servicesConfig[name]
		cfg, ok := raw.(map[string]any)
		if !ok {
			errs = append(errs, fmt.Errorf("service %q: config must be a map", name))
			continue
		}

		pluginName, ok := cfg["plugin"].(string)
		if !ok {
			errs = append(errs, fmt.Errorf("service %q: missing or invalid 'plugin' field", name))
			continue
		}

		plugin, found := plugins.GetByName(pluginName)
		if !found {
			errs = append(errs, fmt.Errorf("service %q: unknown plugin %q", name, pluginName))
			continue
		}

		if !plugin.HasServices() {
			errs = append(errs, fmt.Errorf("service %q: plugin %q does not support services", name, pluginName))
			continue
		}

		// Pass the inner "config" map to the plugin if present, otherwise the whole entry
		pluginCfg := cfg
		if inner, ok := cfg["config"].(map[string]any); ok {
			pluginCfg = inner
		}

		// Create service with timeout to fail fast if external dependencies are unreachable.
		type createResult struct {
			instance any
			err      error
		}
		resultCh := make(chan createResult, 1)
		go func() {
			inst, err := plugin.CreateService(pluginCfg)
			resultCh <- createResult{inst, err}
		}()

		var instance any
		select {
		case res := <-resultCh:
			if res.err != nil {
				errs = append(errs, fmt.Errorf("service %q: create failed: %w", name, res.err))
				continue
			}
			instance = res.instance
		case <-time.After(createTimeout):
			errs = append(errs, fmt.Errorf("service %q: creation timed out after %s", name, createTimeout))
			// Cleanup goroutine: wait for the orphan to complete OR for ctx
			// shutdown. On ctx shutdown, abandon the result (we'll leak the
			// underlying call goroutine, but not the cleanup one).
			go func(name string) {
				select {
				case res := <-resultCh:
					if res.err == nil && res.instance != nil {
						if closer, ok := res.instance.(interface{ Close() error }); ok {
							_ = closer.Close()
						}
						slog.Warn("timed-out service creation completed late, resource closed", "name", name)
					}
				case <-ctx.Done():
					slog.Warn("timed-out service creation cleanup abandoned at shutdown", "name", name)
				}
			}(name)
			continue
		}

		if err := registry.Register(name, instance, plugin); err != nil {
			errs = append(errs, fmt.Errorf("service %q: %w", name, err))
		} else {
			slog.Info("service initialized", "name", name, "plugin", pluginName)
		}
	}

	return registry, errs
}

// HealthCheckAll runs health checks on all registered services.
// Returns a map of service name to error for services that failed their health check.
func (r *ServiceRegistry) HealthCheckAll() map[string]error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make(map[string]error)
	for _, name := range r.order {
		entry := r.services[name]
		if entry.plugin == nil {
			continue
		}
		if err := entry.plugin.HealthCheck(entry.instance); err != nil {
			results[name] = err
		}
	}
	return results
}

// ShutdownAll shuts down all services in reverse initialization order.
// If ctx has a deadline, each service shutdown is bounded by the remaining time.
func (r *ServiceRegistry) ShutdownAll(ctx context.Context) []error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	// Reverse initialization order
	for i := len(r.order) - 1; i >= 0; i-- {
		name := r.order[i]
		entry := r.services[name]
		if entry.plugin == nil {
			continue
		}
		if err := shutdownWithContext(ctx, name, entry); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// shutdownWithContext runs a single service shutdown, respecting the context deadline.
func shutdownWithContext(ctx context.Context, name string, entry serviceEntry) error {
	done := make(chan error, 1)
	go func() {
		done <- entry.plugin.Shutdown(entry.instance)
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("service %q shutdown failed: %w", name, err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("service %q shutdown timed out: %w", name, ctx.Err())
	}
}
