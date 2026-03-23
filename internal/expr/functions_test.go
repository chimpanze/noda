package expr

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func compileAndEvalWithFunctions(t *testing.T, input string, ctx map[string]any) any {
	t.Helper()
	c := NewCompilerWithFunctions()
	compiled, err := c.Compile(input)
	require.NoError(t, err)
	result, err := c.Evaluate(compiled, ctx)
	require.NoError(t, err)
	return result
}

func TestFunction_UUID(t *testing.T) {
	result := compileAndEvalWithFunctions(t, "{{ $uuid() }}", map[string]any{})
	s, ok := result.(string)
	require.True(t, ok)
	assert.Len(t, s, 36) // UUID v4 format: 8-4-4-4-12
	assert.Contains(t, s, "-")
}

func TestFunction_Lower(t *testing.T) {
	result := compileAndEvalWithFunctions(t, `{{ lower("HELLO") }}`, map[string]any{})
	assert.Equal(t, "hello", result)
}

func TestFunction_Upper(t *testing.T) {
	result := compileAndEvalWithFunctions(t, `{{ upper("hello") }}`, map[string]any{})
	assert.Equal(t, "HELLO", result)
}

func TestFunction_Now(t *testing.T) {
	before := time.Now()
	result := compileAndEvalWithFunctions(t, "{{ now() }}", map[string]any{})
	after := time.Now()

	ts, ok := result.(time.Time)
	require.True(t, ok)
	assert.True(t, !ts.Before(before) && !ts.After(after))
}

func TestFunction_LenWithArray(t *testing.T) {
	ctx := map[string]any{
		"items": []any{"a", "b", "c"},
	}
	result := compileAndEvalWithFunctions(t, "{{ len(items) }}", ctx)
	assert.Equal(t, 3, result)
}

func TestFunction_UnknownFunction(t *testing.T) {
	c := NewCompilerWithFunctions()
	compiled, err := c.Compile("{{ nonexistent() }}")
	require.NoError(t, err) // compiles with AllowUndefinedVariables

	// Fails at runtime
	_, err = c.Evaluate(compiled, map[string]any{})
	require.Error(t, err)
}

func TestFunction_Var_ReturnsValue(t *testing.T) {
	vars := map[string]string{"TOPIC": "events", "TABLE": "users"}
	c := NewCompilerWithVars(vars)
	compiled, err := c.Compile(`{{ $var('TOPIC') }}`)
	require.NoError(t, err)
	result, err := c.Evaluate(compiled, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "events", result)
}

func TestFunction_Var_InExpression(t *testing.T) {
	vars := map[string]string{"TABLE": "users"}
	c := NewCompilerWithVars(vars)
	compiled, err := c.Compile(`{{ $var('TABLE') + "_archive" }}`)
	require.NoError(t, err)
	result, err := c.Evaluate(compiled, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "users_archive", result)
}

