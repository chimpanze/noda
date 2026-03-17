package registry

import (
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// InitializeServices creates service instances from the root config's "services" map.
// Each service entry must have a "plugin" field referencing a registered plugin prefix.
func InitializeServices(servicesConfig map[string]any, plugins *PluginRegistry) (*ServiceRegistry, []error) {
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
		const createTimeout = 30 * time.Second
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
func (r *ServiceRegistry) HealthCheckAll() []error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for _, name := range r.order {
		entry := r.services[name]
		if entry.plugin == nil {
			continue
		}
		if err := entry.plugin.HealthCheck(entry.instance); err != nil {
			errs = append(errs, fmt.Errorf("service %q health check failed: %w", name, err))
		}
	}
	return errs
}

// ShutdownAll shuts down all services in reverse initialization order.
func (r *ServiceRegistry) ShutdownAll() []error {
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
		if err := entry.plugin.Shutdown(entry.instance); err != nil {
			errs = append(errs, fmt.Errorf("service %q shutdown failed: %w", name, err))
		}
	}
	return errs
}
