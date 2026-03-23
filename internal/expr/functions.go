package expr

import (
	"crypto/hmac"
	"crypto/md5" //nolint:gosec // MD5 exposed as expression function for non-security hashing
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type registeredFunc struct {
	fn          func(params ...any) (any, error)
	types       []any // type hints for expr
	description string
	signature   string
}

// FunctionInfo describes a registered function for introspection.
type FunctionInfo struct {
	Name        string `json:"name"`
	Signature   string `json:"signature"`
	Description string `json:"description"`
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
	r.RegisterWithInfo("$uuid", func(params ...any) (any, error) {
		return uuid.New().String(), nil
	}, "Generate a UUID v4 string", "() string", new(func() string))

	// now() → current time
	r.RegisterWithInfo("now", func(params ...any) (any, error) {
		return time.Now(), nil
	}, "Returns the current time", "() time.Time", new(func() time.Time))

	// lower(string) → lowercase
	r.RegisterWithInfo("lower", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("lower: expected 1 argument, got %d", len(params))
		}
		s, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("lower: expected string argument, got %T", params[0])
		}
		return strings.ToLower(s), nil
	}, "Convert string to lowercase", "(string) string", new(func(string) string))

	// upper(string) → uppercase
	r.RegisterWithInfo("upper", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("upper: expected 1 argument, got %d", len(params))
		}
		s, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("upper: expected string argument, got %T", params[0])
		}
		return strings.ToUpper(s), nil
	}, "Convert string to uppercase", "(string) string", new(func(string) string))

	// toInt(value) → int (coerces strings and floats)
	r.RegisterWithInfo("toInt", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("toInt: expected 1 argument, got %d", len(params))
		}
		return coerceToInt(params[0])
	}, "Convert value to integer (coerces strings and floats)", "(any) int", new(func(any) int))

	// toFloat(value) → float64 (coerces strings and ints)
	r.RegisterWithInfo("toFloat", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("toFloat: expected 1 argument, got %d", len(params))
		}
		return coerceToFloat(params[0])
	}, "Convert value to float64 (coerces strings and ints)", "(any) float64", new(func(any) float64))

	// sha256(string) → hex-encoded SHA-256 hash
	r.RegisterWithInfo("sha256", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("sha256: expected 1 argument, got %d", len(params))
		}
		s, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("sha256: expected string argument, got %T", params[0])
		}
		h := sha256.Sum256([]byte(s))
		return hex.EncodeToString(h[:]), nil
	}, "Returns hex-encoded SHA-256 hash", "(string) string", new(func(string) string))

	// sha512(string) → hex-encoded SHA-512 hash
	r.RegisterWithInfo("sha512", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("sha512: expected 1 argument, got %d", len(params))
		}
		s, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("sha512: expected string argument, got %T", params[0])
		}
		h := sha512.Sum512([]byte(s))
		return hex.EncodeToString(h[:]), nil
	}, "Returns hex-encoded SHA-512 hash", "(string) string", new(func(string) string))

	// md5(string) → hex-encoded MD5 hash
	r.RegisterWithInfo("md5", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("md5: expected 1 argument, got %d", len(params))
		}
		s, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("md5: expected string argument, got %T", params[0])
		}
		h := md5.Sum([]byte(s)) //nolint:gosec // intentional non-security hash function
		return hex.EncodeToString(h[:]), nil
	}, "Returns hex-encoded MD5 hash", "(string) string", new(func(string) string))

	// hmac(data, key, algorithm) → hex-encoded HMAC
	r.RegisterWithInfo("hmac", func(params ...any) (any, error) {
		if len(params) != 3 {
			return nil, fmt.Errorf("hmac: expected 3 arguments, got %d", len(params))
		}
		data, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("hmac: expected string for data argument, got %T", params[0])
		}
		key, ok := params[1].(string)
		if !ok {
			return nil, fmt.Errorf("hmac: expected string for key argument, got %T", params[1])
		}
		algorithm, ok := params[2].(string)
		if !ok {
			return nil, fmt.Errorf("hmac: expected string for algorithm argument, got %T", params[2])
		}
		var mac []byte
		switch algorithm {
		case "sha256":
			h := hmac.New(sha256.New, []byte(key))
			h.Write([]byte(data))
			mac = h.Sum(nil)
		case "sha512":
			h := hmac.New(sha512.New, []byte(key))
			h.Write([]byte(data))
			mac = h.Sum(nil)
		default:
			return nil, fmt.Errorf("hmac: unsupported algorithm %q (use \"sha256\" or \"sha512\")", algorithm)
		}
		return hex.EncodeToString(mac), nil
	}, "Returns hex-encoded HMAC (sha256 or sha512)", "(data, key, algorithm string) string", new(func(string, string, string) string))

	// bcrypt_hash(password) → bcrypt hash string
	r.RegisterWithInfo("bcrypt_hash", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("bcrypt_hash: expected 1 argument, got %d", len(params))
		}
		s, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("bcrypt_hash: expected string argument, got %T", params[0])
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(s), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("bcrypt_hash: %w", err)
		}
		return string(hash), nil
	}, "Returns a bcrypt hash of the password", "(password string) string", new(func(string) string))

	// bcrypt_verify(password, hash) → bool
	r.RegisterWithInfo("bcrypt_verify", func(params ...any) (any, error) {
		if len(params) != 2 {
			return nil, fmt.Errorf("bcrypt_verify: expected 2 arguments, got %d", len(params))
		}
		password, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("bcrypt_verify: expected string for password argument, got %T", params[0])
		}
		hash, ok := params[1].(string)
		if !ok {
			return nil, fmt.Errorf("bcrypt_verify: expected string for hash argument, got %T", params[1])
		}
		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
		return err == nil, nil
	}, "Returns true if password matches the bcrypt hash", "(password, hash string) bool", new(func(string, string) bool))

	return r
}

