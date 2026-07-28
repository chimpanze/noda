# Value and Type Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the closed `Value` type, the `Type` language, and the normative conversion table specified in §3 of `docs/superpowers/specs/2026-07-28-runtime-foundations-design.md` — stage 1 of §13.

**Architecture:** Two Go packages with a one-way dependency. `internal/value` defines a closed union of seven value kinds, with numbers that preserve their source literal and maps that iterate deterministically; `internal/types` defines the type language and gradual assignability, and imports `value` to check a value against a type. Both are pure — no I/O, no globals, no dependencies outside the standard library.

**Tech Stack:** Go 1.26, standard library only (`encoding/json` token decoding, `math/big` for exact numeric comparison, `net/url` deliberately *not* used here — see Global Constraints).

## Global Constraints

- **Module path:** `github.com/chimpanze/runtime`. Chosen provisionally. If you pick a different name, change it in `go.mod` and in every import in this plan before starting Task 1.
- **Go version:** `go 1.26.0` in `go.mod`. Matches the toolchain the reference implementation uses.
- **Zero third-party dependencies in these two packages.** Standard library only. A dependency here would propagate to every consumer of the runtime.
- **No `any` in exported signatures** except `ToSQLArg`, whose return type is dictated by `database/sql`. This is the whole point of the layer — see spec §3.
- **Immutability is enforced by construction, not documentation.** Every composite type has unexported fields, no mutating methods, and constructors that copy their input. Spec §5.1 rule 3 and §5.7 depend on this.
- **Deterministic iteration means insertion order**, not sorted order. Sorted output is a diagnostics concern (spec §8.2), not a value-model one.
- **Every test must be run and observed to fail before its implementation is written.** A test that has never failed is not known to test anything. The reference implementation shipped several assertions that passed with the code deleted.

---

## File Structure

```
go.mod
internal/value/
  value.go      — the Value union: Kind, constructors, accessors
  number.go     — Number: a JSON number that preserves its literal
  list.go       — List: immutable ordered sequence
  map.go        — Map + MapBuilder: immutable insertion-ordered string map
  equal.go      — structural equality across all kinds
  json.go       — JSON decode/encode preserving literals and key order
  convert.go    — the §3.3 conversion table
internal/types/
  type.go       — the Type language and its rendering
  assignable.go — gradual assignability
  check.go      — does a Value satisfy a Type
```

`internal/types` imports `internal/value`. Nothing imports `internal/types` yet.

---

### Task 1: Module skeleton and the `Number` type

The literal-preserving number is the single highest-value piece in this layer: it is what makes `12345678901234567890` survive from source to output instead of becoming `1.2345678901234567e+19`. Spec §3.1.

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `internal/value/number.go`
- Test: `internal/value/number_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Number struct{ ... }` (unexported fields)
  - `func ParseNumber(lit string) (Number, error)`
  - `func NumberFromInt(i int64) Number`
  - `func NumberFromFloat(f float64) Number`
  - `func (n Number) Literal() string`
  - `func (n Number) Int64() (int64, error)`
  - `func (n Number) Float64() (f float64, exact bool)`
  - `func (n Number) IsInteger() bool`
  - `func (n Number) Cmp(o Number) int`
  - `func (n Number) NumberEqual(o Number) bool`

- [ ] **Step 1: Create the module skeleton**

```bash
mkdir -p internal/value internal/types
cat > go.mod <<'EOF'
module github.com/chimpanze/runtime

go 1.26.0
EOF
cat > .gitignore <<'EOF'
/bin/
*.test
coverage.out
EOF
```

- [ ] **Step 2: Write the failing test**

Create `internal/value/number_test.go`:

```go
package value

import "testing"

func TestParseNumber_PreservesLiteralExactly(t *testing.T) {
	// The whole reason this type exists: a float64 round-trip would render
	// this as 1.2345678901234567e+19.
	const lit = "12345678901234567890"
	n, err := ParseNumber(lit)
	if err != nil {
		t.Fatalf("ParseNumber(%q) returned error: %v", lit, err)
	}
	if got := n.Literal(); got != lit {
		t.Errorf("Literal() = %q, want %q", got, lit)
	}
}

func TestParseNumber_AcceptsJSONGrammar(t *testing.T) {
	valid := []string{
		"0", "-0", "1", "-1", "42",
		"1.5", "-1.5", "0.0000001",
		"1e10", "1E10", "1e+10", "1e-10", "1.5e-7",
		"12345678901234567890", "-12345678901234567890",
	}
	for _, lit := range valid {
		if _, err := ParseNumber(lit); err != nil {
			t.Errorf("ParseNumber(%q) = error %v, want success", lit, err)
		}
	}
}

func TestParseNumber_RejectsNonJSONGrammar(t *testing.T) {
	invalid := []string{
		"", " ", "+1", "01", "-01", ".5", "1.", "1e", "1e+", "0x10",
		"Infinity", "NaN", "1 ", " 1", "--1", "1.2.3",
	}
	for _, lit := range invalid {
		if _, err := ParseNumber(lit); err == nil {
			t.Errorf("ParseNumber(%q) = nil error, want rejection", lit)
		}
	}
}

func TestNumber_Int64(t *testing.T) {
	tests := []struct {
		lit     string
		want    int64
		wantErr bool
	}{
		{lit: "0", want: 0},
		{lit: "42", want: 42},
		{lit: "-42", want: -42},
		{lit: "9223372036854775807", want: 9223372036854775807},
		{lit: "-9223372036854775808", want: -9223372036854775808},
		{lit: "1e3", want: 1000},
		{lit: "1.0", want: 1},
		{lit: "1.5", wantErr: true},                    // not integral
		{lit: "9223372036854775808", wantErr: true},    // overflows int64
		{lit: "12345678901234567890", wantErr: true},   // overflows int64
	}
	for _, tc := range tests {
		n, err := ParseNumber(tc.lit)
		if err != nil {
			t.Fatalf("ParseNumber(%q): %v", tc.lit, err)
		}
		got, err := n.Int64()
		if tc.wantErr {
			if err == nil {
				t.Errorf("Int64() for %q = %d, nil; want error", tc.lit, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Int64() for %q returned error: %v", tc.lit, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Int64() for %q = %d, want %d", tc.lit, got, tc.want)
		}
	}
}

func TestNumber_Float64_ReportsInexactness(t *testing.T) {
	// This is the property that makes precision loss visible instead of silent.
	exactCases := []string{"0", "1", "-1", "1.5", "0.25", "1e10"}
	for _, lit := range exactCases {
		n, err := ParseNumber(lit)
		if err != nil {
			t.Fatalf("ParseNumber(%q): %v", lit, err)
		}
		if _, exact := n.Float64(); !exact {
			t.Errorf("Float64() for %q reported inexact, want exact", lit)
		}
	}

	n, err := ParseNumber("12345678901234567890")
	if err != nil {
		t.Fatalf("ParseNumber: %v", err)
	}
	f, exact := n.Float64()
	if exact {
		t.Errorf("Float64() for 12345678901234567890 reported exact (got %v), want inexact", f)
	}
}

func TestNumber_IsInteger(t *testing.T) {
	tests := map[string]bool{
		"0": true, "42": true, "-42": true, "1e3": true, "1.0": true,
		"1.5": false, "0.0000001": false, "1e-3": false,
	}
	for lit, want := range tests {
		n, err := ParseNumber(lit)
		if err != nil {
			t.Fatalf("ParseNumber(%q): %v", lit, err)
		}
		if got := n.IsInteger(); got != want {
			t.Errorf("IsInteger() for %q = %v, want %v", lit, got, want)
		}
	}
}

func TestNumber_EqualityIsNumericNotTextual(t *testing.T) {
	// 1, 1.0 and 1e0 are three literals for one value. Go's == on the struct
	// would call them different; NumberEqual must not.
	a, _ := ParseNumber("1")
	b, _ := ParseNumber("1.0")
	c, _ := ParseNumber("1e0")
	d, _ := ParseNumber("2")

	if !a.NumberEqual(b) {
		t.Error("1 != 1.0, want equal")
	}
	if !a.NumberEqual(c) {
		t.Error("1 != 1e0, want equal")
	}
	if a.NumberEqual(d) {
		t.Error("1 == 2, want unequal")
	}
	if a.Cmp(d) >= 0 {
		t.Errorf("Cmp(1, 2) = %d, want negative", a.Cmp(d))
	}
	if d.Cmp(a) <= 0 {
		t.Errorf("Cmp(2, 1) = %d, want positive", d.Cmp(a))
	}
}

func TestNumberFromFloat_ProducesNonScientificLiteral(t *testing.T) {
	// The defect this layer exists to prevent.
	n := NumberFromFloat(1e21)
	if got := n.Literal(); got != "1000000000000000000000" {
		t.Errorf("NumberFromFloat(1e21).Literal() = %q, want %q", got, "1000000000000000000000")
	}
	n = NumberFromFloat(0.0000001)
	if got := n.Literal(); got != "0.0000001" {
		t.Errorf("NumberFromFloat(0.0000001).Literal() = %q, want %q", got, "0.0000001")
	}
}

func TestNumberFromInt(t *testing.T) {
	n := NumberFromInt(-42)
	if got := n.Literal(); got != "-42" {
		t.Errorf("Literal() = %q, want %q", got, "-42")
	}
	got, err := n.Int64()
	if err != nil {
		t.Fatalf("Int64(): %v", err)
	}
	if got != -42 {
		t.Errorf("Int64() = %d, want -42", got)
	}
}
```

- [ ] **Step 3: Run the test and verify it fails**

Run: `go test ./internal/value/ -run TestParseNumber -v`
Expected: FAIL — `undefined: ParseNumber`

- [ ] **Step 4: Write the implementation**

Create `internal/value/number.go`:

```go
package value

import (
	"fmt"
	"math/big"
	"strconv"
)

// Number is a JSON number that preserves its source literal exactly.
//
// Every number in a configuration file arrives as text. Decoding it into a
// float64 loses information before any validation runs: 12345678901234567890
// becomes 1.2345678901234567e+19, and every later stage that stringifies it
// emits scientific notation. Number keeps the literal and derives numeric
// views on demand, so no stage can lose precision the source did not.
//
// The zero Number is not valid; construct one with ParseNumber, NumberFromInt
// or NumberFromFloat.
//
// Number is comparable with == only by literal, which is not the semantics you
// want: 1 and 1.0 are one value written two ways. Use NumberEqual or Cmp.
type Number struct {
	lit string
}

// ParseNumber validates lit against the JSON number grammar and returns it
// unchanged. The grammar is deliberately strict — no leading +, no leading
// zeros, no bare ".5" or "1." — so that Literal() round-trips into valid JSON.
func ParseNumber(lit string) (Number, error) {
	if !validJSONNumber(lit) {
		return Number{}, fmt.Errorf("invalid JSON number literal %q", lit)
	}
	return Number{lit: lit}, nil
}

// NumberFromInt returns the exact decimal literal for i.
func NumberFromInt(i int64) Number {
	return Number{lit: strconv.FormatInt(i, 10)}
}

// NumberFromFloat returns a non-scientific decimal literal for f.
//
// Format 'f' is used rather than 'g' precisely to avoid scientific notation:
// a header or query parameter carrying 1e21 must read 1000000000000000000000,
// not 1e+21. Very small magnitudes produce long literals, which is verbose but
// exact — correctness is preferred over brevity here.
//
// NaN and ±Inf have no JSON representation; they render as "0". Callers that
// can produce them should reject them before conversion.
func NumberFromFloat(f float64) Number {
	if f != f || f > 1.7976931348623157e308 || f < -1.7976931348623157e308 {
		return Number{lit: "0"}
	}
	return Number{lit: strconv.FormatFloat(f, 'f', -1, 64)}
}

// Literal returns the exact source text of the number.
func (n Number) Literal() string { return n.lit }

// rat returns the exact rational value. Every valid JSON number is exactly
// representable as a big.Rat, so this never loses information.
func (n Number) rat() *big.Rat {
	r, ok := new(big.Rat).SetString(n.lit)
	if !ok {
		// Unreachable for a Number built through the constructors, all of
		// which validate. Returning zero keeps callers total.
		return new(big.Rat)
	}
	return r
}

// IsInteger reports whether the number has no fractional part. 1, 1.0 and 1e3
// are all integers; 1.5 and 1e-3 are not.
func (n Number) IsInteger() bool {
	return n.rat().IsInt()
}

// Int64 returns the number as an int64. It returns an error if the number has
// a fractional part or does not fit in an int64 — never a silently truncated
// or saturated value.
func (n Number) Int64() (int64, error) {
	r := n.rat()
	if !r.IsInt() {
		return 0, fmt.Errorf("number %s is not an integer", n.lit)
	}
	i := r.Num()
	if !i.IsInt64() {
		return 0, fmt.Errorf("number %s does not fit in int64", n.lit)
	}
	return i.Int64(), nil
}

// Float64 returns the number as a float64 and reports whether the conversion
// was exact. An inexact result is still returned — the caller decides whether
// approximation is acceptable — but it is never silent.
func (n Number) Float64() (float64, bool) {
	f, exact := n.rat().Float64()
	return f, exact
}

// Cmp compares two numbers by value, returning -1, 0 or +1.
func (n Number) Cmp(o Number) int {
	return n.rat().Cmp(o.rat())
}

// NumberEqual reports whether two numbers have the same value, regardless of
// how each was written.
func (n Number) NumberEqual(o Number) bool {
	return n.Cmp(o) == 0
}

// validJSONNumber implements the number production from RFC 8259 section 6:
//
//	number = [ "-" ] int [ frac ] [ exp ]
//	int    = "0" / ( digit1-9 *DIGIT )
//	frac   = "." 1*DIGIT
//	exp    = ("e" / "E") [ "-" / "+" ] 1*DIGIT
func validJSONNumber(s string) bool {
	i := 0
	n := len(s)

	if i < n && s[i] == '-' {
		i++
	}

	// int
	if i >= n {
		return false
	}
	if s[i] == '0' {
		i++
	} else if s[i] >= '1' && s[i] <= '9' {
		for i < n && isDigit(s[i]) {
			i++
		}
	} else {
		return false
	}

	// frac
	if i < n && s[i] == '.' {
		i++
		if i >= n || !isDigit(s[i]) {
			return false
		}
		for i < n && isDigit(s[i]) {
			i++
		}
	}

	// exp
	if i < n && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < n && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if i >= n || !isDigit(s[i]) {
			return false
		}
		for i < n && isDigit(s[i]) {
			i++
		}
	}

	return i == n
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
```

