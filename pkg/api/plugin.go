package api

// Plugin is the top-level interface for all Noda plugins.
type Plugin interface {
	Name() string
	Prefix() string
	Nodes() []NodeRegistration
	HasServices() bool
	CreateService(config map[string]any) (any, error)
	HealthCheck(service any) error
	Shutdown(service any) error

	// ServiceConfigSchema returns a JSON Schema (as a Go map, same
	// conventions as NodeDescriptor.ConfigSchema) describing this plugin's
	// service `config` block. Structural only: required keys, types,
	// enums, descriptions — no value-content constraints, because values
	// are frequently $env()-resolved and may be empty at validate time.
	// Plugins without services return nil.
	ServiceConfigSchema() map[string]any
}
