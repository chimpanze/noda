# Schema `$ref` Name Collision Detection — Design

**Issue:** #405
**Status:** Ready to implement
**Blocks:** OpenAPI generator unification (`docs/superpowers/specs/2026-07-20-openapi-generator-unification-design.md`)

## Problem

`buildSchemaRegistry` (`internal/config/refs.go:41-65`) builds the `$ref` registry by
iterating `rc.Schemas` — a `map[string]map[string]any` — with no duplicate detection.
Two distinct schema definitions can map to the same ref name, and the last writer wins.
Go randomizes map iteration, so **which one wins varies between runs of the same config**.

A route or workflow can therefore validate incoming data against a different schema on
different boots, with no error, warning, or log line. If the two schemas differ in
strictness, this is a silent validation bypass.

### Verified: two collision modes

Confirmed by calling `buildSchemaRegistry` directly, 500 builds per mode in one process:

```
keyed-vs-keyed (schemas/a.json{User} + schemas/b.json{User}):  map[FROM_A:53 FROM_B:447]
bare-vs-keyed  (schemas/User.json + schemas/other.json{User}): map[BARE:78 KEYED:422]
```

1. **Same top-level key in two files in the same directory** — each non-bare schema file
   registers every top-level key as `<reldir>/<key>` (`refs.go:56-61`).
2. **Bare-schema file vs a keyed entry** — a file that is itself a JSON Schema registers
   whole under its filename (`refs.go:50-53`), which can equal a key another file
   contributes.

In both cases the registry ends with exactly one entry where the author declared two.

### Verified: a third, undocumented failure in the same family

`isBareSchema` (`refs.go:70-77`) checks only for keyword **presence**, ignoring value
shape. A keyed file whose schema names happen to be lowercase JSON Schema keywords is
misclassified as a bare schema, and every definition in it silently vanishes:

```
{"Type": {...}, "Items": {...}}  →  [schemas/Type, schemas/Items]   correct
{"type": {...}, "items": {...}}  →  [schemas/domain]                both definitions lost
```

Deterministic rather than random, but it is the same root cause the issue names — the
naming authority is ambiguous — and the OpenAPI generator inherits it identically.
In scope for this change.

### Not affected

Directory qualification already works. `extractSchemasRelPath` (`refs.go:82-90`) keys by
directory, so `schemas/billing/Invoice.json` and `schemas/orders/Invoice.json` register
distinctly as `schemas/billing/Invoice` and `schemas/orders/Invoice`.

## Why a hard error is the right resolution

**No legitimate override path exists.** `discovery.go:57-82` scans exactly one `schemas/`
tree under one project root. The overlay mechanism (`noda.<env>.json`) applies only to the
root config and never to schema files. There is no "environment overrides base schema"
use case to preserve, so a collision is *always* author error. This rules out the
alternative of sorting deterministically and keeping last-writer-wins.

**No existing config breaks.** Every `schemas/` tree in `examples/`, `testdata/`,
`projects/`, and `internal/` was scanned with the exact registry-key algorithm:
**20 files, 0 collisions.** No migration is required and no shipped example changes.

## Design

### 1. Collision detection

`BuildSchemaRegistry` tracks, per ref name, the list of contributing sources
(file path plus the top-level key, or empty key for a bare-schema file). Any name with
more than one contributor becomes a `ValidationError`.

The error output must itself be deterministic — ref names are sorted, and sources within a
collision are sorted by `(FilePath, Key)`. A nondeterministic error message for a
nondeterminism bug would be its own defect.

Error shape:

- `FilePath` — the alphabetically first contributing file
- `JSONPath` — `/<key>` for a keyed contributor, empty for a bare-schema file
- `Message` — names the ref, the contributor count, and every contributor, plus the
  two ways out (rename a definition, or move one file to a subdirectory, since the
  directory is part of the ref name)

### 2. Shape-aware bare-schema classification

Replace presence checks with a three-way classification:

