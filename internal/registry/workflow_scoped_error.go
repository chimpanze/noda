package registry

// WorkflowScopedError attributes a registries-phase failure to the workflow
// file that produced it. ValidateStartup and ValidateStartupDryRun each run
// several "for wfName, wf := range rc.Workflows" loops (node-type, config-
// schema, service-slot, edge-output, and expression checks) whose errors
// already name wfName in their formatted message, but until now that file was
// only recoverable by parsing that text back out — exactly what this branch
// exists to stop doing (see engine.WorkflowCompileError and
// server.MiddlewareBuildError, which attribute their own phases the same
// way). Wrapping the already-formatted error in this type lets
// internal/startup.attributeRegistries recover the file with errors.As
// instead.
//
// Not used for the service-schema check (registry.ServiceConfigError): that
// failure is project-wide (declared in the root config, not a workflow file)
// and already has its own typed error.
type WorkflowScopedError struct {
	// File is the rc.Workflows map key (source file path) that declared the
	// workflow producing this error.
	File string
	// Err is the underlying failure, pre-formatted with wfName and node/edge
	// context. Error() returns it verbatim — this type adds structure, not
	// new text.
	Err error
}

func (e *WorkflowScopedError) Error() string { return e.Err.Error() }

func (e *WorkflowScopedError) Unwrap() error { return e.Err }
