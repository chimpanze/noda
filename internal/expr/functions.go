package expr

import (
	"fmt"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/google/uuid"
)

type registeredFunc struct {
	fn    func(params ...any) (any, error)
	types []any // type hints for expr
}

// FunctionRegistry holds custom functions available in expressions.
type FunctionRegistry struct {
	functions map[string]registeredFunc
}

// NewFunctionRegistry creates a registry with built-in Noda functions.
func NewFunctionRegistry() *FunctionRegistry {
	r := &FunctionRegistry{
		functions: make(map[string]registeredFunc),
	}

	// $uuid() → UUID v4 string
	r.Register("$uuid", func(params ...any) (any, error) {
		return uuid.New().String(), nil
	}, new(func() string))

	// now() → current time
	r.Register("now", func(params ...any) (any, error) {
		return time.Now(), nil
	}, new(func() time.Time))

	// lower(string) → lowercase
	r.Register("lower", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("lower: expected 1 argument, got %d", len(params))
		}
		s, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("lower: expected string argument, got %T", params[0])
		}
		return strings.ToLower(s), nil
	}, new(func(string) string))

	// upper(string) → uppercase
	r.Register("upper", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("upper: expected 1 argument, got %d", len(params))
		}
		s, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("upper: expected string argument, got %T", params[0])
		}
		return strings.ToUpper(s), nil
	}, new(func(string) string))

	return r
}

// Register adds a custom function to the registry.
// types are function signature hints for the expr compiler (e.g., new(func(string) string)).
func (r *FunctionRegistry) Register(name string, fn func(params ...any) (any, error), types ...any) {
	r.functions[name] = registeredFunc{fn: fn, types: types}
}

// ExprOptions returns expr.Option values for use with the compiler.
func (r *FunctionRegistry) ExprOptions() []expr.Option {
	var opts []expr.Option
	for name, rf := range r.functions {
		opts = append(opts, expr.Function(name, rf.fn, rf.types...))
	}
	return opts
}

// NewCompilerWithFunctions creates a compiler with the built-in function registry.
func NewCompilerWithFunctions() *Compiler {
	reg := NewFunctionRegistry()
	return NewCompiler(WithExprOptions(reg.ExprOptions()...))
}
