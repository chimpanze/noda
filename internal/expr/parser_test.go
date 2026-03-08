package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_PureExpression(t *testing.T) {
	pe, err := Parse("{{ input.name }}")
	require.NoError(t, err)

	assert.True(t, pe.IsSimple)
	assert.False(t, pe.IsLiteral)
	assert.Len(t, pe.Segments, 1)
	assert.Equal(t, SegmentExpression, pe.Segments[0].Type)
	assert.Equal(t, "input.name", pe.Segments[0].Value)
}

func TestParse_PlainString(t *testing.T) {
	pe, err := Parse("hello world")
	require.NoError(t, err)

	assert.True(t, pe.IsLiteral)
	assert.False(t, pe.IsSimple)
	assert.Len(t, pe.Segments, 1)
	assert.Equal(t, SegmentLiteral, pe.Segments[0].Type)
	assert.Equal(t, "hello world", pe.Segments[0].Value)
}

func TestParse_InterpolatedString(t *testing.T) {
	pe, err := Parse("Hello {{ input.name }}, you have {{ len(orders) }} orders")
	require.NoError(t, err)

	assert.False(t, pe.IsLiteral)
	assert.False(t, pe.IsSimple)
	assert.Len(t, pe.Segments, 5)

	assert.Equal(t, SegmentLiteral, pe.Segments[0].Type)
	assert.Equal(t, "Hello ", pe.Segments[0].Value)

	assert.Equal(t, SegmentExpression, pe.Segments[1].Type)
	assert.Equal(t, "input.name", pe.Segments[1].Value)

	assert.Equal(t, SegmentLiteral, pe.Segments[2].Type)
	assert.Equal(t, ", you have ", pe.Segments[2].Value)

	assert.Equal(t, SegmentExpression, pe.Segments[3].Type)
	assert.Equal(t, "len(orders)", pe.Segments[3].Value)

	assert.Equal(t, SegmentLiteral, pe.Segments[4].Type)
	assert.Equal(t, " orders", pe.Segments[4].Value)
}

func TestParse_UnclosedDelimiter(t *testing.T) {
	_, err := Parse("Hello {{ input.name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unclosed")
}

func TestParse_EmptyExpression(t *testing.T) {
	_, err := Parse("Hello {{  }}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty expression")
}

func TestParse_NestedBraces(t *testing.T) {
	// Map literal inside expression
	pe, err := Parse("{{ {\"key\": input.value} }}")
	require.NoError(t, err)

	assert.True(t, pe.IsSimple)
	assert.Equal(t, "{\"key\": input.value}", pe.Segments[0].Value)
}

func TestParse_ExpressionAtEnd(t *testing.T) {
	pe, err := Parse("prefix {{ input.x }}")
	require.NoError(t, err)

	assert.Len(t, pe.Segments, 2)
	assert.Equal(t, "prefix ", pe.Segments[0].Value)
	assert.Equal(t, "input.x", pe.Segments[1].Value)
}

func TestParse_ExpressionAtStart(t *testing.T) {
	pe, err := Parse("{{ input.x }} suffix")
	require.NoError(t, err)

	assert.Len(t, pe.Segments, 2)
	assert.Equal(t, "input.x", pe.Segments[0].Value)
	assert.Equal(t, " suffix", pe.Segments[1].Value)
}

func TestParse_MultipleAdjacentExpressions(t *testing.T) {
	pe, err := Parse("{{ a }}{{ b }}")
	require.NoError(t, err)

	assert.Len(t, pe.Segments, 2)
	assert.Equal(t, SegmentExpression, pe.Segments[0].Type)
	assert.Equal(t, SegmentExpression, pe.Segments[1].Type)
}

func TestParse_StringLiteralsInExpression(t *testing.T) {
	pe, err := Parse(`{{ input.role == "admin" ? "yes" : "no" }}`)
	require.NoError(t, err)

	assert.True(t, pe.IsSimple)
	assert.Equal(t, `input.role == "admin" ? "yes" : "no"`, pe.Segments[0].Value)
}

func TestParse_RawPreserved(t *testing.T) {
	input := "{{ input.name }}"
	pe, err := Parse(input)
	require.NoError(t, err)
	assert.Equal(t, input, pe.Raw)
}
