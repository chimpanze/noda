# Feature Request: String manipulation functions in expressions

## Problem

Tester needed phone E.164 formatting and PII masking in logs during real-world workflow build. Neither composes cleanly from what's registered today in `internal/expr/functions.go`:

- Registered: `$uuid`, `now`, `lower`, `upper`, `toInt`, `toFloat`, `sha256`, `sha512`, `md5`, `hmac`, `bcrypt_hash`, `bcrypt_verify`, `$var`
- Expr-lang built-ins (already available): `split`, `replace`, `trim`/`trimPrefix`/`trimSuffix`, `repeat`, `hasPrefix`/`hasSuffix`, `contains`, `string[a:b]` slicing

The gap is fixed-width formatting and redaction helpers. Without them, workflows that touch user data have to drop into Wasm for one-liners.

## Proposed functions

| Function | Signature | Purpose |
|---|---|---|
| `padStart` | `(s string, width int, pad string) string` | Left-pad for fixed-width IDs, phone prefixes, timestamps |
| `padEnd` | `(s string, width int, pad string) string` | Right-pad |
| `mask` | `(s string, keepStart int, keepEnd int, with string) string` | Redact middle of a string (emails, tokens, phone tails in logs) |

Rune-aware, not byte-aware (avoid breaking UTF-8 sequences).

## Non-goals

- Phone E.164 formatting — too domain-specific. With `padStart` + slicing + `replace`, workflows can compose it themselves, and anything more should live in Wasm.
- Template/printf-style formatting — out of scope.

## Why not Wasm for this

These are three-line helpers that would otherwise force every project that handles user data to ship a Wasm module for PII hygiene. The cost-per-feature is trivial (same pattern as the existing `upper`/`lower`), and the audit surface is small.

## Implementation notes

- Add to `NewFunctionRegistry()` in `internal/expr/functions.go` alongside `upper`/`lower`.
- Register with description + signature so they appear in the editor's function autocomplete.
- Tests in `internal/expr/functions_test.go` covering empty input, width ≤ len(s), multi-byte pad strings, negative keep counts for `mask`.
