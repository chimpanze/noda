package registry

import "fmt"

// ServiceConfigError identifies an error produced by ValidateStartupDryRun's
// service-schema step (#376): a service's `config` block failed its plugin's
// declared ServiceConfigSchema. It exists as a typed error (rather than a
// plain fmt.Errorf) so callers that need to distinguish "this file's own
// content is broken" from "some other service in the project is broken" can
// do so with errors.As instead of string-matching the message.
//
// The concrete case this serves: internal/server/editor_validation.go's
// validateFile endpoint validates one file at a time, but the startup
// dry-run's service-schema check always reads rc.Root (services are declared
// project-wide, not per-file) — so without this type, a bad service config in
// noda.json would incorrectly surface as an error on whatever unrelated file
// the editor happened to be saving.
type ServiceConfigError struct {
	Service string // service name, e.g. "db"
	Plugin  string // plugin name, e.g. "postgres"
	Err     error
}

func (e *ServiceConfigError) Error() string {
	return fmt.Sprintf("service %q (plugin %q): %s", e.Service, e.Plugin, e.Err.Error())
}

func (e *ServiceConfigError) Unwrap() error {
	return e.Err
}
