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
}
