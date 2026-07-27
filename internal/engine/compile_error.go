package engine

// WorkflowCompileError reports a workflow that failed to parse or compile,
// carrying the source file that declared it.
//
// The file is a field rather than only a fragment of the message because
// internal/startup attributes failures to files so the editor can mark them,
// and parsing it back out of formatted text would break the moment the
// wording changed. Err is already fully formatted: Error() returns it
// verbatim, so adding this type changed no output.
type WorkflowCompileError struct {
	// File is the rc.Workflows map key that declared the workflow — an
	// absolute path to the source file.
	File string
	// Err is the underlying failure, pre-wrapped with its message.
	Err error
}

func (e *WorkflowCompileError) Error() string { return e.Err.Error() }

func (e *WorkflowCompileError) Unwrap() error { return e.Err }
