package server

// MiddlewareBuildError reports a middleware that fails to build, naming every
// config file whose routes or connection endpoints reference it.
//
// Files is a slice, not a single path, because one misconfigured middleware
// breaks every route that names it — the editor must mark all of them. It is
// empty for global_middleware, which is declared project-wide and belongs to
// no route file; internal/startup maps that case to the root config.
//
// Err is already fully formatted. Error() returns it verbatim so that lifting
// the file attribution out changed no output — #450's determinism tests pin
// this text.
type MiddlewareBuildError struct {
	// Name is the middleware as referenced, including any ":instance" suffix.
	Name string
	// Files are the absolute paths of the route and connection files
	// referencing it, in sorted order. Empty for global_middleware.
	Files []string
	// Err is the underlying failure, pre-wrapped with scopes and name.
	Err error
}

func (e *MiddlewareBuildError) Error() string { return e.Err.Error() }

func (e *MiddlewareBuildError) Unwrap() error { return e.Err }