- [ ] **Step 5: Run the tests and verify they pass**

Run: `go test ./internal/value/ -v`
Expected: PASS, all nine test functions.

- [ ] **Step 6: Verify the tests have teeth**

Change `NumberFromFloat` to use `'g'` instead of `'f'` and re-run.
Expected: `TestNumberFromFloat_ProducesNonScientificLiteral` FAILS with `"1e+21"`.
Then revert the change and confirm the suite is green again.

This step is not optional. A test that has never failed is not known to test anything.

- [ ] **Step 7: Commit**

```bash
git add go.mod .gitignore internal/value/number.go internal/value/number_test.go
git commit -m "feat(value): literal-preserving Number type

A JSON number decoded into float64 is lossy before validation runs.
Number keeps the source literal and derives int64/float64/rational views
on demand, reporting inexactness rather than losing it silently."
```

---

### Task 2: `List` and `Map` with deterministic iteration

Spec §3.2. Go's randomized map iteration leaked into error text, cycle reports and **entry-node dispatch order** in the reference implementation — three separate defects. An insertion-ordered map type makes all three unwritable.

**Files:**
- Create: `internal/value/list.go`
- Create: `internal/value/map.go`
- Test: `internal/value/list_test.go`
- Test: `internal/value/map_test.go`

**Interfaces:**
- Consumes: `Value` is referenced but not yet defined — Task 3 defines it. Write these files against the `Value` type name; the package will not compile until Task 3 lands, so Task 2 and Task 3 share a single verification point (Task 3 Step 5). Commit Task 2 anyway: the code is complete and reviewable on its own.
- Produces:
  - `type List struct{ ... }`, `func NewList(items ...Value) List`, `func (l List) Len() int`, `func (l List) At(i int) Value`, `func (l List) Items() []Value`
  - `type Map struct{ ... }`, `type MapBuilder struct{ ... }`
  - `func NewMapBuilder() *MapBuilder`, `func (b *MapBuilder) Set(key string, v Value) *MapBuilder`, `func (b *MapBuilder) Build() Map`
  - `func (m Map) Len() int`, `func (m Map) Get(key string) (Value, bool)`, `func (m Map) Keys() []string`, `func (m Map) Has(key string) bool`

- [ ] **Step 1: Write the failing tests**

Create `internal/value/list_test.go`:

```go
package value

import "testing"

func TestList_PreservesOrder(t *testing.T) {
	l := NewList(OfString("a"), OfString("b"), OfString("c"))
	if l.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", l.Len())
	}
	want := []string{"a", "b", "c"}
	for i, w := range want {
		s, ok := l.At(i).Str()
		if !ok {
			t.Fatalf("At(%d) is not a string", i)
		}
		if s != w {
			t.Errorf("At(%d) = %q, want %q", i, s, w)
		}
	}
}

func TestList_IsImmutable(t *testing.T) {
	src := []Value{OfString("a"), OfString("b")}
	l := NewList(src...)

	// Mutating the caller's slice must not affect the List.
	src[0] = OfString("MUTATED")
	if s, _ := l.At(0).Str(); s != "a" {
		t.Errorf("List observed a mutation of its constructor input: At(0) = %q", s)
	}

	// Mutating the returned slice must not affect the List either.
	items := l.Items()
	items[1] = OfString("MUTATED")
	if s, _ := l.At(1).Str(); s != "b" {
		t.Errorf("List observed a mutation of Items(): At(1) = %q", s)
	}
}

func TestList_ZeroValueIsEmpty(t *testing.T) {
	var l List
	if l.Len() != 0 {
		t.Errorf("zero List Len() = %d, want 0", l.Len())
	}
	if got := l.Items(); len(got) != 0 {
		t.Errorf("zero List Items() = %v, want empty", got)
	}
}
```

Create `internal/value/map_test.go`:

```go
package value

import "testing"

func TestMap_IteratesInInsertionOrder(t *testing.T) {
	// Go's built-in map randomizes iteration per process. Insertion order must
	// be stable across every construction of the same map.
	build := func() Map {
		return NewMapBuilder().
			Set("zebra", OfString("1")).
			Set("apple", OfString("2")).
			Set("mango", OfString("3")).
			Build()
	}
	want := []string{"zebra", "apple", "mango"}
	for run := 0; run < 100; run++ {
		got := build().Keys()
		if len(got) != len(want) {
			t.Fatalf("run %d: Keys() = %v, want %v", run, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("run %d: Keys() = %v, want %v", run, got, want)
			}
		}
	}
}

func TestMap_SetExistingKeyKeepsOriginalPosition(t *testing.T) {
	m := NewMapBuilder().
		Set("a", OfString("1")).
		Set("b", OfString("2")).
		Set("a", OfString("overwritten")).
		Build()

	want := []string{"a", "b"}
	got := m.Keys()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
	v, ok := m.Get("a")
	if !ok {
		t.Fatal("Get(\"a\") not found")
	}
	if s, _ := v.Str(); s != "overwritten" {
		t.Errorf("Get(\"a\") = %q, want %q", s, "overwritten")
	}
}

func TestMap_GetAndHas(t *testing.T) {
	m := NewMapBuilder().Set("present", OfString("yes")).Build()

	if !m.Has("present") {
		t.Error("Has(\"present\") = false, want true")
	}
	if m.Has("absent") {
		t.Error("Has(\"absent\") = true, want false")
	}
	if _, ok := m.Get("absent"); ok {
		t.Error("Get(\"absent\") reported found, want not found")
	}
	if m.Len() != 1 {
		t.Errorf("Len() = %d, want 1", m.Len())
	}
}

func TestMap_IsImmutable(t *testing.T) {
	b := NewMapBuilder().Set("a", OfString("1"))
	m := b.Build()

	// Continuing to use the builder must not affect the built Map.
	b.Set("b", OfString("2"))
	if m.Len() != 1 {
		t.Errorf("Map observed a post-Build() builder mutation: Len() = %d, want 1", m.Len())
	}

	// Mutating the returned key slice must not affect the Map.
	keys := m.Keys()
	keys[0] = "MUTATED"
	if got := m.Keys(); got[0] != "a" {
		t.Errorf("Map observed a mutation of Keys(): %v", got)
	}
}

func TestMap_ZeroValueIsEmpty(t *testing.T) {
	var m Map
	if m.Len() != 0 {
		t.Errorf("zero Map Len() = %d, want 0", m.Len())
	}
	if _, ok := m.Get("anything"); ok {
		t.Error("zero Map Get() reported found")
	}
	if got := m.Keys(); len(got) != 0 {
		t.Errorf("zero Map Keys() = %v, want empty", got)
	}
}
```

- [ ] **Step 2: Write the implementations**

Create `internal/value/list.go`:

```go
package value

// List is an immutable ordered sequence of values.
//
// The zero List is a valid empty list.
type List struct {
	items []Value
}

// NewList returns a List holding a copy of items. The copy is what makes the
// List immutable: the caller may keep and mutate its own slice.
func NewList(items ...Value) List {
	if len(items) == 0 {
		return List{}
	}
	cp := make([]Value, len(items))
	copy(cp, items)
	return List{items: cp}
}

// Len returns the number of items.
func (l List) Len() int { return len(l.items) }

// At returns the item at index i. It panics if i is out of range, matching
// slice indexing — callers iterate with Len.
func (l List) At(i int) Value { return l.items[i] }

// Items returns a copy of the items. Prefer Len and At in hot paths; Items
// exists for callers that need a slice, and copies so they cannot reach in.
func (l List) Items() []Value {
	cp := make([]Value, len(l.items))
	copy(cp, l.items)
	return cp
}

// ListBuilder accumulates items for a List. Use it when the item count is not
// known up front, such as while decoding.
type ListBuilder struct {
	items []Value
}

// NewListBuilder returns an empty builder.
func NewListBuilder() *ListBuilder { return &ListBuilder{} }

// Append adds v and returns the builder for chaining.
func (b *ListBuilder) Append(v Value) *ListBuilder {
	b.items = append(b.items, v)
	return b
}

// Build returns an immutable List. The builder may continue to be used
// afterwards without affecting the returned List.
func (b *ListBuilder) Build() List { return NewList(b.items...) }
```

Create `internal/value/map.go`:

```go
package value

// Map is an immutable, string-keyed map that iterates in insertion order.
//
// Insertion order — not sorted order — is the semantic: it preserves the order
// the author wrote in the source document, which matters for round-tripping and
// for presenting fields in an editor. Sorted output is a diagnostics concern.
//
// The reason this type exists rather than a built-in map: Go randomizes map
// iteration per process, and in the reference implementation that randomness
// reached error message text, cycle reports, and the order in which a workflow's
// entry nodes were dispatched. Determinism belongs in the type, not in a
// convention that each site must remember.
//
// The zero Map is a valid empty map.
type Map struct {
	keys []string
	vals map[string]Value
}

// Len returns the number of entries.
func (m Map) Len() int { return len(m.keys) }

// Get returns the value for key and whether it was present.
func (m Map) Get(key string) (Value, bool) {
	v, ok := m.vals[key]
	return v, ok
}

// Has reports whether key is present.
func (m Map) Has(key string) bool {
	_, ok := m.vals[key]
	return ok
}

// Keys returns a copy of the keys in insertion order. Iterate a Map by ranging
// this and calling Get.
func (m Map) Keys() []string {
	cp := make([]string, len(m.keys))
	copy(cp, m.keys)
	return cp
}

// MapBuilder accumulates entries for a Map.
type MapBuilder struct {
	keys []string
	vals map[string]Value
}

// NewMapBuilder returns an empty builder.
func NewMapBuilder() *MapBuilder {
	return &MapBuilder{vals: make(map[string]Value)}
}

// Set adds or replaces key. Replacing an existing key updates its value and
// leaves its original position unchanged, so order reflects first insertion.
// Returns the builder for chaining.
func (b *MapBuilder) Set(key string, v Value) *MapBuilder {
	if _, exists := b.vals[key]; !exists {
		b.keys = append(b.keys, key)
	}
	b.vals[key] = v
	return b
}

// Build returns an immutable Map. The builder may continue to be used
// afterwards without affecting the returned Map.
func (b *MapBuilder) Build() Map {
	keys := make([]string, len(b.keys))
	copy(keys, b.keys)
	vals := make(map[string]Value, len(b.vals))
	for k, v := range b.vals {
		vals[k] = v
	}
	return Map{keys: keys, vals: vals}
}
```

- [ ] **Step 3: Confirm the package does not yet compile**

Run: `go build ./internal/value/`
Expected: FAIL — `undefined: Value`, `undefined: OfString`.

This is expected. `List` and `Map` hold `Value`s, which Task 3 defines. The two tasks share one verification point at Task 3 Step 5.

- [ ] **Step 4: Commit**

```bash
git add internal/value/list.go internal/value/map.go internal/value/list_test.go internal/value/map_test.go
git commit -m "feat(value): immutable List and insertion-ordered Map

Go randomizes map iteration per process; in the reference implementation
that reached error text, cycle reports and entry-node dispatch order.
Determinism belongs in the type rather than in a convention every call
site must remember.

Does not compile standalone: both hold Value, defined in the next commit."
```

---

### Task 3: The `Value` union, constructors, accessors and equality

Spec §3. The closed union is what replaces `map[string]any` — the single change that makes every downstream stage able to know what it is holding.

**Files:**
- Create: `internal/value/value.go`
- Create: `internal/value/equal.go`
- Test: `internal/value/value_test.go`
- Test: `internal/value/equal_test.go`