1. **Decidably bare** — a keyword is present *with the shape that keyword actually takes
   in a schema document*: `$schema`/`$ref` string, `type` string or array,
   `enum`/`oneOf`/`anyOf`/`allOf` array. A `type` whose value is an object is a schema
   *named* `type`, not the `type` keyword.
2. **Decidably keyed** — a keyword is present with the *wrong* shape. This is positive
   proof, not merely absence of evidence: a top-level `"type": {...}` cannot be the
   `type` keyword in any valid schema document, so the file cannot be a bare schema at
   all, and that conclusion holds regardless of what else the file contains.
   *(Corrected during implementation — the original design treated a wrong-shaped
   keyword as neutral, which classified `{"type": {...}, "properties": {...}}` as
   ambiguous even though it is fully decidable, reintroducing exactly the
   definitions-silently-lost bug this change exists to fix.)*
3. **Ambiguous** — no `bareSchemaKeywords` key present at all, but `properties` or
   `items` is present at top level. Both take object values in a real schema *and* as a definition name, so shape
   cannot separate them. This is exactly the "bind silently to an arbitrary
   interpretation" outcome the issue condemns, so it becomes a `ValidationError` asking
   the author to disambiguate (add `"type"` for a bare schema; rename the definition
   otherwise).
4. **Keyed** — everything else.

**Verified behavior-preserving:** all 20 in-repo schema files classify identically under
the old presence rule and the new decidable rule, and **0** fall into the new ambiguous
bucket.

### 3. `ResolveRefs` returns `[]ValidationError`

The issue asks for errors carrying `FilePath`/`JSONPath` "like other config validation",
but `ResolveRefs` returns `[]error` and `pipeline.go:88-95` wraps them setting **only**
`Message`. `FilePath` is dropped and `ValidationError.Error()` renders a leading `": "`.

`ResolveRefs` changes to return `[]ValidationError`. The redundant `in %s` / `(in %s)`
suffixes come out of the existing unresolved-ref and circular-ref messages and become
`FilePath` instead. Call sites: 1 in production (`pipeline.go:88`), 12 in tests — all
compile unchanged, because `assert.Empty` works on any slice and `errs[0].Error()` is
valid on an addressable slice element despite the pointer receiver.

**Deliberately out of scope:** `JSONPath` for unresolved/circular `$ref` errors.
`resolveRefsInValue` does not thread a JSON pointer through its recursion, and adding one
is a separate change. Only collision and ambiguity errors set `JSONPath`.

### 4. Export the registry

`buildSchemaRegistry` is currently called once and discarded, and
`ResolvedConfig.Schemas` remains keyed by **file path** — which is precisely the
`openapi.go:37` vs `openapi.go:246` dangling-`$ref` bug the unification spec describes.
Making names unambiguous unblocks nothing unless the registry survives the pipeline.

`BuildSchemaRegistry` becomes exported. `ResolveRefs` stores its result on a new
`RawConfig.SchemaRegistry` field (keeping the single-return arity, so no test churn), and
`ValidateAll` copies it to a new `ResolvedConfig.SchemaRegistry`.

## Behavior changes

| Config | Before | After |
|---|---|---|
| Two files contributing the same ref name | Nondeterministic winner, silent | `ValidationError` at validate / dry-run / boot |
| Keyed file with a lowercase-keyword definition name (`{"type": {...}}`) | Whole file registers as one schema; definitions lost, silent | Classified keyed; definitions register correctly |
| File with top-level `properties`/`items` and no `type`/`$schema` | Treated as bare | `ValidationError` asking to disambiguate |
| Unresolved / circular `$ref` | `": message (in path)"` | `"path: message"` |

The first three are the intended fix. The fourth is cosmetic — same information, correct
`ValidationError` fields.

## Verification

- Collision detection is proven by running the registry build many times in one process
  and asserting the error appears on **every** iteration — a single build could pass by
  luck under the old code only if it errored, so the repeat loop is what distinguishes
  "detects the collision" from "happened to pick the same winner".
- Full `internal/config` suite plus `go build ./...` and `go vet ./...`.
- The corpus scan is re-run as a guard: all shipped configs must still load clean.
