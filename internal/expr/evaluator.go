package expr

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
)

// Evaluate runs a compiled expression against a context map and returns the result.
// For simple expressions (entire string is one {{ }}), the result type is preserved.
// For interpolated strings, all expression results are converted to strings and concatenated.
// For literals, the original string value is returned.
func Evaluate(compiled *CompiledExpression, context map[string]any) (any, error) {
	if compiled.Parsed.IsLiteral {
		return compiled.Parsed.Raw, nil
	}

	if compiled.Parsed.IsSimple {
		result, err := expr.Run(compiled.Programs[0], context)
		if err != nil {
			return nil, fmt.Errorf("evaluation error in %q: %w", compiled.Parsed.Raw, err)
		}
		return result, nil
	}

	// Interpolated string: evaluate each expression, concatenate
	var b strings.Builder
	for i, seg := range compiled.Parsed.Segments {
		if seg.Type == SegmentLiteral {
			b.WriteString(seg.Value)
			continue
		}

		result, err := expr.Run(compiled.Programs[i], context)
		if err != nil {
			return nil, fmt.Errorf("evaluation error in %q (segment %q): %w", compiled.Parsed.Raw, seg.Value, err)
		}

		b.WriteString(fmt.Sprintf("%v", result))
	}

	return b.String(), nil
}