**Interfaces:**
- Consumes: `Number` (Task 1), `List`, `Map` (Task 2).
- Produces:
  - `type Kind uint8` with `KindNull`, `KindBool`, `KindNumber`, `KindString`, `KindBytes`, `KindList`, `KindMap`
  - `func (k Kind) String() string`
  - `type Value struct{ ... }`
  - `func Null() Value`, `func OfBool(b bool) Value`, `func OfNumber(n Number) Value`, `func OfString(s string) Value`, `func OfBytes(b []byte) Value`, `func OfList(l List) Value`, `func OfMap(m Map) Value`
  - `func OfInt(i int64) Value`, `func OfFloat(f float64) Value`
  - `func (v Value) Kind() Kind`, `func (v Value) IsNull() bool`
  - `func (v Value) Bool() (bool, bool)`, `func (v Value) Number() (Number, bool)`, `func (v Value) Str() (string, bool)`, `func (v Value) Bytes() ([]byte, bool)`, `func (v Value) List() (List, bool)`, `func (v Value) Map() (Map, bool)`
  - `func Equal(a, b Value) bool`

- [ ] **Step 1: Write the failing tests**

Create `internal/value/value_test.go`:

```go
package value

import "testing"

func TestZeroValueIsNull(t *testing.T) {
	var v Value
	if v.Kind() != KindNull {
		t.Errorf("zero Value Kind() = %v, want KindNull", v.Kind())
	}
	if !v.IsNull() {
		t.Error("zero Value IsNull() = false, want true")
	}
}

func TestConstructorsAndAccessors(t *testing.T) {
	t.Run("bool", func(t *testing.T) {
		v := OfBool(true)
		if v.Kind() != KindBool {
			t.Fatalf("Kind() = %v, want KindBool", v.Kind())
		}
		got, ok := v.Bool()
		if !ok || got != true {
			t.Errorf("Bool() = %v, %v; want true, true", got, ok)
		}
	})

	t.Run("number", func(t *testing.T) {
		n, err := ParseNumber("42")
		if err != nil {
			t.Fatalf("ParseNumber: %v", err)
		}
		v := OfNumber(n)
		if v.Kind() != KindNumber {
			t.Fatalf("Kind() = %v, want KindNumber", v.Kind())
		}
		got, ok := v.Number()
		if !ok {
			t.Fatal("Number() reported not-a-number")
		}
		if got.Literal() != "42" {
			t.Errorf("Literal() = %q, want %q", got.Literal(), "42")
		}
	})

	t.Run("string", func(t *testing.T) {
		v := OfString("hello")
		if v.Kind() != KindString {
			t.Fatalf("Kind() = %v, want KindString", v.Kind())
		}
		got, ok := v.Str()
		if !ok || got != "hello" {
			t.Errorf("Str() = %q, %v; want %q, true", got, ok, "hello")
		}
	})

	t.Run("bytes", func(t *testing.T) {
		v := OfBytes([]byte{1, 2, 3})
		if v.Kind() != KindBytes {
			t.Fatalf("Kind() = %v, want KindBytes", v.Kind())
		}
		got, ok := v.Bytes()
		if !ok || len(got) != 3 || got[0] != 1 {
			t.Errorf("Bytes() = %v, %v; want [1 2 3], true", got, ok)
		}
	})

	t.Run("list", func(t *testing.T) {
		v := OfList(NewList(OfString("a")))
		if v.Kind() != KindList {
			t.Fatalf("Kind() = %v, want KindList", v.Kind())
		}
		got, ok := v.List()
		if !ok || got.Len() != 1 {
			t.Errorf("List() len = %d, ok = %v; want 1, true", got.Len(), ok)
		}
	})

	t.Run("map", func(t *testing.T) {
		v := OfMap(NewMapBuilder().Set("k", OfString("v")).Build())
		if v.Kind() != KindMap {
			t.Fatalf("Kind() = %v, want KindMap", v.Kind())
		}
		got, ok := v.Map()
		if !ok || got.Len() != 1 {
			t.Errorf("Map() len = %d, ok = %v; want 1, true", got.Len(), ok)
		}
	})
}

func TestAccessorsRejectWrongKind(t *testing.T) {
	v := OfString("not a number")

	if _, ok := v.Bool(); ok {
		t.Error("Bool() on a string reported ok")
	}
	if _, ok := v.Number(); ok {
		t.Error("Number() on a string reported ok")
	}
	if _, ok := v.Bytes(); ok {
		t.Error("Bytes() on a string reported ok")
	}
	if _, ok := v.List(); ok {
		t.Error("List() on a string reported ok")
	}
	if _, ok := v.Map(); ok {
		t.Error("Map() on a string reported ok")
	}
}

func TestOfBytes_CopiesInput(t *testing.T) {
	src := []byte{1, 2, 3}
	v := OfBytes(src)
	src[0] = 99
	got, _ := v.Bytes()
	if got[0] != 1 {
		t.Errorf("Value observed a mutation of the constructor input: %v", got)
	}
	got[1] = 99
	again, _ := v.Bytes()
	if again[1] != 2 {
		t.Errorf("Value observed a mutation of Bytes(): %v", again)
	}
}

func TestOfIntAndOfFloat(t *testing.T) {
	v := OfInt(-7)
	n, ok := v.Number()
	if !ok || n.Literal() != "-7" {
		t.Errorf("OfInt(-7) literal = %q, ok = %v", n.Literal(), ok)
	}

	v = OfFloat(1.5)
	n, ok = v.Number()
	if !ok || n.Literal() != "1.5" {
		t.Errorf("OfFloat(1.5) literal = %q, ok = %v", n.Literal(), ok)
	}
}

func TestKindString(t *testing.T) {
	want := map[Kind]string{
		KindNull: "null", KindBool: "bool", KindNumber: "number",
		KindString: "string", KindBytes: "bytes", KindList: "list", KindMap: "map",
	}
	for k, w := range want {
		if got := k.String(); got != w {
			t.Errorf("Kind(%d).String() = %q, want %q", k, got, w)
		}
	}
}

func TestValueUnionIsClosedAtSevenKinds(t *testing.T) {
	// Spec §3.5: a stream is a process, not a value. Adding a KindStream (or
	// any other member) would break scope immutability and free capture, on
	// which the execution model depends. This test is the guard: it fails if
	// the union grows, forcing whoever grows it to justify the change against
	// §3.5 rather than discovering the consequences later.
	//
	// KindMap must remain the highest-valued Kind.
	if KindMap != 6 {
		t.Fatalf("KindMap = %d, want 6 — the Value union has changed size", KindMap)
	}
	if got := Kind(7).String(); got != "invalid" {
		t.Errorf("Kind(7).String() = %q, want %q — an eighth kind exists", got, "invalid")
	}
}
```

Create `internal/value/equal_test.go`:

```go
package value

import "testing"

func TestEqual_Scalars(t *testing.T) {
	tests := []struct {
		name string
		a, b Value
		want bool
	}{
		{"null == null", Null(), Null(), true},
		{"null != false", Null(), OfBool(false), false},
		{"true == true", OfBool(true), OfBool(true), true},
		{"true != false", OfBool(true), OfBool(false), false},
		{"\"a\" == \"a\"", OfString("a"), OfString("a"), true},
		{"\"a\" != \"b\"", OfString("a"), OfString("b"), false},
		{"1 == 1.0", OfInt(1), OfFloat(1.0), true},
		{"1 != 2", OfInt(1), OfInt(2), false},
		{"1 != \"1\"", OfInt(1), OfString("1"), false},
	}
	for _, tc := range tests {
		if got := Equal(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: Equal = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestEqual_Bytes(t *testing.T) {
	if !Equal(OfBytes([]byte{1, 2}), OfBytes([]byte{1, 2})) {
		t.Error("equal byte slices reported unequal")
	}
	if Equal(OfBytes([]byte{1, 2}), OfBytes([]byte{1, 3})) {
		t.Error("differing byte slices reported equal")
	}
	if Equal(OfBytes([]byte{1}), OfBytes([]byte{1, 2})) {
		t.Error("different-length byte slices reported equal")
	}
}

func TestEqual_ListsAreOrderSensitive(t *testing.T) {
	ab := OfList(NewList(OfString("a"), OfString("b")))
	ab2 := OfList(NewList(OfString("a"), OfString("b")))
	ba := OfList(NewList(OfString("b"), OfString("a")))
	a := OfList(NewList(OfString("a")))

	if !Equal(ab, ab2) {
		t.Error("identical lists reported unequal")
	}
	if Equal(ab, ba) {
		t.Error("reordered lists reported equal; List equality must be order-sensitive")
	}
	if Equal(ab, a) {
		t.Error("different-length lists reported equal")
	}
}

func TestEqual_MapsAreOrderInsensitive(t *testing.T) {
	// Iteration order is part of a Map's identity for rendering, but two maps
	// with the same entries are the same value.
	m1 := OfMap(NewMapBuilder().Set("a", OfInt(1)).Set("b", OfInt(2)).Build())
	m2 := OfMap(NewMapBuilder().Set("b", OfInt(2)).Set("a", OfInt(1)).Build())
	m3 := OfMap(NewMapBuilder().Set("a", OfInt(1)).Build())
	m4 := OfMap(NewMapBuilder().Set("a", OfInt(1)).Set("b", OfInt(99)).Build())

	if !Equal(m1, m2) {
		t.Error("maps with the same entries in different insertion order reported unequal")
	}
	if Equal(m1, m3) {
		t.Error("maps of different size reported equal")
	}
	if Equal(m1, m4) {
		t.Error("maps differing in a value reported equal")
	}
}

func TestEqual_Nested(t *testing.T) {
	build := func(inner int64) Value {
		return OfMap(NewMapBuilder().
			Set("list", OfList(NewList(OfInt(inner), OfString("x")))).
			Build())
	}
	if !Equal(build(1), build(1)) {
		t.Error("identical nested structures reported unequal")
	}
	if Equal(build(1), build(2)) {
		t.Error("differing nested structures reported equal")
	}
}
```

- [ ] **Step 2: Write the implementation**

Create `internal/value/value.go`:

```go
// Package value defines the closed set of values the runtime can hold.
//
// Nothing in the runtime carries configuration or workflow data as `any`. Every
// value is a Value, whose Kind is always known, whose numbers preserve their
// source literal, and whose maps iterate deterministically. That closure is
// what lets later stages check, coerce and render values in exactly one place
// instead of once per consumer.
//
// Values are immutable. Composite kinds copy on construction and on any
// accessor that returns a slice, so a Value can be bound into a scope, captured
// by deferred work, and read concurrently without synchronisation.
package value

// Kind identifies which member of the Value union is present.
type Kind uint8

const (
	// KindNull is the zero Kind, so the zero Value is null.
	KindNull Kind = iota
	KindBool
	KindNumber
	KindString
	KindBytes
	KindList
	KindMap
)

// String renders the kind for diagnostics.
func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindBool:
		return "bool"
	case KindNumber:
		return "number"
	case KindString:
		return "string"
	case KindBytes:
		return "bytes"
	case KindList:
		return "list"
	case KindMap:
		return "map"
	default:
		return "invalid"
	}
}

// Value is a member of the runtime's closed value union.
//
// The zero Value is null.
//
// Do not compare Values with ==; it compares the internal representation, which
// treats the numbers 1 and 1.0 as different and dereferences nothing. Use Equal.
type Value struct {
	kind Kind
	s    string // KindString: the string. KindNumber: the literal.
	b    bool   // KindBool
	ref  any    // KindBytes: []byte. KindList: List. KindMap: Map.
}

// Null returns the null value.
func Null() Value { return Value{} }

// OfBool returns a bool value.
func OfBool(b bool) Value { return Value{kind: KindBool, b: b} }

// OfNumber returns a number value.
func OfNumber(n Number) Value { return Value{kind: KindNumber, s: n.Literal()} }

// OfInt returns a number value for i.
func OfInt(i int64) Value { return OfNumber(NumberFromInt(i)) }

// OfFloat returns a number value for f, in non-scientific notation.
func OfFloat(f float64) Value { return OfNumber(NumberFromFloat(f)) }

// OfString returns a string value.
func OfString(s string) Value { return Value{kind: KindString, s: s} }

// OfBytes returns a bytes value holding a copy of b.
func OfBytes(b []byte) Value {
	cp := make([]byte, len(b))
	copy(cp, b)
	return Value{kind: KindBytes, ref: cp}
}

// OfList returns a list value.
func OfList(l List) Value { return Value{kind: KindList, ref: l} }

// OfMap returns a map value.
func OfMap(m Map) Value { return Value{kind: KindMap, ref: m} }

// Kind returns which member of the union is present.
func (v Value) Kind() Kind { return v.kind }

// IsNull reports whether the value is null.
func (v Value) IsNull() bool { return v.kind == KindNull }

// Bool returns the bool and whether the value is one.
func (v Value) Bool() (bool, bool) {
	if v.kind != KindBool {
		return false, false
	}
	return v.b, true
}

// Number returns the number and whether the value is one.
func (v Value) Number() (Number, bool) {
	if v.kind != KindNumber {
		return Number{}, false
	}
	return Number{lit: v.s}, true
}

// Str returns the string and whether the value is one.
//
// Named Str rather than String so that Value does not accidentally satisfy
// fmt.Stringer, which would make a bare %v print a bare string and hide the
// kind in every log line and error message.
func (v Value) Str() (string, bool) {
	if v.kind != KindString {
		return "", false
	}
	return v.s, true
}

// Bytes returns a copy of the bytes and whether the value is a bytes value.
func (v Value) Bytes() ([]byte, bool) {
	if v.kind != KindBytes {
		return nil, false
	}
	src, _ := v.ref.([]byte)
	cp := make([]byte, len(src))
	copy(cp, src)
	return cp, true
}

// List returns the list and whether the value is one.
func (v Value) List() (List, bool) {
	if v.kind != KindList {
		return List{}, false
	}
	l, _ := v.ref.(List)
	return l, true
}

// Map returns the map and whether the value is one.
func (v Value) Map() (Map, bool) {
	if v.kind != KindMap {
		return Map{}, false
	}
	m, _ := v.ref.(Map)
	return m, true
}
```

