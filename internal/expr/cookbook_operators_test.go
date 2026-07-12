package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pins the expr-lang behavior the expression cookbook documents (#308):
// contains, startsWith, endsWith, matches are binary infix operators, not
// callable functions. If a case here starts compiling as a function call
// (e.g. after an expr-lang upgrade or a custom function registration), the
// cookbook needs re-checking.
func TestCookbookOperators_InfixCompiles_FunctionFormDoesNot(t *testing.T) {
	c := NewCompilerWithFunctions()

	infix := []string{
		`{{ 'abc' contains 'b' }}`,
		`{{ '/api/x' startsWith '/api' }}`,
		`{{ 'a@company.com' endsWith '@company.com' }}`,
		`{{ 'a@b.c' matches '^[^@]+@[^@]+$' }}`,
	}
	for _, src := range infix {
		_, err := c.Compile(src)
		require.NoError(t, err, "infix form must compile: %s", src)
	}

	fnForm := []string{
		`{{ contains('abc', 'b') }}`,
		`{{ startsWith('/api/x', '/api') }}`,
		`{{ endsWith('a@company.com', '@company.com') }}`,
		`{{ matches('a@b.c', '^[^@]+@[^@]+$') }}`,
	}
	for _, src := range fnForm {
		_, err := c.Compile(src)
		assert.Error(t, err, "function-call form must NOT compile (docs say infix-only): %s", src)
	}
}
