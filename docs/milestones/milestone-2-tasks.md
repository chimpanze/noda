# Milestone 2: Expression Engine — Task Breakdown

**Depends on:** Milestone 0 (`pkg/api/` interfaces)
**Result:** `{{ }}` expressions compile at load time and evaluate at runtime against execution contexts. String interpolation, custom functions, and static field detection all work.

---

## Task 2.1: Expression Parser

**Description:** Extract `{{ }}` delimited expressions from strings and classify them.

**Subtasks:**

- [ ] Create `internal/expr/parser.go`
- [ ] Implement `Parse(input string) (*ParsedExpression, error)`:
  - Pure expression: `"{{ input.name }}"` → single expression, no interpolation
  - Interpolated string: `"Hello {{ input.name }}, you have {{ len(orders) }} orders"` → multiple expression segments interleaved with literal text
  - Plain string: `"hello world"` → no expressions, literal value
- [ ] Define `ParsedExpression` struct:
  - `IsLiteral` — true if no `{{ }}` found
  - `IsSimple` — true if entire string is one `{{ }}` (no surrounding text)
  - `Segments` — ordered list of literal and expression segments
  - `Raw` — the original input string
- [ ] Handle edge cases:
  - Nested braces inside expressions (Expr supports map literals)
  - Unclosed `{{ ` → error with position
  - Empty `{{ }}` → error
  - Escaped braces (if needed — decide and document)

**Tests:**
- [ ] Pure expression parsed correctly
- [ ] Interpolated string with multiple expressions
- [ ] Plain string classified as literal
- [ ] Unclosed delimiter produces error
- [ ] Empty expression produces error
- [ ] Nested braces in Expr syntax (e.g., map literals) handled

**Acceptance criteria:** Any string can be parsed into a `ParsedExpression` with correct segment classification.

---

## Task 2.2: Expression Compiler

**Description:** Compile parsed expressions into executable programs using the Expr library, cached for reuse.

**Subtasks:**

- [ ] Create `internal/expr/compiler.go`
- [ ] Implement `Compile(parsed *ParsedExpression) (*CompiledExpression, error)`:
  - For each expression segment, call `expr.Compile()` from the Expr library
  - Cache the compiled programs in the `CompiledExpression` struct
  - Compilation validates syntax — invalid Expr syntax produces a clear error
- [ ] `CompiledExpression` stores:
  - The original `ParsedExpression`
  - Compiled Expr programs for each expression segment
  - Whether the result should be string-interpolated or returned as-is
- [ ] Implement batch compilation: `CompileAll(expressions map[string]string) (map[string]*CompiledExpression, []error)`
  - Compiles all expressions in a config map, collecting all errors
  - Used at startup to pre-compile all workflow expressions

**Tests:**
- [ ] Valid expression compiles successfully
- [ ] Invalid Expr syntax produces error with expression text
- [ ] Batch compilation collects multiple errors
- [ ] Compiled expression retains original text for debugging
- [ ] Compilation is idempotent (same input → same output)

**Acceptance criteria:** Expressions compile at startup. Invalid syntax is caught early with clear errors.

---

## Task 2.3: Expression Evaluator

**Description:** Evaluate compiled expressions against a runtime context.

**Subtasks:**

- [ ] Create `internal/expr/evaluator.go`
- [ ] Implement `Evaluate(compiled *CompiledExpression, context map[string]any) (any, error)`:
  - For simple expressions: evaluate the single program, return the result as-is (preserving type — int, bool, string, map, array)
  - For interpolated strings: evaluate each expression segment, convert results to strings, concatenate with literal segments, return a string
  - For literals: return the original value
- [ ] Context map contains: `input`, `auth`, `trigger`, and all node output keys
- [ ] Handle evaluation errors:
  - Undefined variable reference → clear error with variable name
  - Type mismatch (e.g., adding string to int) → Expr library error, wrapped with context
  - Nil access (e.g., `input.user.name` when `user` is nil) → clear error

**Tests:**
- [ ] Simple path access: `{{ input.name }}` → `"Alice"`
- [ ] Nested path: `{{ input.user.address.city }}` → `"Berlin"`
- [ ] Arithmetic: `{{ input.page * input.limit }}` → `40`
- [ ] Comparison: `{{ input.role == "admin" }}` → `true`
- [ ] Ternary: `{{ input.role == "admin" ? "full" : "limited" }}` → `"limited"`
- [ ] String interpolation: `"Hello {{ input.name }}"` → `"Hello Alice"`
- [ ] Multiple interpolations in one string
- [ ] Array access: `{{ items[0].name }}`
- [ ] `len()` function: `{{ len(items) }}` → `3`
- [ ] Undefined variable produces clear error
- [ ] Nil nested access produces clear error
- [ ] Boolean results preserved as bool (not stringified)
- [ ] Integer results preserved as int

