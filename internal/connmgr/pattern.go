package connmgr

import (
	"log/slog"
	"strings"

	"github.com/chimpanze/noda/internal/expr"
)

// resolveChannelPattern evaluates a channel pattern using the expression engine.
// Supports {{ auth.sub }}, {{ request.params.id }}, and any valid expression.
// Falls back to literal string replacement for non-expression placeholders like :id.
func resolveChannelPattern(compiler *expr.Compiler, pattern string, params map[string]string, userID string, logger *slog.Logger) string {
	if compiler == nil {
		compiler = expr.NewCompiler()
	}
	// If pattern has no expression markers, return as-is
	if !strings.Contains(pattern, "{{") && !strings.Contains(pattern, ":") && !strings.Contains(pattern, "{") {
		return pattern
	}

	// Build expression context
	paramsAny := make(map[string]any, len(params))
	for k, v := range params {
		paramsAny[k] = v
	}
	context := map[string]any{
		"auth": map[string]any{
			"sub": userID,
		},
		"request": map[string]any{
			"params": paramsAny,
		},
	}

	// Use the expression engine for {{ }} patterns
	if strings.Contains(pattern, "{{") {
		resolver := expr.NewResolver(compiler, context)
		result, err := resolver.Resolve(pattern)
		if err == nil {
			if s, ok := result.(string); ok {
				return s
			}
		}
		// Log warning and fall through to manual replacement on error
		if err != nil && logger != nil {
			logger.Warn("channel pattern expression failed, falling back to param replacement",
				"pattern", pattern, "error", err)
		}
	}

	// Fallback: replace :param and {param} style placeholders
	result := pattern
	for k, v := range params {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
		result = strings.ReplaceAll(result, ":"+k, v)
	}
	return result
}