// register adds a custom function to the registry (test helper, prefer RegisterWithInfo).
func (r *FunctionRegistry) register(name string, fn func(params ...any) (any, error), types ...any) {
	r.functions[name] = registeredFunc{fn: fn, types: types}
}

// RegisterWithInfo adds a custom function with description and signature metadata.
func (r *FunctionRegistry) RegisterWithInfo(name string, fn func(params ...any) (any, error), desc, sig string, types ...any) {
	r.functions[name] = registeredFunc{fn: fn, types: types, description: desc, signature: sig}
}

// RegisteredFunctions returns info about all registered functions, sorted by name.
func (r *FunctionRegistry) RegisteredFunctions() []FunctionInfo {
	infos := make([]FunctionInfo, 0, len(r.functions))
	for name, rf := range r.functions {
		infos = append(infos, FunctionInfo{
			Name:        name,
			Signature:   rf.signature,
			Description: rf.description,
		})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	return infos
}

// RegisteredNames returns the names of all registered functions sorted alphabetically.
func (r *FunctionRegistry) RegisteredNames() []string {
	names := make([]string, 0, len(r.functions))
	for name := range r.functions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ExprOptions returns expr.Option values for use with the compiler.
func (r *FunctionRegistry) ExprOptions() []expr.Option {
	var opts []expr.Option
	for name, rf := range r.functions {
		opts = append(opts, expr.Function(name, rf.fn, rf.types...))
	}
	return opts
}

// NewFunctionRegistryWithVars creates a registry with built-in functions plus a $var function
// that looks up keys from the provided vars map. If vars is nil, $var() returns an error.
func NewFunctionRegistryWithVars(vars map[string]string) *FunctionRegistry {
	r := NewFunctionRegistry()

	r.RegisterWithInfo("$var", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("$var: expected 1 argument, got %d", len(params))
		}
		key, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("$var: expected string argument, got %T", params[0])
		}
		if vars == nil {
			return nil, fmt.Errorf("$var: unknown variable %q (no vars defined)", key)
		}
		val, exists := vars[key]
		if !exists {
			return nil, fmt.Errorf("$var: unknown variable %q", key)
		}
		return val, nil
	}, "Look up a shared variable by key from vars.json", "(key string) string", new(func(string) string))

	return r
}

// NewCompilerWithVars creates a compiler with built-in functions and $var() bound to the given vars map.
// Additional CompilerOption values are applied after the defaults.
func NewCompilerWithVars(vars map[string]string, opts ...CompilerOption) *Compiler {
	allOpts := []CompilerOption{WithExprOptions(NewFunctionRegistryWithVars(vars).ExprOptions()...), WithMaxCacheSize(10000)}
	allOpts = append(allOpts, opts...)
	return NewCompiler(allOpts...)
}

// NewCompilerWithFunctions creates a compiler with the built-in function registry.
// Additional CompilerOption values are applied after the defaults.
func NewCompilerWithFunctions(opts ...CompilerOption) *Compiler {
	return NewCompilerWithVars(nil, opts...)
}

// coerceToInt converts a value to int. Handles string, float64, int, and json.Number.
func coerceToInt(v any) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case float64:
		return int(val), nil
	case string:
		if i, err := strconv.Atoi(val); err == nil {
			return i, nil
		}
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return int(f), nil
		}
		return 0, fmt.Errorf("toInt: cannot convert %q to int", val)
	default:
		return 0, fmt.Errorf("toInt: unsupported type %T", v)
	}
}

// coerceToFloat converts a value to float64. Handles string, int, float64, and json.Number.
func coerceToFloat(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, nil
		}
		return 0, fmt.Errorf("toFloat: cannot convert %q to float", val)
	default:
		return 0, fmt.Errorf("toFloat: unsupported type %T", v)
	}
}