Create `internal/value/equal.go`:

```go
package value

// Equal reports structural equality.
//
// Numbers compare by value, so 1, 1.0 and 1e0 are equal. Lists compare
// element-wise and are order-sensitive. Maps compare entry-wise and are order-
// insensitive: insertion order is part of how a Map renders, not of what it is.
func Equal(a, b Value) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case KindNull:
		return true
	case KindBool:
		return a.b == b.b
	case KindNumber:
		an, _ := a.Number()
		bn, _ := b.Number()
		return an.NumberEqual(bn)
	case KindString:
		return a.s == b.s
	case KindBytes:
		ab, _ := a.ref.([]byte)
		bb, _ := b.ref.([]byte)
		if len(ab) != len(bb) {
			return false
		}
		for i := range ab {
			if ab[i] != bb[i] {
				return false
			}
		}
		return true
	case KindList:
		al, _ := a.List()
		bl, _ := b.List()
		if al.Len() != bl.Len() {
			return false
		}
		for i := 0; i < al.Len(); i++ {
			if !Equal(al.At(i), bl.At(i)) {
				return false
			}
		}
		return true
	case KindMap:
		am, _ := a.Map()
		bm, _ := b.Map()
		if am.Len() != bm.Len() {
			return false
		}
		for _, k := range am.Keys() {
			bv, ok := bm.Get(k)
			if !ok {
				return false
			}
			av, _ := am.Get(k)
			if !Equal(av, bv) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
```

- [ ] **Step 3: Run the tests and verify they pass**

Run: `go test ./internal/value/ -v`
Expected: PASS. This is the first point at which Task 2's `List` and `Map` tests also compile and run — confirm `TestMap_IteratesInInsertionOrder` and `TestList_IsImmutable` are among the passing tests.

- [ ] **Step 4: Verify the tests have teeth**

Two mutations, each reverted after observing the failure:

1. In `Equal`, change the `KindNumber` case to `return a.s == b.s`.
   Expected: `TestEqual_Scalars` FAILS on `1 == 1.0`.
2. In `Map.Build`, add `sort.Strings(keys)` after the `copy(keys, b.keys)` line (and `import "sort"`).
   Expected: `TestMap_IteratesInInsertionOrder` FAILS — it wants `zebra, apple, mango`, not sorted order.

- [ ] **Step 5: Commit**

```bash
git add internal/value/value.go internal/value/equal.go internal/value/value_test.go internal/value/equal_test.go
git commit -m "feat(value): closed Value union with structural equality

Replaces map[string]any as the runtime's data representation. Kind is
always known, numbers compare by value rather than by literal, and every
composite copies on construction and on slice-returning accessors so a
Value can be bound into a scope and read concurrently."
```

---

### Task 4: JSON decoding and encoding

Spec §3.1 and §3.2 together. `encoding/json` into `map[string]any` destroys both properties at once — numbers become `float64` and object key order is lost. Token-based decoding preserves both.

**Files:**
- Create: `internal/value/json.go`
- Test: `internal/value/json_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–3.
- Produces:
  - `func DecodeJSON(data []byte) (Value, error)`
  - `func EncodeJSON(v Value) ([]byte, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/value/json_test.go`:

```go
package value

import "testing"

func TestDecodeJSON_PreservesNumberLiterals(t *testing.T) {
	// The defect this whole layer exists to prevent. encoding/json into
	// map[string]any renders this back as 1.2345678901234567e+19.
	const src = `{"id":12345678901234567890,"ratio":0.0000001,"big":1e21}`
	v, err := DecodeJSON([]byte(src))
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	m, ok := v.Map()
	if !ok {
		t.Fatalf("top level is %v, want map", v.Kind())
	}
	want := map[string]string{
		"id":    "12345678901234567890",
		"ratio": "0.0000001",
		"big":   "1e21",
	}
	for key, wantLit := range want {
		got, ok := m.Get(key)
		if !ok {
			t.Fatalf("key %q missing", key)
		}
		n, ok := got.Number()
		if !ok {
			t.Fatalf("key %q is %v, want number", key, got.Kind())
		}
		if n.Literal() != wantLit {
			t.Errorf("key %q literal = %q, want %q", key, n.Literal(), wantLit)
		}
	}
}

func TestDecodeJSON_PreservesObjectKeyOrder(t *testing.T) {
	const src = `{"zebra":1,"apple":2,"mango":3}`
	v, err := DecodeJSON([]byte(src))
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	m, _ := v.Map()
	want := []string{"zebra", "apple", "mango"}
	got := m.Keys()
	if len(got) != len(want) {
		t.Fatalf("Keys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Keys() = %v, want %v", got, want)
		}
	}
}

func TestDecodeJSON_AllKinds(t *testing.T) {
	const src = `{"n":null,"t":true,"f":false,"num":1.5,"s":"hi","l":[1,"two",null],"o":{"k":"v"}}`
	v, err := DecodeJSON([]byte(src))
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	m, _ := v.Map()

	check := func(key string, want Kind) Value {
		t.Helper()
		got, ok := m.Get(key)
		if !ok {
			t.Fatalf("key %q missing", key)
		}
		if got.Kind() != want {
			t.Fatalf("key %q kind = %v, want %v", key, got.Kind(), want)
		}
		return got
	}

	check("n", KindNull)
	if b, _ := check("t", KindBool).Bool(); b != true {
		t.Error("t = false, want true")
	}
	if b, _ := check("f", KindBool).Bool(); b != false {
		t.Error("f = true, want false")
	}
	check("num", KindNumber)
	if s, _ := check("s", KindString).Str(); s != "hi" {
		t.Error("s mismatch")
	}
	l, _ := check("l", KindList).List()
	if l.Len() != 3 {
		t.Errorf("l len = %d, want 3", l.Len())
	}
	if !l.At(2).IsNull() {
		t.Error("l[2] is not null")
	}
	o, _ := check("o", KindMap).Map()
	if o.Len() != 1 {
		t.Errorf("o len = %d, want 1", o.Len())
	}
}

func TestDecodeJSON_RejectsMalformedAndTrailingContent(t *testing.T) {
	bad := []string{
		``, `{`, `{"a":}`, `[1,]`, `{"a":1}{"b":2}`, `1 2`, `{"a" 1}`,
	}
	for _, src := range bad {
		if _, err := DecodeJSON([]byte(src)); err == nil {
			t.Errorf("DecodeJSON(%q) = nil error, want rejection", src)
		}
	}
}

func TestEncodeJSON_EmitsExactNumberLiterals(t *testing.T) {
	v := OfMap(NewMapBuilder().
		Set("id", OfNumber(mustParse(t, "12345678901234567890"))).
		Set("big", OfNumber(mustParse(t, "1e21"))).
		Build())

	got, err := EncodeJSON(v)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	const want = `{"id":12345678901234567890,"big":1e21}`
	if string(got) != want {
		t.Errorf("EncodeJSON = %s, want %s", got, want)
	}
}

func TestEncodeJSON_EscapesStrings(t *testing.T) {
	v := OfString("a\"b\\c\nd\te")
	got, err := EncodeJSON(v)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	const want = `"a\"b\\c\nd\te"`
	if string(got) != want {
		t.Errorf("EncodeJSON = %s, want %s", got, want)
	}
}

func TestEncodeJSON_BytesAsBase64(t *testing.T) {
	got, err := EncodeJSON(OfBytes([]byte("hello")))
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	const want = `"aGVsbG8="`
	if string(got) != want {
		t.Errorf("EncodeJSON = %s, want %s", got, want)
	}
}

func TestJSON_RoundTripsByteForByte(t *testing.T) {
	// Canonical input — no insignificant whitespace — must survive intact.
	sources := []string{
		`null`,
		`true`,
		`1.5`,
		`"hello"`,
		`[]`,
		`{}`,
		`[1,2,3]`,
		`{"zebra":1,"apple":[true,null],"nested":{"deep":{"deeper":"x"}}}`,
		`{"id":12345678901234567890}`,
	}
	for _, src := range sources {
		v, err := DecodeJSON([]byte(src))
		if err != nil {
			t.Errorf("DecodeJSON(%s): %v", src, err)
			continue
		}
		got, err := EncodeJSON(v)
		if err != nil {
			t.Errorf("EncodeJSON for %s: %v", src, err)
			continue
		}
		if string(got) != src {
			t.Errorf("round trip: %s -> %s", src, got)
		}
	}
}

func mustParse(t *testing.T, lit string) Number {
	t.Helper()
	n, err := ParseNumber(lit)
	if err != nil {
		t.Fatalf("ParseNumber(%q): %v", lit, err)
	}
	return n
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/value/ -run TestDecodeJSON -v`
Expected: FAIL — `undefined: DecodeJSON`

- [ ] **Step 3: Write the implementation**

Create `internal/value/json.go`:

```go
package value

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// DecodeJSON parses JSON into a Value.
//
// It decodes token by token rather than into map[string]any, because that
// target loses both properties this package exists to keep: numbers become
// float64 (losing the literal) and object key order is discarded.
//
// Trailing content after the first value is an error.
func DecodeJSON(data []byte) (Value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	v, err := decodeValue(dec)
	if err != nil {
		return Value{}, err
	}

	// Reject trailing content: "{}{}"" and "1 2" are not single values.
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return Value{}, fmt.Errorf("unexpected content after top-level JSON value")
		}
		return Value{}, err
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return Value{}, err
	}
	return decodeFromToken(dec, tok)
}

func decodeFromToken(dec *json.Decoder, tok json.Token) (Value, error) {
	switch t := tok.(type) {
	case nil:
		return Null(), nil
	case bool:
		return OfBool(t), nil
	case string:
		return OfString(t), nil
	case json.Number:
		n, err := ParseNumber(t.String())
		if err != nil {
			return Value{}, err
		}
		return OfNumber(n), nil
	case json.Delim:
		switch t {
		case '[':
			return decodeList(dec)
		case '{':
			return decodeMap(dec)
		default:
			return Value{}, fmt.Errorf("unexpected delimiter %q", t)
		}
	default:
		return Value{}, fmt.Errorf("unexpected JSON token %T", tok)
	}
}

func decodeList(dec *json.Decoder) (Value, error) {
	b := NewListBuilder()
	for dec.More() {
		v, err := decodeValue(dec)
		if err != nil {
			return Value{}, err
		}
		b.Append(v)
	}
	if _, err := dec.Token(); err != nil { // consume ']'
		return Value{}, err
	}
	return OfList(b.Build()), nil
}

func decodeMap(dec *json.Decoder) (Value, error) {
	b := NewMapBuilder()
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return Value{}, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return Value{}, fmt.Errorf("object key is %T, want string", keyTok)
		}
		v, err := decodeValue(dec)
		if err != nil {
			return Value{}, err
		}
		b.Set(key, v)
	}
	if _, err := dec.Token(); err != nil { // consume '}'
		return Value{}, err
	}
	return OfMap(b.Build()), nil
}

