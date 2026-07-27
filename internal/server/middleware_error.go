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

func (e *MiddlewareBuildError) MiddlewareFiles() []string { return e.Files }

// MiddlewareFilesError is implemented by every error ValidateMiddlewareBuilds
// returns that knows which config files it implicates.
//
// internal/startup matches this interface rather than each concrete type, so a
// typed error added here gains file attribution on every surface without an
// edit there. Matching concrete types is how the two faults below reached the
// editor with an empty Files, where its per-file filter dropped them and a
// route that could not boot reported valid: true.
type MiddlewareFilesError interface {
	error
	MiddlewareFiles() []string
}

// MiddlewareChainError reports a route's or connection endpoint's middleware
// chain that could not be resolved at all — an unknown middleware preset, or a
// violated ordering constraint — as opposed to a named middleware that fails to
// build, which is MiddlewareBuildError.
//
// It is a sibling of that type rather than a reuse of it because neither fault
// names a single middleware: a preset expansion names a preset, and an ordering
// violation names a pair of them. MiddlewareBuildError.Name would have to be
// empty for both, and an empty Name there means "declared project-wide", not
// "not applicable".
//
// Err is already fully formatted, and Error() returns it verbatim: both faults
// were plain fmt.Errorf values with exactly this text before they were typed,
// and #450's determinism tests pin it.
type MiddlewareChainError struct {
	// Files holds the absolute path of the config file declaring the failing
	// scope — the route file, or the connection file for an endpoint. It is a
	// slice for one reason: to satisfy MiddlewareFilesError alongside
	// MiddlewareBuildError, whose faults genuinely span files.
	Files []string
	// Err is the underlying failure, pre-wrapped with its scope.
	Err error
}

func (e *MiddlewareChainError) Error() string { return e.Err.Error() }

func (e *MiddlewareChainError) Unwrap() error { return e.Err }

func (e *MiddlewareChainError) MiddlewareFiles() []string { return e.Files }
