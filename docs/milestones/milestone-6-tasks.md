# Milestone 6: Transform and Utility Nodes — Task Breakdown

**Depends on:** Milestone 4 (workflow engine)
**Result:** All `transform.*` and `util.*` nodes work. Data manipulation, schema validation, logging, UUID generation, delays, and timestamps are available in workflows.

---

## Task 6.1: Transform Node Plugin Shell

**Description:** Create the core transform and utility node plugins.

**Subtasks:**

- [ ] Create `plugins/core/transform/plugin.go`:
  - Name: `"core.transform"`, Prefix: `"transform"`
  - HasServices: false
  - Nodes: `transform.set`, `transform.map`, `transform.filter`, `transform.merge`, `transform.delete`, `transform.validate`
- [ ] Create `plugins/core/util/plugin.go`:
  - Name: `"core.util"`, Prefix: `"util"`
  - HasServices: false
  - Nodes: `util.log`, `util.uuid`, `util.delay`, `util.timestamp`
- [ ] Register both plugins during startup

**Tests:**
- [ ] Plugins register with correct prefixes
- [ ] All node types are retrievable from the registry

**Acceptance criteria:** Transform and utility plugins are registered.

---

## Task 6.2: `transform.set` Node

**Description:** Create or overwrite fields using a field mapping.

**Subtasks:**

- [ ] Create `plugins/core/transform/set.go`
- [ ] ConfigSchema: `fields` (required, map of string expressions)
- [ ] Outputs: `["success", "error"]`
- [ ] Execute: resolve each expression in `fields` map, produce output object with key-value results
- [ ] If any expression fails → error

**Tests:**
- [ ] Simple field mapping produces correct output
- [ ] Multiple fields all resolved
- [ ] Expression referencing upstream node output works
- [ ] String interpolation in field values
- [ ] Expression failure → error output

**Acceptance criteria:** Field mapping creates new data objects from expressions.

---

## Task 6.3: `transform.map` Node

**Description:** Apply an expression to each item in a collection.

**Subtasks:**

- [ ] Create `plugins/core/transform/map.go`
- [ ] ConfigSchema: `collection` (required expression → array), `expression` (required expression using `$item`, `$index`)
- [ ] Outputs: `["success", "error"]`
- [ ] Execute: resolve `collection`, for each item evaluate `expression` with `$item` and `$index` in scope, return new array
- [ ] Preserve original array order

**Tests:**
- [ ] Map extracts a field: `{{ $item.name }}` → array of names
- [ ] Map computes values: `{{ $item.price * $item.quantity }}`
- [ ] `$index` is available and correct
- [ ] Empty collection → empty array
- [ ] Expression failure on one item → error

**Acceptance criteria:** Collections are transformed element-by-element.

---

## Task 6.4: `transform.filter` Node

**Description:** Filter a collection by a predicate expression.

**Subtasks:**

- [ ] Create `plugins/core/transform/filter.go`
- [ ] ConfigSchema: `collection` (required expression → array), `expression` (required expression → bool, using `$item`, `$index`)
- [ ] Outputs: `["success", "error"]`
- [ ] Execute: resolve `collection`, for each item evaluate `expression`, keep items where result is truthy
- [ ] Preserve original order of kept items

**Tests:**
- [ ] Filter by condition: `{{ $item.age >= 18 }}` keeps adults
- [ ] All items pass → original array
- [ ] No items pass → empty array
- [ ] Empty collection → empty array
- [ ] `$index` available in predicate
- [ ] Expression failure → error

**Acceptance criteria:** Collections are filtered by a predicate.

---

## Task 6.5: `transform.merge` Node

**Description:** Combine data from multiple sources with configurable merge strategy.

**Subtasks:**

- [ ] Create `plugins/core/transform/merge.go`
- [ ] ConfigSchema:
  - `mode` (required, static: `"append"` | `"match"` | `"position"`)
  - `inputs` (required, array of expressions)
  - `match` (required when mode=`"match"`): `{ "type": "inner"|"outer"|"enrich", "fields": { "left": string, "right": string } }`
- [ ] **Append mode:** concatenate all input arrays into one
- [ ] **Match mode (inner):** keep rows where left[field] == right[field], merge fields
- [ ] **Match mode (outer):** keep all rows from both, merge matching ones, null-fill non-matching
- [ ] **Match mode (enrich):** keep all rows from left, add matching right data, null-fill non-matching
- [ ] **Position mode:** merge by index — row 0 from each input combined
- [ ] Validate: `match` requires exactly 2 inputs. Validate `mode` is static at startup.

**Tests:**
- [ ] Append: `[1,2]` + `[3,4]` → `[1,2,3,4]`
- [ ] Append: 3 inputs concatenated
- [ ] Match inner: only matching rows kept
- [ ] Match outer: all rows kept, nulls where no match
- [ ] Match enrich: all left rows, matched right data added
- [ ] Position: row-by-row merge
- [ ] Position with different lengths → error
- [ ] Match with more than 2 inputs → error
- [ ] Static validation rejects expression in `mode`

**Acceptance criteria:** All three merge strategies work correctly.

---

## Task 6.6: `transform.delete` Node

**Description:** Remove fields from an object.

