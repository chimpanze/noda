package expr

import "fmt"

// Resolver provides runtime expression resolution using pre-compiled expressions.
type Resolver struct {
	compiler *Compiler
	context  map[string]any
}

// NewResolver creates a new resolver with the given compiler and runtime context.
func NewResolver(compiler *Compiler, context map[string]any) *Resolver {
	return &Resolver{
		compiler: compiler,
		context:  context,
	}
}

// Resolve evaluates a single expression string against the current context.
// The expression is looked up in the compiler's cache (pre-compiled at startup).
func (r *Resolver) Resolve(expression string) (any, error) {
	compiled, err := r.compiler.Compile(expression)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", expression, err)
	}
	return r.compiler.Evaluate(compiled, r.context)
}

// ResolveMap recursively walks a config map and resolves all string values
// that contain expressions. Non-string values pass through unchanged.
func (r *Resolver) ResolveMap(config map[string]any) (map[string]any, error) {
	result := make(map[string]any, len(config))
	for k, v := range config {
		resolved, err := r.resolveValue(v)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", k, err)
		}
		result[k] = resolved
	}
	return result, nil
}

func (r *Resolver) resolveValue(v any) (any, error) {
	switch val := v.(type) {
	case string:
		return r.Resolve(val)
	case map[string]any:
		return r.ResolveMap(val)
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			resolved, err := r.resolveValue(item)
			if err != nil {
				return nil, err
			}
			result[i] = resolved
		}
		return result, nil
	default:
		return v, nil
	}
}