func TestFunction_Var_MissingKey(t *testing.T) {
	vars := map[string]string{"TOPIC": "events"}
	c := NewCompilerWithVars(vars)
	compiled, err := c.Compile(`{{ $var('MISSING') }}`)
	require.NoError(t, err)
	_, err = c.Evaluate(compiled, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MISSING")
}

func TestFunction_Var_WrongArity(t *testing.T) {
	vars := map[string]string{"A": "1"}
	c := NewCompilerWithVars(vars)
	_, err := c.Compile(`{{ $var('A', 'B') }}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many arguments")
}

func TestFunction_Var_NilVars(t *testing.T) {
	c := NewCompilerWithVars(nil)
	compiled, err := c.Compile(`{{ $var('KEY') }}`)
	require.NoError(t, err)
	_, err = c.Evaluate(compiled, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "KEY")
	assert.Contains(t, err.Error(), "no vars defined")
}

func TestFunction_SHA256(t *testing.T) {
	result := compileAndEvalWithFunctions(t, `{{ sha256("hello") }}`, map[string]any{})
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", result)
}

func TestFunction_SHA256_WrongArity(t *testing.T) {
	c := NewCompilerWithFunctions()
	_, err := c.Compile(`{{ sha256("a", "b") }}`)
	require.Error(t, err)
}

func TestFunction_SHA256_WrongType(t *testing.T) {
	c := NewCompilerWithFunctions()
	_, err := c.Compile(`{{ sha256(123) }}`)
	require.Error(t, err)
}

func TestFunction_SHA512(t *testing.T) {
	result := compileAndEvalWithFunctions(t, `{{ sha512("hello") }}`, map[string]any{})
	s := result.(string)
	assert.Len(t, s, 128) // SHA-512 = 64 bytes = 128 hex chars
	assert.Equal(t, "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043", s)
}

func TestFunction_MD5(t *testing.T) {
	result := compileAndEvalWithFunctions(t, `{{ md5("hello") }}`, map[string]any{})
	assert.Equal(t, "5d41402abc4b2a76b9719d911017c592", result)
}

func TestFunction_MD5_WrongArity(t *testing.T) {
	c := NewCompilerWithFunctions()
	_, err := c.Compile(`{{ md5("a", "b") }}`)
	require.Error(t, err)
}

func TestFunction_HMAC_SHA256(t *testing.T) {
	result := compileAndEvalWithFunctions(t, `{{ hmac("hello", "secret", "sha256") }}`, map[string]any{})
	assert.Equal(t, "88aab3ede8d3adf94d26ab90d3bafd4a2083070c3bcce9c014ee04a443847c0b", result)
}

func TestFunction_HMAC_SHA512(t *testing.T) {
	result := compileAndEvalWithFunctions(t, `{{ hmac("hello", "secret", "sha512") }}`, map[string]any{})
	s := result.(string)
	assert.Len(t, s, 128)
}

func TestFunction_HMAC_InvalidAlgorithm(t *testing.T) {
	c := NewCompilerWithFunctions()
	compiled, err := c.Compile(`{{ hmac("hello", "secret", "md5") }}`)
	require.NoError(t, err)
	_, err = c.Evaluate(compiled, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported algorithm")
}

func TestFunction_HMAC_WrongArity(t *testing.T) {
	c := NewCompilerWithFunctions()
	_, err := c.Compile(`{{ hmac("a", "b") }}`)
	require.Error(t, err)
}

func TestFunction_BcryptHash(t *testing.T) {
	result := compileAndEvalWithFunctions(t, `{{ bcrypt_hash("password123") }}`, map[string]any{})
	hash, ok := result.(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(hash, "$2a$"))
	// Verify the hash is valid
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("password123"))
	assert.NoError(t, err)
}

func TestFunction_BcryptHash_WrongType(t *testing.T) {
	c := NewCompilerWithFunctions()
	_, err := c.Compile(`{{ bcrypt_hash(123) }}`)
	require.Error(t, err)
}

func TestFunction_BcryptVerify_Match(t *testing.T) {
	// First hash a password
	hashResult := compileAndEvalWithFunctions(t, `{{ bcrypt_hash("mypassword") }}`, map[string]any{})
	hash := hashResult.(string)

	// Then verify it
	ctx := map[string]any{"hash": hash}
	result := compileAndEvalWithFunctions(t, `{{ bcrypt_verify("mypassword", hash) }}`, ctx)
	assert.Equal(t, true, result)
}

func TestFunction_BcryptVerify_Mismatch(t *testing.T) {
	hashResult := compileAndEvalWithFunctions(t, `{{ bcrypt_hash("mypassword") }}`, map[string]any{})
	hash := hashResult.(string)

	ctx := map[string]any{"hash": hash}
	result := compileAndEvalWithFunctions(t, `{{ bcrypt_verify("wrongpassword", hash) }}`, ctx)
	assert.Equal(t, false, result)
}

func TestFunction_BcryptVerify_WrongArity(t *testing.T) {
	c := NewCompilerWithFunctions()
	_, err := c.Compile(`{{ bcrypt_verify("a") }}`)
	require.Error(t, err)
}

func TestFunctionRegistry_RegisteredFunctions(t *testing.T) {
	reg := NewFunctionRegistry()
	funcs := reg.RegisteredFunctions()

	// Build a lookup map
	byName := make(map[string]FunctionInfo, len(funcs))
	for _, f := range funcs {
		byName[f.Name] = f
	}

	expected := []struct {
		name string
		sig  string
		desc string
	}{
		{"$uuid", "() string", "Generate a UUID v4 string"},
		{"now", "() time.Time", "Returns the current time"},
		{"lower", "(string) string", "Convert string to lowercase"},
		{"upper", "(string) string", "Convert string to uppercase"},
		{"toInt", "(any) int", "Convert value to integer (coerces strings and floats)"},
		{"toFloat", "(any) float64", "Convert value to float64 (coerces strings and ints)"},
		{"sha256", "(string) string", "Returns hex-encoded SHA-256 hash"},
		{"sha512", "(string) string", "Returns hex-encoded SHA-512 hash"},
		{"md5", "(string) string", "Returns hex-encoded MD5 hash"},
		{"hmac", "(data, key, algorithm string) string", "Returns hex-encoded HMAC (sha256 or sha512)"},
		{"bcrypt_hash", "(password string) string", "Returns a bcrypt hash of the password"},
		{"bcrypt_verify", "(password, hash string) bool", "Returns true if password matches the bcrypt hash"},
	}

	assert.Len(t, funcs, len(expected))

	for _, e := range expected {
		f, ok := byName[e.name]
		require.True(t, ok, "missing function %q", e.name)
		assert.Equal(t, e.sig, f.Signature, "wrong signature for %q", e.name)
		assert.Equal(t, e.desc, f.Description, "wrong description for %q", e.name)
	}

	// Verify sorted order
	for i := 1; i < len(funcs); i++ {
		assert.True(t, funcs[i-1].Name < funcs[i].Name, "functions not sorted: %q >= %q", funcs[i-1].Name, funcs[i].Name)
	}
}

func TestFunctionRegistry_RegisteredNames(t *testing.T) {
	reg := NewFunctionRegistry()
	names := reg.RegisteredNames()

	assert.Len(t, names, 12)
	assert.Contains(t, names, "$uuid")
	assert.Contains(t, names, "lower")
	assert.Contains(t, names, "bcrypt_verify")

	// Verify sorted
	for i := 1; i < len(names); i++ {
		assert.True(t, names[i-1] < names[i], "names not sorted: %q >= %q", names[i-1], names[i])
	}

	// With vars adds $var
	regWithVars := NewFunctionRegistryWithVars(map[string]string{"K": "V"})
	namesWithVars := regWithVars.RegisteredNames()
	assert.Len(t, namesWithVars, 13)
	assert.Contains(t, namesWithVars, "$var")
}

func TestFunctionRegistry_CustomFunction(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.register("double", func(params ...any) (any, error) {
		return params[0].(int) * 2, nil
	}, new(func(int) int))

	c := NewCompiler(WithExprOptions(reg.ExprOptions()...))
	compiled, err := c.Compile("{{ double(21) }}")
	require.NoError(t, err)

	result, err := c.Evaluate(compiled, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}

func TestCoerceToInt_FromString(t *testing.T) {
	v, err := coerceToInt("42")
	require.NoError(t, err)
	assert.Equal(t, 42, v)
}

func TestCoerceToInt_FromFloat(t *testing.T) {
	v, err := coerceToInt(3.14)
	require.NoError(t, err)
	assert.Equal(t, 3, v)
}

func TestCoerceToInt_FromInt64(t *testing.T) {
	v, err := coerceToInt(int64(99))
	require.NoError(t, err)
	assert.Equal(t, 99, v)
}

func TestCoerceToInt_FromFloatString(t *testing.T) {
	v, err := coerceToInt("3.14")
	require.NoError(t, err)
	assert.Equal(t, 3, v)
}

func TestCoerceToInt_Invalid(t *testing.T) {
	_, err := coerceToInt("not_a_number")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot convert")
}

func TestCoerceToInt_UnsupportedType(t *testing.T) {
	_, err := coerceToInt([]int{1, 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestCoerceToFloat_FromString(t *testing.T) {
	v, err := coerceToFloat("3.14")
	require.NoError(t, err)
	assert.InDelta(t, 3.14, v, 0.001)
}

func TestCoerceToFloat_FromInt(t *testing.T) {
	v, err := coerceToFloat(42)
	require.NoError(t, err)
	assert.Equal(t, 42.0, v)
}

func TestCoerceToFloat_FromInt64(t *testing.T) {
	v, err := coerceToFloat(int64(99))
	require.NoError(t, err)
	assert.Equal(t, 99.0, v)
}

func TestCoerceToFloat_Invalid(t *testing.T) {
	_, err := coerceToFloat("not_a_number")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot convert")
}

func TestCoerceToFloat_UnsupportedType(t *testing.T) {
	_, err := coerceToFloat([]int{1, 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
}
