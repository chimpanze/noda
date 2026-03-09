package registry

import "fmt"

// InitializeServices creates service instances from the root config's "services" map.
// Each service entry must have a "plugin" field referencing a registered plugin prefix.
func InitializeServices(servicesConfig map[string]any, plugins *PluginRegistry) (*ServiceRegistry, []error) {
	registry := NewServiceRegistry()
	var errs []error

	for name, raw := range servicesConfig {
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

		plugin, found := plugins.Get(pluginName)
		if !found {
			errs = append(errs, fmt.Errorf("service %q: unknown plugin %q", name, pluginName))
			continue
		}

		if !plugin.HasServices() {
			errs = append(errs, fmt.Errorf("service %q: plugin %q does not support services", name, pluginName))
			continue
		}

		instance, err := plugin.CreateService(cfg)
		if err != nil {
			errs = append(errs, fmt.Errorf("service %q: create failed: %w", name, err))
			continue
		}

		if err := registry.Register(name, instance, plugin); err != nil {
			errs = append(errs, fmt.Errorf("service %q: %w", name, err))
		}
	}

	return registry, errs
}

// HealthCheckAll runs health checks on all registered services.
func (r *ServiceRegistry) HealthCheckAll() []error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var errs []error
	for name, entry := range r.services {
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
		if err := entry.plugin.Shutdown(entry.instance); err != nil {
			errs = append(errs, fmt.Errorf("service %q shutdown failed: %w", name, err))
		}
	}
	return errs
}
