package expr

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr/vm"
)

// Evaluate runs a compiled expression against a context map and returns the result.
// For simple expressions (entire string is one {{ }}), the result type is preserved.
// For interpolated strings, all expression results are converted to strings and concatenated.
// For literals, the original string value is returned.
//
// The memory budget from the compiler config is applied to the VM. If an expression
// exceeds the budget, an error is returned instead of panicking.
func (c *Compiler) Evaluate(compiled *CompiledExpression, context map[string]any) (any, error) {
	if compiled.Parsed.IsLiteral {
		return compiled.Parsed.Raw, nil
	}

	if compiled.Parsed.IsSimple {
		result, err := c.runWithBudget(compiled.Programs[0], context)
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

		result, err := c.runWithBudget(compiled.Programs[i], context)
		if err != nil {
			return nil, fmt.Errorf("evaluation error in %q (segment %q): %w", compiled.Parsed.Raw, seg.Value, err)
		}

		fmt.Fprintf(&b, "%v", result)
	}

	return b.String(), nil
}

// runWithBudget executes a compiled program using the VM directly, applying the
// configured memory budget. When the budget is exceeded, the VM returns an error.
func (c *Compiler) runWithBudget(program *vm.Program, env map[string]any) (any, error) {
	v := vm.VM{}
	if c.opts.memoryBudget > 0 {
		v.MemoryBudget = c.opts.memoryBudget
	}
	return v.Run(program, env)
}
