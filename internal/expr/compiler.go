package expr

import (
	"fmt"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// CompiledExpression holds a parsed expression with compiled Expr programs.
type CompiledExpression struct {
	Parsed   *ParsedExpression
	Programs []*vm.Program // one per expression segment (nil for literal segments)
}

// CompilerOption configures the expression compiler.
type CompilerOption func(*compilerConfig)

type compilerConfig struct {
	exprOptions  []expr.Option
	maxCacheSize int  // 0 means unlimited
	memoryBudget uint // 0 means use expr default (1M)
}

// WithMemoryBudget sets the memory budget for expression evaluation.
// The VM tracks allocations for arrays, maps, and ranges.
// When the budget is exceeded, evaluation returns an error.
// 0 means use the expr default (1M allocation units).
func WithMemoryBudget(n uint) CompilerOption {
	return func(c *compilerConfig) {
		c.memoryBudget = n
	}
}

// WithExprOptions adds expr.Option values to be used during compilation.
func WithExprOptions(opts ...expr.Option) CompilerOption {
	return func(c *compilerConfig) {
		c.exprOptions = append(c.exprOptions, opts...)
	}
}

// WithMaxCacheSize sets the maximum number of compiled expressions to cache.
// When the limit is reached, the cache is cleared. 0 means unlimited.
func WithMaxCacheSize(size int) CompilerOption {
	return func(c *compilerConfig) {
		c.maxCacheSize = size
	}
}

// Compiler compiles and caches expressions.
type Compiler struct {
	mu    sync.RWMutex
	cache map[string]*CompiledExpression
	order []string // insertion order for LRU eviction
	opts  compilerConfig
}

// NewCompiler creates a new expression compiler with the given options.
func NewCompiler(opts ...CompilerOption) *Compiler {
	c := &Compiler{
		cache: make(map[string]*CompiledExpression),
	}
	for _, opt := range opts {
		opt(&c.opts)
	}
	return c
}

// Compile parses and compiles a single expression string.
func (c *Compiler) Compile(input string) (*CompiledExpression, error) {
	c.mu.RLock()
	if cached, ok := c.cache[input]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	parsed, err := Parse(input)
	if err != nil {
		return nil, err
	}

	compiled := &CompiledExpression{
		Parsed:   parsed,
		Programs: make([]*vm.Program, len(parsed.Segments)),
	}

	if parsed.IsLiteral {
		c.mu.Lock()
		c.addToCache(input, compiled)
		c.mu.Unlock()
		return compiled, nil
	}

	// Build expr options: allow undefined variables for flexible context
	opts := []expr.Option{
		expr.AllowUndefinedVariables(),
	}
	opts = append(opts, c.opts.exprOptions...)

	for i, seg := range parsed.Segments {
		if seg.Type != SegmentExpression {
			continue
		}

		program, err := expr.Compile(seg.Value, opts...)
		if err != nil {
			return nil, fmt.Errorf("compile error in expression %q: %w", seg.Value, err)
		}
		compiled.Programs[i] = program
	}

	c.mu.Lock()
	c.addToCache(input, compiled)
	c.mu.Unlock()

	return compiled, nil
}

// addToCache adds a compiled expression to the cache, evicting the oldest 25%
// of entries when the limit is reached. Must be called with c.mu held.
func (c *Compiler) addToCache(key string, compiled *CompiledExpression) {
	if c.opts.maxCacheSize > 0 && len(c.cache) >= c.opts.maxCacheSize {
		evictCount := c.opts.maxCacheSize / 4
		if evictCount < 1 {
			evictCount = 1
		}
		for i := 0; i < evictCount && i < len(c.order); i++ {
			delete(c.cache, c.order[i])
		}
		c.order = c.order[evictCount:]
	}
	if _, exists := c.cache[key]; !exists {
		c.order = append(c.order, key)
	}
	c.cache[key] = compiled
}

// CompileAll compiles all expressions in a string map, collecting all errors.
func (c *Compiler) CompileAll(expressions map[string]string) (map[string]*CompiledExpression, []error) {
	result := make(map[string]*CompiledExpression, len(expressions))
	var errs []error

	for key, exprStr := range expressions {
		compiled, err := c.Compile(exprStr)
		if err != nil {
			errs = append(errs, fmt.Errorf("field %q: %w", key, err))
		} else {
			result[key] = compiled
		}
	}

	return result, errs
}