**Subtasks:**

- [ ] Create `plugins/core/transform/delete.go`
- [ ] ConfigSchema: `data` (required expression → object), `fields` (required, static string array)
- [ ] Outputs: `["success", "error"]`
- [ ] Execute: resolve `data`, return a copy with named fields removed. Non-existent fields silently ignored.

**Tests:**
- [ ] Remove one field
- [ ] Remove multiple fields
- [ ] Remove non-existent field → no error
- [ ] Original data unchanged (copy returned)
- [ ] Nested object — only top-level fields removed

**Acceptance criteria:** Fields are removed from objects cleanly.

---

## Task 6.7: `transform.validate` Node

**Description:** Validate data against a JSON Schema.

**Subtasks:**

- [ ] Create `plugins/core/transform/validate.go`
- [ ] ConfigSchema: `data` (required expression), `schema` (required, static JSON Schema object or `$ref`)
- [ ] Outputs: `["success", "error"]`
- [ ] Execute: resolve `data`, validate against schema using `santhosh-tekuri/jsonschema/v6`
  - Valid → `success` with data unchanged
  - Invalid → `error` with `ValidationError` containing field-level details
- [ ] Schema is compiled once (at factory creation time), not per execution

**Tests:**
- [ ] Valid data → success
- [ ] Missing required field → error with field name
- [ ] Wrong type → error with field name and expected type
- [ ] Pattern mismatch → error with field and pattern
- [ ] Multiple validation errors collected in one response
- [ ] `$ref` schema resolution works
- [ ] Complex nested schema validation

**Acceptance criteria:** JSON Schema validation with detailed, field-level error reporting.

---

## Task 6.8: `util.log` Node

**Description:** Write a structured log entry.

**Subtasks:**

- [ ] Create `plugins/core/util/log.go`
- [ ] ConfigSchema: `level` (required, static: `"debug"|"info"|"warn"|"error"`), `message` (required expression), `fields` (optional, map of expressions)
- [ ] Outputs: `["success", "error"]`
- [ ] Execute: resolve `message` and `fields`, call `nCtx.Log(level, message, fields)`, return `success` with no data

**Tests:**
- [ ] Log at each level
- [ ] Message with expression interpolation
- [ ] Fields resolved and included
- [ ] No fields → log without fields
- [ ] Output is empty (logging is a side effect)

**Acceptance criteria:** Structured logging from workflows.

---

## Task 6.9: `util.uuid` Node

**Description:** Generate a UUID v4.

**Subtasks:**

- [ ] Create `plugins/core/util/uuid.go`
- [ ] ConfigSchema: none (empty)
- [ ] Outputs: `["success", "error"]`
- [ ] Execute: generate UUID v4 via `google/uuid`, return as string

**Tests:**
- [ ] Produces valid UUID format
- [ ] Each invocation produces a different UUID

**Acceptance criteria:** UUID generation available in workflows.

---

## Task 6.10: `util.delay` Node

**Description:** Pause execution for a specified duration.

**Subtasks:**

- [ ] Create `plugins/core/util/delay.go`
- [ ] ConfigSchema: `timeout` (required, static duration string)
- [ ] Outputs: `["success", "error"]`
- [ ] Execute: parse duration, sleep using `time.After` with `select` on context cancellation
  - Duration elapses → `success` with no data
  - Context expires first → `error` with `TimeoutError`

**Tests:**
- [ ] Delay of 100ms actually waits ~100ms
- [ ] Context cancellation interrupts the delay
- [ ] Duration parsing: `"5s"`, `"100ms"`, `"1m"`
- [ ] Invalid duration → error at startup (static validation)

**Acceptance criteria:** Delays work with context-aware cancellation.

---

## Task 6.11: `util.timestamp` Node

**Description:** Return the current timestamp in a configurable format.

**Subtasks:**

- [ ] Create `plugins/core/util/timestamp.go`
- [ ] ConfigSchema: `format` (optional, static: `"iso8601"|"unix"|"unix_ms"`, default `"iso8601"`)
- [ ] Outputs: `["success", "error"]`
- [ ] Execute:
  - `"iso8601"` → `"2024-01-15T10:30:00Z"` (string)
  - `"unix"` → `1705312200` (int, seconds)
  - `"unix_ms"` → `1705312200000` (int, milliseconds)

**Tests:**
- [ ] Each format produces correct type and shape
- [ ] Default format is iso8601
- [ ] Timestamps are accurate (within 1 second of time.Now)

**Acceptance criteria:** Timestamp generation in multiple formats.

---

## Task 6.12: Integration Tests

**Description:** Workflows combining transform and utility nodes.

**Subtasks:**

- [ ] Test: workflow that validates input, transforms it, and produces output — using validate → set → map
- [ ] Test: filter + merge — filter two lists, merge results
- [ ] Test: loop (from M5) with transform.set inside the sub-workflow
- [ ] Test: util.delay within a workflow with context timeout
- [ ] Test: transform.validate failure triggers error path with field-level details
- [ ] Test: complex data pipeline — validate → filter → map → set → merge in sequence
- [ ] Create workflow test JSON files for all above scenarios

**Acceptance criteria:** Transform and utility nodes work correctly in combination with control flow.