// EncodeJSON serialises a Value to JSON.
//
// Numbers are written as their stored literal, so a value decoded from JSON and
// re-encoded is byte-identical for canonical input. Bytes are base64-encoded
// strings, matching how they decode back.
func EncodeJSON(v Value) ([]byte, error) {
	var buf bytes.Buffer
	if err := encodeInto(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeInto(buf *bytes.Buffer, v Value) error {
	switch v.Kind() {
	case KindNull:
		buf.WriteString("null")
		return nil
	case KindBool:
		b, _ := v.Bool()
		buf.WriteString(strconv.FormatBool(b))
		return nil
	case KindNumber:
		n, _ := v.Number()
		buf.WriteString(n.Literal())
		return nil
	case KindString:
		s, _ := v.Str()
		return encodeString(buf, s)
	case KindBytes:
		b, _ := v.Bytes()
		return encodeString(buf, base64.StdEncoding.EncodeToString(b))
	case KindList:
		l, _ := v.List()
		buf.WriteByte('[')
		for i := 0; i < l.Len(); i++ {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeInto(buf, l.At(i)); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	case KindMap:
		m, _ := v.Map()
		buf.WriteByte('{')
		for i, k := range m.Keys() {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeString(buf, k); err != nil {
				return err
			}
			buf.WriteByte(':')
			mv, _ := m.Get(k)
			if err := encodeInto(buf, mv); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	default:
		return fmt.Errorf("cannot encode value of kind %v", v.Kind())
	}
}

// encodeString writes a JSON string. It delegates escaping to encoding/json so
// the escape rules are the standard library's, then strips the trailing newline
// SetEscapeHTML/Encode adds.
func encodeString(buf *bytes.Buffer, s string) error {
	var tmp bytes.Buffer
	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	out := tmp.Bytes()
	buf.Write(bytes.TrimRight(out, "\n"))
	return nil
}
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./internal/value/ -v`
Expected: PASS, all tests including Tasks 1–3.

- [ ] **Step 5: Verify the tests have teeth**

Replace the `json.Number` case in `decodeFromToken` with:

```go
case json.Number:
    f, _ := t.Float64()
    return OfFloat(f), nil
```

Expected: `TestDecodeJSON_PreservesNumberLiterals` FAILS on `id` and `big`, and `TestJSON_RoundTripsByteForByte` FAILS on `{"id":12345678901234567890}`. Revert.

- [ ] **Step 6: Commit**

```bash
git add internal/value/json.go internal/value/json_test.go
git commit -m "feat(value): token-based JSON decode/encode

Decoding into map[string]any loses the number literal and the object key
order in one step. Token-based decoding keeps both, so canonical JSON
round-trips byte for byte."
```

---

### Task 5: The conversion table

Spec §3.3. This table is normative: it is the one place a value becomes a string, a query parameter, a header, or a SQL argument. It replaces the 18 independent coercion sites the census found.

Two layering decisions, recorded here because a reviewer will ask:

- **Percent-encoding is not done here.** `ToQueryValues` returns raw strings; the HTTP trigger driver passes them to `url.Values`, which encodes. Encoding depends on URL position, which the value layer does not know.
- **A number too large for int64 becomes a SQL argument as its literal string.** PostgreSQL and MySQL both parse a string into a numeric column correctly, and the alternative — silently returning a lossy float64 — is the defect this layer exists to prevent. Revisit when a real database driver exists.

**Files:**
- Create: `internal/value/convert.go`
- Test: `internal/value/convert_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–4.
- Produces:
  - `func (v Value) ToString() (string, error)`
  - `func (v Value) ToQueryValues() ([]string, error)`
  - `func (v Value) ToHeaderValues() ([]string, error)`
  - `func (v Value) ToSQLArg() (any, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/value/convert_test.go`:

```go
package value

import (
	"strings"
	"testing"
)

func TestToString(t *testing.T) {
	tests := []struct {
		name    string
		v       Value
		want    string
		wantErr bool
	}{
		{name: "bool true", v: OfBool(true), want: "true"},
		{name: "bool false", v: OfBool(false), want: "false"},
		{name: "string", v: OfString("hi"), want: "hi"},
		{name: "small int", v: OfInt(10), want: "10"},
		{name: "float", v: OfFloat(1.5), want: "1.5"},
		// The four rows that motivated the whole layer.
		{name: "huge int", v: OfNumber(mustParse(t, "12345678901234567890")), want: "12345678901234567890"},
		{name: "1e21", v: OfNumber(mustParse(t, "1e21")), want: "1e21"},
		{name: "tiny", v: OfNumber(mustParse(t, "0.0000001")), want: "0.0000001"},
		{name: "null", v: Null(), wantErr: true},
		{name: "bytes", v: OfBytes([]byte{1}), wantErr: true},
		{name: "list", v: OfList(NewList(OfInt(1))), wantErr: true},
		{name: "map", v: OfMap(NewMapBuilder().Build()), wantErr: true},
	}
	for _, tc := range tests {
		got, err := tc.v.ToString()
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: ToString() = %q, nil; want error", tc.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: ToString() error: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: ToString() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestToQueryValues(t *testing.T) {
	tests := []struct {
		name    string
		v       Value
		want    []string
		wantErr bool
	}{
		{name: "null is omitted", v: Null(), want: nil},
		{name: "bool", v: OfBool(true), want: []string{"true"}},
		{name: "string", v: OfString("a b"), want: []string{"a b"}}, // raw; url.Values encodes
		{name: "huge number", v: OfNumber(mustParse(t, "12345678901234567890")), want: []string{"12345678901234567890"}},
		{name: "list becomes repeats", v: OfList(NewList(OfString("new"), OfString("sale"))), want: []string{"new", "sale"}},
		{name: "empty list", v: OfList(NewList()), want: nil},
		{name: "list with null drops the null", v: OfList(NewList(OfString("a"), Null())), want: []string{"a"}},
		{name: "nested list", v: OfList(NewList(OfList(NewList(OfInt(1))))), wantErr: true},
		{name: "map", v: OfMap(NewMapBuilder().Build()), wantErr: true},
	}
	for _, tc := range tests {
		got, err := tc.v.ToQueryValues()
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: ToQueryValues() = %v, nil; want error", tc.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: ToQueryValues() error: %v", tc.name, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: ToQueryValues() = %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("%s: ToQueryValues() = %v, want %v", tc.name, got, tc.want)
				break
			}
		}
	}
}

func TestToHeaderValues_MirrorsQueryForRepeats(t *testing.T) {
	// Headers and query parameters diverged on exactly this in the reference
	// implementation: headers preserved repeats, query parameters dropped all
	// but one. One table, one behaviour.
	v := OfList(NewList(OfString("a"), OfString("b")))

	q, err := v.ToQueryValues()
	if err != nil {
		t.Fatalf("ToQueryValues: %v", err)
	}
	h, err := v.ToHeaderValues()
	if err != nil {
		t.Fatalf("ToHeaderValues: %v", err)
	}
	if len(q) != len(h) {
		t.Fatalf("query gave %d values, header gave %d; they must agree", len(q), len(h))
	}
	for i := range q {
		if q[i] != h[i] {
			t.Errorf("index %d: query %q, header %q", i, q[i], h[i])
		}
	}
}

func TestToHeaderValues_RejectsCRLF(t *testing.T) {
	// Header injection.
	bad := []string{"a\r\nX-Evil: 1", "a\nb", "a\rb"}
	for _, s := range bad {
		if _, err := OfString(s).ToHeaderValues(); err == nil {
			t.Errorf("ToHeaderValues(%q) = nil error, want rejection", s)
		}
	}
	if _, err := OfList(NewList(OfString("ok"), OfString("bad\nvalue"))).ToHeaderValues(); err == nil {
		t.Error("ToHeaderValues with a CRLF element = nil error, want rejection")
	}
}

func TestToHeaderValues_NullIsOmitted(t *testing.T) {
	got, err := Null().ToHeaderValues()
	if err != nil {
		t.Fatalf("ToHeaderValues: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ToHeaderValues(null) = %v, want empty", got)
	}
}

func TestToSQLArg(t *testing.T) {
	t.Run("null", func(t *testing.T) {
		got, err := Null().ToSQLArg()
		if err != nil {
			t.Fatalf("ToSQLArg: %v", err)
		}
		if got != nil {
			t.Errorf("ToSQLArg(null) = %v, want nil", got)
		}
	})

	t.Run("bool", func(t *testing.T) {
		got, err := OfBool(true).ToSQLArg()
		if err != nil {
			t.Fatalf("ToSQLArg: %v", err)
		}
		if b, ok := got.(bool); !ok || !b {
			t.Errorf("ToSQLArg(true) = %#v, want bool true", got)
		}
	})

	t.Run("integral number becomes int64", func(t *testing.T) {
		got, err := OfInt(42).ToSQLArg()
		if err != nil {
			t.Fatalf("ToSQLArg: %v", err)
		}
		if i, ok := got.(int64); !ok || i != 42 {
			t.Errorf("ToSQLArg(42) = %#v, want int64 42", got)
		}
	})

	t.Run("number too large for int64 becomes its literal", func(t *testing.T) {
		got, err := OfNumber(mustParse(t, "12345678901234567890")).ToSQLArg()
		if err != nil {
			t.Fatalf("ToSQLArg: %v", err)
		}
		s, ok := got.(string)
		if !ok || s != "12345678901234567890" {
			t.Errorf("ToSQLArg(huge) = %#v, want string %q", got, "12345678901234567890")
		}
	})

	t.Run("fractional number becomes its literal", func(t *testing.T) {
		got, err := OfFloat(1.5).ToSQLArg()
		if err != nil {
			t.Fatalf("ToSQLArg: %v", err)
		}
		if s, ok := got.(string); !ok || s != "1.5" {
			t.Errorf("ToSQLArg(1.5) = %#v, want string %q", got, "1.5")
		}
	})

	t.Run("string", func(t *testing.T) {
		got, err := OfString("hi").ToSQLArg()
		if err != nil {
			t.Fatalf("ToSQLArg: %v", err)
		}
		if s, ok := got.(string); !ok || s != "hi" {
			t.Errorf("ToSQLArg = %#v, want string \"hi\"", got)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		got, err := OfBytes([]byte{1, 2}).ToSQLArg()
		if err != nil {
			t.Fatalf("ToSQLArg: %v", err)
		}
		b, ok := got.([]byte)
		if !ok || len(b) != 2 || b[0] != 1 {
			t.Errorf("ToSQLArg = %#v, want []byte{1,2}", got)
		}
	})

	t.Run("list becomes a slice of converted elements", func(t *testing.T) {
		got, err := OfList(NewList(OfInt(1), OfString("x"))).ToSQLArg()
		if err != nil {
			t.Fatalf("ToSQLArg: %v", err)
		}
		s, ok := got.([]any)
		if !ok || len(s) != 2 {
			t.Fatalf("ToSQLArg = %#v, want []any of len 2", got)
		}
		if i, ok := s[0].(int64); !ok || i != 1 {
			t.Errorf("element 0 = %#v, want int64 1", s[0])
		}
	})

	t.Run("map becomes JSON bytes", func(t *testing.T) {
		got, err := OfMap(NewMapBuilder().Set("k", OfString("v")).Build()).ToSQLArg()
		if err != nil {
			t.Fatalf("ToSQLArg: %v", err)
		}
		b, ok := got.([]byte)
		if !ok {
			t.Fatalf("ToSQLArg = %#v, want []byte", got)
		}
		if string(b) != `{"k":"v"}` {
			t.Errorf("ToSQLArg = %s, want %s", b, `{"k":"v"}`)
		}
	})
}

func TestConversionErrorsNameTheKind(t *testing.T) {
	// A diagnostic that does not say what it received is one the reader has to
	// reproduce to understand.
	_, err := OfList(NewList()).ToString()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "list") {
		t.Errorf("error %q does not name the offending kind", err)
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/value/ -run TestToString -v`
Expected: FAIL — `v.ToString undefined`

- [ ] **Step 3: Write the implementation**

Create `internal/value/convert.go`:

```go
package value

import (
	"fmt"
	"strconv"
	"strings"
)

// The conversions below are the normative table from the design spec, §3.3.
// They are the only place a Value becomes a string, a query parameter, a header
// or a SQL argument. The reference implementation had eighteen independent
// implementations of this, two of which agreed.

// ToString renders the value as a bare string.
//
// Only scalars have a string rendering. A list or map reaching a string context
// is a bug in the caller, not something to paper over with a Go-syntax dump.
func (v Value) ToString() (string, error) {
	switch v.Kind() {
	case KindBool:
		b, _ := v.Bool()
		return strconv.FormatBool(b), nil
	case KindNumber:
		n, _ := v.Number()
		return n.Literal(), nil
	case KindString:
		s, _ := v.Str()
		return s, nil
	default:
		return "", fmt.Errorf("cannot render %s as a string", v.Kind())
	}
}

// ToQueryValues renders the value as zero or more query-parameter values.
//
// A null is omitted entirely — the parameter is absent, not empty. A list
// becomes one value per element, which a caller renders as a repeated
// parameter. Nested lists have no query representation.
//
// The values returned are raw and unencoded: percent-encoding depends on
// position within a URL, which this layer does not know. Callers pass them to
// net/url.Values, which encodes.
func (v Value) ToQueryValues() ([]string, error) {
	return v.toMultiValue("query parameter")
}

// ToHeaderValues renders the value as zero or more header values, with the same
// rules as ToQueryValues plus a rejection of CR and LF.
//
// Headers and query parameters share one implementation deliberately: in the
// reference implementation they diverged, and a repeated header survived while
// a repeated query parameter was silently truncated to one.
func (v Value) ToHeaderValues() ([]string, error) {
	vals, err := v.toMultiValue("header value")
	if err != nil {
		return nil, err
	}
	for _, s := range vals {
		if strings.ContainsAny(s, "\r\n") {
			return nil, fmt.Errorf("header value contains CR or LF: %q", s)
		}
	}
	return vals, nil
}

func (v Value) toMultiValue(context string) ([]string, error) {
	switch v.Kind() {
	case KindNull:
		return nil, nil
	case KindBool, KindNumber, KindString:
		s, err := v.ToString()
		if err != nil {
			return nil, err
		}
		return []string{s}, nil
	case KindList:
		l, _ := v.List()
		var out []string
		for i := 0; i < l.Len(); i++ {
			item := l.At(i)
			if item.IsNull() {
				continue
			}
			s, err := item.ToString()
			if err != nil {
				return nil, fmt.Errorf("%s element %d: %w", context, i, err)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("cannot render %s as a %s", v.Kind(), context)
	}
}

// ToSQLArg renders the value as a database/sql argument.
//
// A number that is integral and fits in an int64 becomes one. Anything else —
// fractional, or too large — becomes its exact literal as a string, which both
// PostgreSQL and MySQL parse correctly into numeric columns. Returning a lossy
// float64 instead would reintroduce the precision loss this package exists to
// prevent.
//
// The `any` return type is dictated by database/sql.
func (v Value) ToSQLArg() (any, error) {
	switch v.Kind() {
	case KindNull:
		return nil, nil
	case KindBool:
		b, _ := v.Bool()
		return b, nil
	case KindNumber:
		n, _ := v.Number()
		if i, err := n.Int64(); err == nil {
			return i, nil
		}
		return n.Literal(), nil
	case KindString:
		s, _ := v.Str()
		return s, nil
	case KindBytes:
		b, _ := v.Bytes()
		return b, nil
	case KindList:
		l, _ := v.List()
		out := make([]any, 0, l.Len())
		for i := 0; i < l.Len(); i++ {
			a, err := l.At(i).ToSQLArg()
			if err != nil {
				return nil, fmt.Errorf("sql argument element %d: %w", i, err)
			}
			out = append(out, a)
		}
		return out, nil
	case KindMap:
		return EncodeJSON(v)
	default:
		return nil, fmt.Errorf("cannot render %s as a SQL argument", v.Kind())
	}
}
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./internal/value/ -v`
Expected: PASS.

- [ ] **Step 5: Verify the tests have teeth**

Two mutations, each reverted after observing the failure:

1. In `ToString`, change the `KindNumber` case to:
   ```go
   n, _ := v.Number()
   f, _ := n.Float64()
   return fmt.Sprintf("%v", f), nil
   ```
   Expected: `TestToString` FAILS on `huge int`, `1e21` and `tiny` — reproducing the exact defect from the reference implementation.
2. In `toMultiValue`, change the `KindList` case to return only `out[:1]`.
   Expected: `TestToHeaderValues_MirrorsQueryForRepeats` FAILS — reproducing the query/header divergence.

- [ ] **Step 6: Commit**

```bash
git add internal/value/convert.go internal/value/convert_test.go
git commit -m "feat(value): the normative conversion table

One implementation of Value -> string / query / header / SQL argument.
The reference implementation had eighteen, two of which agreed, and the
query and header paths disagreed inside a single file twenty lines apart."
```

---

### Task 6: The `Type` language

Spec §3.4. Types are what make expressions checkable; this task defines the vocabulary and its rendering, and nothing else.

**Files:**
- Create: `internal/types/type.go`
- Test: `internal/types/type_test.go`

**Interfaces:**
- Consumes: nothing from `internal/value` yet.
- Produces:
  - `type Kind uint8` with `KindAny`, `KindNull`, `KindBool`, `KindNumber`, `KindString`, `KindBytes`, `KindList`, `KindMap`, `KindRecord`, `KindUnion`, `KindRef`
  - `type Type struct{ ... }`, `type Field struct{ Name string; Type Type; Optional bool }`
  - `func Any() Type`, `func Null() Type`, `func Bool() Type`, `func Number() Type`, `func String() Type`, `func Bytes() Type`
  - `func List(elem Type) Type`, `func Map(elem Type) Type`, `func Record(fields ...Field) Type`, `func Union(members ...Type) Type`, `func Ref(name string) Type`
  - `func (t Type) Kind() Kind`, `func (t Type) Elem() Type`, `func (t Type) Fields() []Field`, `func (t Type) Members() []Type`, `func (t Type) RefName() string`
  - `func (t Type) String() string`

- [ ] **Step 1: Write the failing test**

Create `internal/types/type_test.go`:

```go
package types

import "testing"

func TestZeroTypeIsAny(t *testing.T) {
	// The zero Type must be the permissive one. A zero value that meant "null"
	// would silently reject everything wherever a Type was left unset.
	var zero Type
	if zero.Kind() != KindAny {
		t.Errorf("zero Type Kind() = %v, want KindAny", zero.Kind())
	}
}

func TestConstructorsSetKind(t *testing.T) {
	tests := []struct {
		t    Type
		want Kind
	}{
		{Any(), KindAny},
		{Null(), KindNull},
		{Bool(), KindBool},
		{Number(), KindNumber},
		{String(), KindString},
		{Bytes(), KindBytes},
		{List(String()), KindList},
		{Map(Number()), KindMap},
		{Record(), KindRecord},
		{Union(String(), Number()), KindUnion},
		{Ref("Article"), KindRef},
	}
	for _, tc := range tests {
		if got := tc.t.Kind(); got != tc.want {
			t.Errorf("%s: Kind() = %v, want %v", tc.t, got, tc.want)
		}
	}
}

func TestCompositeAccessors(t *testing.T) {
	l := List(String())
	if l.Elem().Kind() != KindString {
		t.Errorf("List(String()).Elem() = %v, want string", l.Elem())
	}

	m := Map(Number())
	if m.Elem().Kind() != KindNumber {
		t.Errorf("Map(Number()).Elem() = %v, want number", m.Elem())
	}

	r := Record(
		Field{Name: "id", Type: String()},
		Field{Name: "count", Type: Number(), Optional: true},
	)
	fields := r.Fields()
	if len(fields) != 2 {
		t.Fatalf("Fields() len = %d, want 2", len(fields))
	}
	if fields[0].Name != "id" || fields[0].Optional {
		t.Errorf("field 0 = %+v, want required id", fields[0])
	}
	if fields[1].Name != "count" || !fields[1].Optional {
		t.Errorf("field 1 = %+v, want optional count", fields[1])
	}

	u := Union(String(), Number())
	if len(u.Members()) != 2 {
		t.Errorf("Members() len = %d, want 2", len(u.Members()))
	}

	if Ref("Article").RefName() != "Article" {
		t.Errorf("RefName() = %q, want %q", Ref("Article").RefName(), "Article")
	}
}

func TestRecordPreservesFieldOrder(t *testing.T) {
	// Field order is declaration order, so diagnostics and editor forms present
	// fields the way the author wrote them.
	r := Record(
		Field{Name: "zebra", Type: String()},
		Field{Name: "apple", Type: String()},
		Field{Name: "mango", Type: String()},
	)
	want := []string{"zebra", "apple", "mango"}
	for i, f := range r.Fields() {
		if f.Name != want[i] {
			t.Fatalf("Fields() order = %v, want %v", r.Fields(), want)
		}
	}
}

func TestCompositesAreImmutable(t *testing.T) {
	src := []Field{{Name: "a", Type: String()}}
	r := Record(src...)
	src[0].Name = "MUTATED"
	if r.Fields()[0].Name != "a" {
		t.Error("Record observed a mutation of its constructor input")
	}
	got := r.Fields()
	got[0].Name = "MUTATED"
	if r.Fields()[0].Name != "a" {
		t.Error("Record observed a mutation of Fields()")
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		t    Type
		want string
	}{
		{Any(), "Any"},
		{Null(), "Null"},
		{Bool(), "Bool"},
		{Number(), "Number"},
		{String(), "String"},
		{Bytes(), "Bytes"},
		{List(String()), "List<String>"},
		{Map(Number()), "Map<Number>"},
		{List(List(Any())), "List<List<Any>>"},
		{Record(), "{}"},
		{Record(Field{Name: "id", Type: String()}), "{ id: String }"},
		{Record(
			Field{Name: "id", Type: String()},
			Field{Name: "n", Type: Number(), Optional: true},
		), "{ id: String, n?: Number }"},
		{Union(String(), Number()), "String | Number"},
		{Ref("Article"), "Article"},
		{Map(Record(Field{Name: "k", Type: Bool()})), "Map<{ k: Bool }>"},
	}
	for _, tc := range tests {
		if got := tc.t.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/types/ -v`
Expected: FAIL — `undefined: Type`

- [ ] **Step 3: Write the implementation**

Create `internal/types/type.go`:

```go
// Package types defines the type language the runtime checks values and
// expressions against.
//
// Typing is gradual: Any is assignable to and from every type, and reading a
// field of Any yields Any. That is not a loophole but a requirement — an HTTP
// response body or an untyped database row is genuinely unknown at compile
// time. Static checks apply wherever a shape is declared and step aside where
// it is Any.
package types

import "strings"

// Kind identifies which member of the Type union is present.
type Kind uint8

const (
	// KindAny is the zero Kind, so the zero Type is Any — the permissive one.
	// A zero value meaning Null would silently reject everything wherever a
	// Type was left unset.
	KindAny Kind = iota
	KindNull
	KindBool
	KindNumber
	KindString
	KindBytes
	KindList
	KindMap
	KindRecord
	KindUnion
	KindRef
)

// Field is one member of a Record.
type Field struct {
	Name     string
	Type     Type
	Optional bool
}

// Type is a member of the type language.
//
// The zero Type is Any.
type Type struct {
	kind    Kind
	elem    *Type   // KindList, KindMap
	fields  []Field // KindRecord
	members []Type  // KindUnion
	name    string  // KindRef
}

// Any returns the type every value satisfies.
func Any() Type { return Type{kind: KindAny} }

// Null returns the type only null satisfies.
func Null() Type { return Type{kind: KindNull} }

// Bool returns the boolean type.
func Bool() Type { return Type{kind: KindBool} }

// Number returns the numeric type.
func Number() Type { return Type{kind: KindNumber} }

// String returns the string type.
func String() Type { return Type{kind: KindString} }

// Bytes returns the byte-string type.
func Bytes() Type { return Type{kind: KindBytes} }

// List returns the type of a list whose elements are elem.
func List(elem Type) Type { return Type{kind: KindList, elem: &elem} }

// Map returns the type of a string-keyed map whose values are elem.
func Map(elem Type) Type { return Type{kind: KindMap, elem: &elem} }

// Record returns the type of a map with a known set of fields. Field order is
// declaration order and is preserved, so diagnostics and editor forms present
// fields the way the author wrote them.
func Record(fields ...Field) Type {
	cp := make([]Field, len(fields))
	copy(cp, fields)
	return Type{kind: KindRecord, fields: cp}
}

// Union returns the type satisfied by any of members.
func Union(members ...Type) Type {
	cp := make([]Type, len(members))
	copy(cp, members)
	return Type{kind: KindUnion, members: cp}
}

// Ref returns a named reference, resolved against a Registry at check time.
func Ref(name string) Type { return Type{kind: KindRef, name: name} }

// Kind returns which member of the union is present.
func (t Type) Kind() Kind { return t.kind }

// Elem returns the element type of a List or Map, or Any for other kinds.
func (t Type) Elem() Type {
	if t.elem == nil {
		return Any()
	}
	return *t.elem
}

// Fields returns a copy of a Record's fields in declaration order.
func (t Type) Fields() []Field {
	cp := make([]Field, len(t.fields))
	copy(cp, t.fields)
	return cp
}

// Members returns a copy of a Union's members.
func (t Type) Members() []Type {
	cp := make([]Type, len(t.members))
	copy(cp, t.members)
	return cp
}

// RefName returns a Ref's name, or "" for other kinds.
func (t Type) RefName() string { return t.name }

// String renders the type for diagnostics.
func (t Type) String() string {
	switch t.kind {
	case KindAny:
		return "Any"
	case KindNull:
		return "Null"
	case KindBool:
		return "Bool"
	case KindNumber:
		return "Number"
	case KindString:
		return "String"
	case KindBytes:
		return "Bytes"
	case KindList:
		return "List<" + t.Elem().String() + ">"
	case KindMap:
		return "Map<" + t.Elem().String() + ">"
	case KindRecord:
		if len(t.fields) == 0 {
			return "{}"
		}
		parts := make([]string, 0, len(t.fields))
		for _, f := range t.fields {
			opt := ""
			if f.Optional {
				opt = "?"
			}
			parts = append(parts, f.Name+opt+": "+f.Type.String())
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	case KindUnion:
		parts := make([]string, 0, len(t.members))
		for _, m := range t.members {
			parts = append(parts, m.String())
		}
		return strings.Join(parts, " | ")
	case KindRef:
		return t.name
	default:
		return "invalid"
	}
}
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./internal/types/ -v`
Expected: PASS.

- [ ] **Step 5: Verify the tests have teeth**

Change `KindAny` so it is no longer the zero constant — insert `kindInvalid Kind = iota` before it.
Expected: `TestZeroTypeIsAny` FAILS. Revert.

- [ ] **Step 6: Commit**

```bash
git add internal/types/type.go internal/types/type_test.go
git commit -m "feat(types): the type language and its rendering

Zero Type is Any, so an unset type is permissive rather than silently
rejecting everything. Record field order is declaration order, which is
what diagnostics and editor forms present."
```

---

### Task 7: Gradual assignability

Spec §3.4. `Assignable` answers "may a value of type `from` be used where `to` is expected." It is what the parameter type check (spec §8.1 check 2) and the binding input check (check 7) will call.

**Files:**
- Create: `internal/types/assignable.go`
- Test: `internal/types/assignable_test.go`

**Interfaces:**
- Consumes: Task 6.
- Produces:
  - `type Registry interface{ Lookup(name string) (Type, bool) }`
  - `type MapRegistry map[string]Type` with `func (r MapRegistry) Lookup(name string) (Type, bool)`
  - `func Assignable(from, to Type, reg Registry) bool`

- [ ] **Step 1: Write the failing test**

Create `internal/types/assignable_test.go`:

```go
package types

import (
	"testing"
	"time"
)

func TestAssignable_AnyIsBidirectional(t *testing.T) {
	// Gradual typing: Any accepts everything and is accepted everywhere. This
	// is what lets an untyped HTTP response body flow without defeating the
	// checks that apply where a shape IS declared.
	concrete := []Type{
		Null(), Bool(), Number(), String(), Bytes(),
		List(String()), Map(Number()),
		Record(Field{Name: "a", Type: String()}),
		Union(String(), Number()),
	}
	for _, c := range concrete {
		if !Assignable(c, Any(), nil) {
			t.Errorf("Assignable(%s, Any) = false, want true", c)
		}
		if !Assignable(Any(), c, nil) {
			t.Errorf("Assignable(Any, %s) = false, want true", c)
		}
	}
}

func TestAssignable_Scalars(t *testing.T) {
	tests := []struct {
		from, to Type
		want     bool
	}{
		{String(), String(), true},
		{Number(), Number(), true},
		{Bool(), Bool(), true},
		{Null(), Null(), true},
		{Bytes(), Bytes(), true},
		{String(), Number(), false},
		{Number(), String(), false},
		{Null(), String(), false},
		{Bool(), Number(), false},
	}
	for _, tc := range tests {
		if got := Assignable(tc.from, tc.to, nil); got != tc.want {
			t.Errorf("Assignable(%s, %s) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestAssignable_ListAndMapAreCovariantInTheirElement(t *testing.T) {
	tests := []struct {
		from, to Type
		want     bool
	}{
		{List(String()), List(String()), true},
		{List(String()), List(Any()), true},
		{List(Any()), List(String()), true},
		{List(String()), List(Number()), false},
		{Map(String()), Map(String()), true},
		{Map(String()), Map(Number()), false},
		{List(String()), Map(String()), false},
		{List(List(String())), List(List(String())), true},
		{List(List(String())), List(List(Number())), false},
	}
	for _, tc := range tests {
		if got := Assignable(tc.from, tc.to, nil); got != tc.want {
			t.Errorf("Assignable(%s, %s) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestAssignable_RecordWidthSubtyping(t *testing.T) {
	narrow := Record(Field{Name: "id", Type: String()})
	wide := Record(
		Field{Name: "id", Type: String()},
		Field{Name: "extra", Type: Number()},
	)
	withOptional := Record(
		Field{Name: "id", Type: String()},
		Field{Name: "note", Type: String(), Optional: true},
	)
	wrongType := Record(Field{Name: "id", Type: Number()})

	// Extra fields are fine: a wider record satisfies a narrower requirement.
	if !Assignable(wide, narrow, nil) {
		t.Error("Assignable(wide, narrow) = false, want true (width subtyping)")
	}
	// Missing a required field is not.
	if Assignable(narrow, wide, nil) {
		t.Error("Assignable(narrow, wide) = true, want false (missing required field)")
	}
	// A missing optional field is fine.
	if !Assignable(narrow, withOptional, nil) {
		t.Error("Assignable(narrow, withOptional) = false, want true (optional may be absent)")
	}
	// A present field of the wrong type is not.
	if Assignable(wrongType, narrow, nil) {
		t.Error("Assignable(wrongType, narrow) = true, want false")
	}
	// A record is assignable to a Map of a compatible element type.
	if !Assignable(narrow, Map(String()), nil) {
		t.Error("Assignable({id: String}, Map<String>) = false, want true")
	}
	if Assignable(wide, Map(String()), nil) {
		t.Error("Assignable({id: String, extra: Number}, Map<String>) = true, want false")
	}
}

func TestAssignable_Unions(t *testing.T) {
	su := Union(String(), Number())

	// TO a union: it is enough to match one member.
	if !Assignable(String(), su, nil) {
		t.Error("Assignable(String, String|Number) = false, want true")
	}
	if Assignable(Bool(), su, nil) {
		t.Error("Assignable(Bool, String|Number) = true, want false")
	}

	// FROM a union: every member must be assignable.
	if !Assignable(su, Union(String(), Number(), Bool()), nil) {
		t.Error("Assignable(String|Number, String|Number|Bool) = false, want true")
	}
	if Assignable(su, String(), nil) {
		t.Error("Assignable(String|Number, String) = true, want false (Number is not)")
	}
	if !Assignable(Union(String(), String()), String(), nil) {
		t.Error("Assignable(String|String, String) = false, want true")
	}
}

func TestAssignable_RefsResolveThroughTheRegistry(t *testing.T) {
	reg := MapRegistry{
		"Article": Record(Field{Name: "slug", Type: String()}),
		"Alias":   Ref("Article"),
	}

	if !Assignable(Ref("Article"), Record(Field{Name: "slug", Type: String()}), reg) {
		t.Error("Assignable(Ref(Article), {slug: String}) = false, want true")
	}
	if !Assignable(Record(Field{Name: "slug", Type: String()}), Ref("Article"), reg) {
		t.Error("Assignable({slug: String}, Ref(Article)) = false, want true")
	}
	// A ref chain resolves.
	if !Assignable(Ref("Alias"), Ref("Article"), reg) {
		t.Error("Assignable(Ref(Alias), Ref(Article)) = false, want true")
	}
	// An unresolvable ref is not assignable to anything concrete.
	if Assignable(Ref("Missing"), String(), reg) {
		t.Error("Assignable(Ref(Missing), String) = true, want false")
	}
	// A nil registry cannot resolve any ref.
	if Assignable(Ref("Article"), String(), nil) {
		t.Error("Assignable with a nil registry resolved a ref")
	}
}

func TestAssignable_SelfReferentialTypeTerminates(t *testing.T) {
	// A recursive type must not hang the checker.
	reg := MapRegistry{
		"Node": Record(
			Field{Name: "value", Type: String()},
			Field{Name: "next", Type: Ref("Node"), Optional: true},
		),
	}
	done := make(chan bool, 1)
	go func() { done <- Assignable(Ref("Node"), Ref("Node"), reg) }()

	select {
	case got := <-done:
		if !got {
			t.Error("Assignable(Ref(Node), Ref(Node)) = false, want true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Assignable did not terminate on a recursive type")
	}
}

func TestAssignable_DirectlyCyclicRefTerminates(t *testing.T) {
	reg := MapRegistry{"Loop": Ref("Loop")}
	// Must return rather than recurse forever. The value is unimportant; the
	// return is the assertion.
	_ = Assignable(Ref("Loop"), String(), reg)
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/types/ -run TestAssignable -v`
Expected: FAIL — `undefined: Assignable`

- [ ] **Step 3: Write the implementation**

Create `internal/types/assignable.go`:

```go
package types

// Registry resolves named type references.
type Registry interface {
	Lookup(name string) (Type, bool)
}

// MapRegistry is a Registry backed by a map.
type MapRegistry map[string]Type

// Lookup implements Registry.
func (r MapRegistry) Lookup(name string) (Type, bool) {
	t, ok := r[name]
	return t, ok
}

// Assignable reports whether a value of type from may be used where to is
// expected.
//
// The rules:
//   - Any is assignable to and from everything (gradual typing).
//   - Scalars match their own kind.
//   - List and Map are covariant in their element type. Covariance is sound
//     here because values are immutable — there is no write path through which
//     a narrower element could be stored.
//   - A Record satisfies another Record if it supplies every required field at
//     an assignable type. Extra fields are allowed (width subtyping).
//   - A Record satisfies Map<E> if every field type is assignable to E.
//   - Assigning TO a Union needs one matching member; assigning FROM a Union
//     needs every member to be assignable.
//   - A Ref resolves through reg. An unresolvable Ref is assignable only to Any.
//
// Recursive and cyclic types terminate: each (from, to) pair is visited once.
func Assignable(from, to Type, reg Registry) bool {
	return assignable(from, to, reg, make(map[pair]bool))
}

type pair struct{ from, to string }

func assignable(from, to Type, reg Registry, seen map[pair]bool) bool {
	// Any short-circuits before anything else, including ref resolution.
	if from.kind == KindAny || to.kind == KindAny {
		return true
	}

	// Guard against recursive types. Assuming true on re-entry is the standard
	// coinductive treatment: a cycle is accepted unless some finite path
	// refutes it.
	key := pair{from: from.String(), to: to.String()}
	if seen[key] {
		return true
	}
	seen[key] = true

	// Resolve references.
	if from.kind == KindRef {
		resolved, ok := lookup(from.name, reg)
		if !ok {
			return false
		}
		return assignable(resolved, to, reg, seen)
	}
	if to.kind == KindRef {
		resolved, ok := lookup(to.name, reg)
		if !ok {
			return false
		}
		return assignable(from, resolved, reg, seen)
	}

	// FROM a union: every member must work. Checked before the TO-union case so
	// that union-to-union requires each source member to match some target one.
	if from.kind == KindUnion {
		for _, m := range from.Members() {
			if !assignable(m, to, reg, seen) {
				return false
			}
		}
		return true
	}

	// TO a union: one member is enough.
	if to.kind == KindUnion {
		for _, m := range to.Members() {
			if assignable(from, m, reg, seen) {
				return true
			}
		}
		return false
	}

	switch to.kind {
	case KindNull, KindBool, KindNumber, KindString, KindBytes:
		return from.kind == to.kind

	case KindList:
		if from.kind != KindList {
			return false
		}
		return assignable(from.Elem(), to.Elem(), reg, seen)

	case KindMap:
		switch from.kind {
		case KindMap:
			return assignable(from.Elem(), to.Elem(), reg, seen)
		case KindRecord:
			for _, f := range from.Fields() {
				if !assignable(f.Type, to.Elem(), reg, seen) {
					return false
				}
			}
			return true
		default:
			return false
		}

	case KindRecord:
		if from.kind != KindRecord {
			return false
		}
		have := make(map[string]Field, len(from.fields))
		for _, f := range from.Fields() {
			have[f.Name] = f
		}
		for _, want := range to.Fields() {
			got, present := have[want.Name]
			if !present {
				if want.Optional {
					continue
				}
				return false
			}
			if !assignable(got.Type, want.Type, reg, seen) {
				return false
			}
		}
		return true

	default:
		return false
	}
}

func lookup(name string, reg Registry) (Type, bool) {
	if reg == nil {
		return Type{}, false
	}
	return reg.Lookup(name)
}
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./internal/types/ -v`
Expected: PASS.

- [ ] **Step 5: Verify the tests have teeth**

Two mutations, each reverted after observing the failure:

1. In the `KindRecord` case, change `if want.Optional { continue }` to `return false`.
   Expected: `TestAssignable_RecordWidthSubtyping` FAILS on the optional-field case.
2. Move the `TO a union` block above the `FROM a union` block.
   Expected: `TestAssignable_Unions` FAILS on `Assignable(String|Number, String)`, which would wrongly become true.

- [ ] **Step 6: Commit**

```bash
git add internal/types/assignable.go internal/types/assignable_test.go
git commit -m "feat(types): gradual assignability

Any is bidirectional; records use width subtyping; unions are directional
(one member to, every member from); refs resolve through a registry and
recursive types terminate by visiting each pair once."
```

---

### Task 8: Checking a `Value` against a `Type`

Spec §3.4 and §8.1 check 7 — the runtime half. Where `Assignable` compares two types at compile time, `Check` validates an actual value at a boundary, and reports *where* it failed.

**Files:**
- Create: `internal/types/check.go`
- Test: `internal/types/check_test.go`

**Interfaces:**
- Consumes: Task 6, Task 7, and `internal/value` (Tasks 1–5).
- Produces:
  - `type CheckError struct{ Path string; Want Type; Got string; Detail string }` with `func (e *CheckError) Error() string`
  - `func Check(v value.Value, t Type, reg Registry) error`

- [ ] **Step 1: Write the failing test**

Create `internal/types/check_test.go`:

```go
package types

import (
	"errors"
	"strings"
	"testing"

	"github.com/chimpanze/runtime/internal/value"
)

func TestCheck_ScalarsAndAny(t *testing.T) {
	tests := []struct {
		name string
		v    value.Value
		t    Type
		ok   bool
	}{
		{"null vs Null", value.Null(), Null(), true},
		{"null vs String", value.Null(), String(), false},
		{"bool vs Bool", value.OfBool(true), Bool(), true},
		{"bool vs String", value.OfBool(true), String(), false},
		{"number vs Number", value.OfInt(1), Number(), true},
		{"string vs String", value.OfString("x"), String(), true},
		{"bytes vs Bytes", value.OfBytes([]byte{1}), Bytes(), true},
		{"anything vs Any", value.OfList(value.NewList()), Any(), true},
		{"null vs Any", value.Null(), Any(), true},
	}
	for _, tc := range tests {
		err := Check(tc.v, tc.t, nil)
		if tc.ok && err != nil {
			t.Errorf("%s: Check = %v, want nil", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s: Check = nil, want error", tc.name)
		}
	}
}

func TestCheck_ListElements(t *testing.T) {
	good := value.OfList(value.NewList(value.OfString("a"), value.OfString("b")))
	if err := Check(good, List(String()), nil); err != nil {
		t.Errorf("Check(good list) = %v, want nil", err)
	}

	bad := value.OfList(value.NewList(value.OfString("a"), value.OfInt(2)))
	err := Check(bad, List(String()), nil)
	if err == nil {
		t.Fatal("Check(bad list) = nil, want error")
	}

	var ce *CheckError
	if !errors.As(err, &ce) {
		t.Fatalf("error is %T, want *CheckError", err)
	}
	if ce.Path != "[1]" {
		t.Errorf("Path = %q, want %q", ce.Path, "[1]")
	}
}

func TestCheck_MapValues(t *testing.T) {
	good := value.OfMap(value.NewMapBuilder().
		Set("a", value.OfInt(1)).
		Set("b", value.OfInt(2)).
		Build())
	if err := Check(good, Map(Number()), nil); err != nil {
		t.Errorf("Check(good map) = %v, want nil", err)
	}

	bad := value.OfMap(value.NewMapBuilder().
		Set("a", value.OfInt(1)).
		Set("b", value.OfString("nope")).
		Build())
	err := Check(bad, Map(Number()), nil)
	if err == nil {
		t.Fatal("Check(bad map) = nil, want error")
	}
	var ce *CheckError
	if !errors.As(err, &ce) {
		t.Fatalf("error is %T, want *CheckError", err)
	}
	if ce.Path != "b" {
		t.Errorf("Path = %q, want %q", ce.Path, "b")
	}
}

func TestCheck_RecordFields(t *testing.T) {
	rec := Record(
		Field{Name: "id", Type: String()},
		Field{Name: "count", Type: Number(), Optional: true},
	)

	t.Run("required present, optional absent", func(t *testing.T) {
		v := value.OfMap(value.NewMapBuilder().Set("id", value.OfString("x")).Build())
		if err := Check(v, rec, nil); err != nil {
			t.Errorf("Check = %v, want nil", err)
		}
	})

	t.Run("extra fields are allowed", func(t *testing.T) {
		v := value.OfMap(value.NewMapBuilder().
			Set("id", value.OfString("x")).
			Set("unexpected", value.OfBool(true)).
			Build())
		if err := Check(v, rec, nil); err != nil {
			t.Errorf("Check = %v, want nil (width subtyping)", err)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		v := value.OfMap(value.NewMapBuilder().Set("count", value.OfInt(1)).Build())
		err := Check(v, rec, nil)
		if err == nil {
			t.Fatal("Check = nil, want error")
		}
		if !strings.Contains(err.Error(), "id") {
			t.Errorf("error %q does not name the missing field", err)
		}
	})

	t.Run("wrong field type reports a nested path", func(t *testing.T) {
		nested := Record(Field{
			Name: "outer",
			Type: Record(Field{Name: "inner", Type: String()}),
		})
		v := value.OfMap(value.NewMapBuilder().
			Set("outer", value.OfMap(value.NewMapBuilder().
				Set("inner", value.OfInt(1)).
				Build())).
			Build())
		err := Check(v, nested, nil)
		if err == nil {
			t.Fatal("Check = nil, want error")
		}
		var ce *CheckError
		if !errors.As(err, &ce) {
			t.Fatalf("error is %T, want *CheckError", err)
		}
		if ce.Path != "outer.inner" {
			t.Errorf("Path = %q, want %q", ce.Path, "outer.inner")
		}
	})

	t.Run("non-map against a record", func(t *testing.T) {
		if err := Check(value.OfString("x"), rec, nil); err == nil {
			t.Error("Check(string, record) = nil, want error")
		}
	})
}

func TestCheck_Unions(t *testing.T) {
	u := Union(String(), Number())
	if err := Check(value.OfString("x"), u, nil); err != nil {
		t.Errorf("Check(string, String|Number) = %v, want nil", err)
	}
	if err := Check(value.OfInt(1), u, nil); err != nil {
		t.Errorf("Check(number, String|Number) = %v, want nil", err)
	}
	err := Check(value.OfBool(true), u, nil)
	if err == nil {
		t.Fatal("Check(bool, String|Number) = nil, want error")
	}
	if !strings.Contains(err.Error(), "String | Number") {
		t.Errorf("error %q does not render the expected union", err)
	}
}

func TestCheck_Refs(t *testing.T) {
	reg := MapRegistry{"Article": Record(Field{Name: "slug", Type: String()})}

	good := value.OfMap(value.NewMapBuilder().Set("slug", value.OfString("a")).Build())
	if err := Check(good, Ref("Article"), reg); err != nil {
		t.Errorf("Check = %v, want nil", err)
	}

	bad := value.OfMap(value.NewMapBuilder().Set("slug", value.OfInt(1)).Build())
	if err := Check(bad, Ref("Article"), reg); err == nil {
		t.Error("Check = nil, want error")
	}

	if err := Check(good, Ref("Missing"), reg); err == nil {
		t.Error("Check against an unresolvable ref = nil, want error")
	}
	if err := Check(good, Ref("Article"), nil); err == nil {
		t.Error("Check with a nil registry resolved a ref")
	}
}

func TestCheckError_MessageNamesPathWantAndGot(t *testing.T) {
	// A diagnostic missing any of the three forces the reader to reproduce it.
	v := value.OfMap(value.NewMapBuilder().Set("id", value.OfInt(1)).Build())
	err := Check(v, Record(Field{Name: "id", Type: String()}), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{"id", "String", "number"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/types/ -run TestCheck -v`
Expected: FAIL — `undefined: Check`

- [ ] **Step 3: Write the implementation**

Create `internal/types/check.go`:

```go
package types

import (
	"fmt"

	"github.com/chimpanze/runtime/internal/value"
)

// CheckError reports a value that does not satisfy a type, and where.
//
// Path is a dotted path from the checked root — "outer.inner", "[1]",
// "rows[0].id" — so a diagnostic can point at the offending leaf rather than
// the whole document. In the reference implementation this had to be
// retrofitted after a nested resolution bug shipped without it.
type CheckError struct {
	Path   string
	Want   Type
	Got    string
	Detail string
}

func (e *CheckError) Error() string {
	where := e.Path
	if where == "" {
		where = "value"
	}
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", where, e.Detail)
	}
	return fmt.Sprintf("%s: expected %s, got %s", where, e.Want, e.Got)
}

// Check reports whether v satisfies t, returning a *CheckError naming the path
// of the first failure.
//
// Where Assignable compares two types before anything runs, Check validates an
// actual value at a boundary — a trigger's input mapping, a node parameter, a
// declared output.
func Check(v value.Value, t Type, reg Registry) error {
	return check(v, t, reg, "")
}

func check(v value.Value, t Type, reg Registry, path string) error {
	switch t.Kind() {
	case KindAny:
		return nil

	case KindRef:
		resolved, ok := lookup(t.RefName(), reg)
		if !ok {
			return &CheckError{Path: path, Want: t, Got: v.Kind().String(),
				Detail: fmt.Sprintf("unknown type reference %q", t.RefName())}
		}
		return check(v, resolved, reg, path)

	case KindUnion:
		for _, m := range t.Members() {
			if check(v, m, reg, path) == nil {
				return nil
			}
		}
		return &CheckError{Path: path, Want: t, Got: v.Kind().String()}

	case KindNull:
		if v.Kind() != value.KindNull {
			return &CheckError{Path: path, Want: t, Got: v.Kind().String()}
		}
		return nil

	case KindBool:
		if v.Kind() != value.KindBool {
			return &CheckError{Path: path, Want: t, Got: v.Kind().String()}
		}
		return nil

	case KindNumber:
		if v.Kind() != value.KindNumber {
			return &CheckError{Path: path, Want: t, Got: v.Kind().String()}
		}
		return nil

	case KindString:
		if v.Kind() != value.KindString {
			return &CheckError{Path: path, Want: t, Got: v.Kind().String()}
		}
		return nil

	case KindBytes:
		if v.Kind() != value.KindBytes {
			return &CheckError{Path: path, Want: t, Got: v.Kind().String()}
		}
		return nil

	case KindList:
		l, ok := v.List()
		if !ok {
			return &CheckError{Path: path, Want: t, Got: v.Kind().String()}
		}
		for i := 0; i < l.Len(); i++ {
			if err := check(l.At(i), t.Elem(), reg, indexPath(path, i)); err != nil {
				return err
			}
		}
		return nil

	case KindMap:
		m, ok := v.Map()
		if !ok {
			return &CheckError{Path: path, Want: t, Got: v.Kind().String()}
		}
		for _, k := range m.Keys() {
			mv, _ := m.Get(k)
			if err := check(mv, t.Elem(), reg, fieldPath(path, k)); err != nil {
				return err
			}
		}
		return nil

	case KindRecord:
		m, ok := v.Map()
		if !ok {
			return &CheckError{Path: path, Want: t, Got: v.Kind().String()}
		}
		for _, f := range t.Fields() {
			fv, present := m.Get(f.Name)
			if !present {
				if f.Optional {
					continue
				}
				return &CheckError{Path: fieldPath(path, f.Name), Want: f.Type, Got: "absent",
					Detail: fmt.Sprintf("missing required field %q", f.Name)}
			}
			if err := check(fv, f.Type, reg, fieldPath(path, f.Name)); err != nil {
				return err
			}
		}
		// Extra fields are allowed: width subtyping, matching Assignable.
		return nil

	default:
		return &CheckError{Path: path, Want: t, Got: v.Kind().String(),
			Detail: "invalid type"}
	}
}

func fieldPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func indexPath(path string, i int) string {
	return fmt.Sprintf("%s[%d]", path, i)
}
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `go test ./... -v`
Expected: PASS across both packages.

- [ ] **Step 5: Verify the tests have teeth**

Two mutations, each reverted after observing the failure:

1. In `check`'s `KindList` case, replace `indexPath(path, i)` with `path`.
   Expected: `TestCheck_ListElements` FAILS — `Path` is `""` rather than `[1]`.
2. In `check`'s `KindRecord` case, delete the `if !present` block so a missing required field is skipped.
   Expected: `TestCheck_RecordFields/missing_required_field` FAILS.

- [ ] **Step 6: Run the full gate**

```bash
go build ./...
go vet ./...
go test ./... -count=1
go test ./... -count=5   # map-iteration determinism does not appear over one run
```

Expected: all clean, all passing. The `-count=5` run matters: in the reference implementation a byte-exact assertion passed for weeks before failing under repeated runs, because Go seeds map iteration per process.

- [ ] **Step 7: Commit**

```bash
git add internal/types/check.go internal/types/check_test.go
git commit -m "feat(types): check a Value against a Type with a failure path

CheckError names the path, the expected type and what was found, so a
diagnostic can point at the offending leaf. Record checking allows extra
fields, matching Assignable's width subtyping."
```

---

## Verification

After Task 8, the following must all hold. Run them before declaring stage 1 complete.

```bash
go build ./...
go vet ./...
go test ./... -count=5
go test ./... -cover        # expect >90% for both packages; these are pure functions
```

Manual confirmation that the layer does the one thing it exists for:

```go
v, _ := value.DecodeJSON([]byte(`{"id":12345678901234567890}`))
m, _ := v.Map()
id, _ := m.Get("id")
s, _ := id.ToString()
// s == "12345678901234567890", not "1.2345678901234567e+19"
```

## What stage 1 deliberately does not include

- **Source positions.** Spec §8 requires every value to carry `Position(file, line, col, path)` so diagnostics are precise. That belongs to the `Document` tree in the pipeline (stage 4), not to `Value` — attaching positions here would make every value carry provenance it does not need, and would break `Equal`.
- **Expression parsing.** Stage 2.
- **Percent-encoding and header canonicalisation.** The HTTP trigger driver's job, stage 7.
- **Anything touching a database, a network, or a file.** These two packages have no I/O.