**Acceptance criteria:** Expressions evaluate correctly with proper type preservation and clear error messages.

---

## Task 2.4: Custom Function Registration

**Description:** Register Noda-specific functions that are available in all expressions.

**Subtasks:**

- [ ] Create `internal/expr/functions.go`
- [ ] Implement a function registry that passes functions to `expr.Compile()` as Expr environment options
- [ ] Register built-in functions:
  - `$uuid()` → generates UUID v4 string
  - `now()` → returns current timestamp (time.Time)
  - `len(array)` → built-in to Expr, verify it works
  - `lower(string)` → lowercase
  - `upper(string)` → uppercase
- [ ] Functions are registered at compile time — the compiler receives the function registry and includes functions in the Expr environment
- [ ] Plugin-registered functions: define a `RegisterFunction(name string, fn any)` interface for plugins to add custom functions. Implementation in later milestones, but the hook exists now.

**Tests:**
- [ ] `{{ $uuid() }}` produces a valid UUID
- [ ] `{{ lower("HELLO") }}` → `"hello"`
- [ ] `{{ upper("hello") }}` → `"HELLO"`
- [ ] `{{ now() }}` returns a time value
- [ ] `{{ len(items) }}` works with arrays
- [ ] Unknown function produces compile-time error

**Acceptance criteria:** All built-in functions work. Plugin function registration hook exists.

---

## Task 2.5: Static Field Detection

**Description:** Detect whether a config value is a static literal or contains expressions. Used to validate fields that must be static (e.g., `mode`, `cases`, `workflow`).

**Subtasks:**

- [ ] Create `internal/expr/static.go`
- [ ] Implement `IsStatic(value string) bool` — returns true if the string contains no `{{ }}` delimiters
- [ ] Implement `ValidateStaticFields(config map[string]any, staticFields []string) []error`:
  - Given a node's config and a list of field names that must be static
  - Check each field — if it contains `{{ }}`, produce an error
  - Error message: `"field 'mode' must be a static value, not an expression"`
- [ ] Used during startup validation when loading workflow nodes

**Tests:**
- [ ] Static string detected as static
- [ ] Expression string detected as non-static
- [ ] Mixed static and expression fields — only expression fields error
- [ ] Nested fields checked (e.g., `match.type`)

**Acceptance criteria:** Static-only fields are enforced at startup.

---

## Task 2.6: Resolve Convenience Method

**Description:** Implement the `Resolve()` method that nodes call at runtime — the bridge between raw config and evaluated values.

**Subtasks:**

- [ ] Create `internal/expr/resolver.go`
- [ ] Implement `Resolver` struct that holds a cache of compiled expressions and the current execution context
- [ ] `Resolve(expression string) (any, error)`:
  - Look up the pre-compiled expression (compiled at startup)
  - Evaluate against the current context
  - Return the result
- [ ] `ResolveMap(config map[string]any) (map[string]any, error)`:
  - Recursively walk a config map, resolving all string values that are expressions
  - Non-string values pass through unchanged
  - Nested maps and arrays are walked recursively
- [ ] This is what `nCtx.Resolve()` calls internally (the ExecutionContext implementation will use this)

**Tests:**
- [ ] Resolve single expression returns correct value
- [ ] ResolveMap resolves all expressions in nested config
- [ ] Non-expression strings pass through unchanged
- [ ] Non-string values (ints, bools, arrays) pass through unchanged
- [ ] Missing context variable produces clear error
- [ ] Pre-compiled cache hit (no re-compilation at runtime)

**Acceptance criteria:** Nodes can call `Resolve()` to lazily evaluate any expression against the current context.

---

## Task 2.7: Integration Tests

**Description:** End-to-end tests of the full expression pipeline.

**Subtasks:**

- [ ] Test: compile expressions from a sample workflow config, evaluate against a mock context, verify all results
- [ ] Test: string interpolation with multiple expressions in route trigger mapping
- [ ] Test: custom functions in expressions
- [ ] Test: static field validation catches expressions in `mode` and `cases` fields
- [ ] Test: compile-time errors are collected when loading a workflow with invalid expressions
- [ ] Test: runtime evaluation error includes the original expression text and the context path

**Acceptance criteria:** Full compile → evaluate pipeline works end-to-end with realistic configs.
