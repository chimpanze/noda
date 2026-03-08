# Noda — Core Node Catalog

**Version**: 0.4.0
**Status**: Planning

This document specifies every built-in node in Noda. Each entry defines the node's config schema, service dependencies, outputs, and behavior. Field naming and value formats follow the **Config Conventions** document.

---

## 1. Control Flow Nodes

### control.if

Evaluates a condition and routes to one of two branches.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `condition` | expression → bool | Yes | Expression that evaluates to true or false |

**Outputs:** `then`, `else`, `error`

**Behavior:** Resolves `condition`. If truthy, fires `then`. If falsy, fires `else`. If the expression fails to evaluate, fires `error`. The output data is the resolved condition value.

**Service deps:** none

---

### control.switch

Routes to one of N branches based on a value.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `expression` | expression → any | Yes | Expression to evaluate and match |
| `cases` | static string[] | Yes | List of branch names to match against |

**Outputs:** one per case name, plus `default`, `error`

**Behavior:** Resolves `expression`. Compares the result against each case name as a string. If a match is found, fires that output. If no match, fires `default`. If the expression fails, fires `error`. The output data is the resolved expression value.

**Service deps:** none

**Note:** Case names are static string literals. They define the node's output ports and must be known at startup.

---

### control.loop

Iterates a sub-workflow over a collection.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `collection` | expression → array | Yes | Array to iterate over |
| `workflow` | static string | Yes | Sub-workflow ID to invoke per item |
| `input` | map of expressions | Yes | Input mapping for the sub-workflow. `$item` and `$index` available. |

**Outputs:** `done`, `error`

**Behavior:** Resolves `collection` to an array. For each element, invokes the sub-workflow with `$.input` populated from the `input` map (`$item` = current element, `$index` = zero-based index). Iterations run sequentially. Collects the output from each iteration's `workflow.output` node into an array. When all iterations complete, fires `done` with the collected array. If any iteration fails (sub-workflow errors with no error edge), fires `error` immediately — remaining iterations are skipped.

**Service deps:** none

---

## 2. Workflow Nodes

### workflow.run

Invokes a sub-workflow. Outputs are dynamic — determined by the sub-workflow's `workflow.output` nodes.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `workflow` | static string | Yes | Sub-workflow ID |
| `input` | map of expressions | Yes | Input mapping for `$.input` |
| `transaction` | static bool | No | Wrap in database transaction. Default: `false` |

**Outputs:** dynamic — collected from the sub-workflow's `workflow.output` node names, plus `error`

**Behavior:** Resolves `input` expressions and populates the sub-workflow's `$.input`. Executes the sub-workflow. Whichever `workflow.output` node fires determines which output this node emits in the parent, along with that output node's data. If the sub-workflow fails without reaching a `workflow.output`, fires `error`. When `transaction: true`, the `services.database` slot must be filled — the engine wraps execution in a database transaction and swaps the connection for all `db.*` nodes inside.

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `database` | `db` | Only when `transaction: true` |

---

### workflow.output

Declares a named output and return data for a sub-workflow. Terminal node — must have no outbound edges.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | static string | Yes | Output name (becomes a port on the parent's `workflow.run`) |
| `data` | expression → any | No | Data to return to the parent. Default: `null` |

**Outputs:** none (terminal node)

**Behavior:** When reached, the sub-workflow completes with this output name and the resolved `data`. The parent's `workflow.run` node fires the output matching this `name`.

**Service deps:** none

**Constraints:** All `workflow.output` nodes in a sub-workflow must have unique `name` values. They must be on mutually exclusive branches (validated at startup).

---

## 3. Transform Nodes

### transform.set

Creates or overwrites fields on the execution context.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `fields` | map of expressions | Yes | Map of `field_name: expression` |

**Outputs:** `success`, `error`

**Behavior:** Resolves each expression in `fields` and produces an output object with the resulting key-value pairs. If any expression fails to resolve, fires `error`.

**Service deps:** none

**Example:**
```json
{
  "fields": {
    "full_name": "{{ input.first }} {{ input.last }}",
    "is_admin": "{{ input.role == 'admin' }}"
  }
}
```

---

### transform.map

Applies an expression to each item in a collection, producing a new array.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `collection` | expression → array | Yes | Array to transform |
| `expression` | expression → any | Yes | Expression applied per item. `$item` and `$index` available. |

**Outputs:** `success`, `error`

**Behavior:** Resolves `collection` to an array. For each element, evaluates `expression` with `$item` as the current element and `$index` as the index. Produces a new array of the results. Fires `success` with the mapped array.

**Service deps:** none

---

### transform.filter

Filters a collection, keeping items where an expression evaluates to true.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `collection` | expression → array | Yes | Array to filter |
| `expression` | expression → bool | Yes | Predicate applied per item. `$item` and `$index` available. |

**Outputs:** `success`, `error`

**Behavior:** Resolves `collection`. For each element, evaluates `expression`. Keeps items where the result is truthy. Fires `success` with the filtered array.

**Service deps:** none

---

### transform.merge

Combines data from multiple upstream node outputs into a single dataset.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `mode` | static: `"append"`, `"match"`, `"position"` | Yes | Merge strategy |
| `inputs` | expression[] | Yes | Array of expressions, each resolving to a dataset |
| `match` | object | When mode = `"match"` | Match configuration (see below) |
| `match.type` | static: `"inner"`, `"outer"`, `"enrich"` | Yes (in match) | Join type |
| `match.fields` | object | Yes (in match) | `{ "left": "field_name", "right": "field_name" }` |

**Outputs:** `success`, `error`

**Behavior:**

- **append** — concatenates all input arrays into a single array. Works with any number of inputs.
- **match** — joins two inputs by matching field values. Requires exactly two inputs. `inner` keeps only matching rows. `outer` keeps all rows from both. `enrich` keeps all rows from the first input and adds matching data from the second.
- **position** — combines inputs by index. Row 0 from input A merges with row 0 from input B. Requires inputs of equal length.

**Service deps:** none

---

### transform.delete

Removes fields from an object.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `data` | expression → object | Yes | The source object |
| `fields` | static string[] | Yes | Field names to remove |

**Outputs:** `success`, `error`

**Behavior:** Resolves `data` to an object. Returns a copy with the named fields removed. Does not error if a field doesn't exist.

**Service deps:** none

---

### transform.validate

Validates data against a JSON Schema.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `data` | expression → any | Yes | Data to validate |
| `schema` | static object or `$ref` | Yes | JSON Schema to validate against |

**Outputs:** `success`, `error`

**Behavior:** Resolves `data`. Validates it against `schema`. If valid, fires `success` with the data unchanged. If invalid, fires `error` with a `ValidationError` containing field-level details.

**Service deps:** none

---

## 4. Response Nodes

### response.json

Returns an HTTP JSON response. Signals the trigger layer to send the response to the client.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `status` | expression → int | Yes | HTTP status code |
| `body` | expression → any | Yes | Response body (serialized as JSON) |
| `headers` | map of expressions | No | Additional response headers |
| `cookies` | expression → Cookie[] | No | Response cookies |

**Outputs:** `success`, `error`

**Behavior:** Resolves all fields. Produces an `HTTPResponse` object. The trigger layer intercepts this and writes the HTTP response to the client immediately. The node fires `success` after producing the response — downstream nodes continue executing asynchronously.

**Service deps:** none

---

### response.redirect

Returns an HTTP redirect.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `url` | expression → string | Yes | Redirect target URL |
| `status` | static int | No | Redirect status code. Default: `302` |

**Outputs:** `success`, `error`

**Behavior:** Produces an `HTTPResponse` with the given status, a `Location` header set to `url`, and an empty body.

**Service deps:** none

---

### response.error

Returns a standardized error response.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `status` | expression → int | Yes | HTTP status code |
| `code` | expression → string | Yes | Machine-readable error code |
| `message` | expression → string | Yes | Human-readable message |
| `details` | expression → any | No | Additional error details |

**Outputs:** `success`, `error`

**Behavior:** Produces an `HTTPResponse` with the body formatted as the standardized error structure: `{ "error": { "code", "message", "details", "trace_id" } }`. The `trace_id` is automatically injected from the execution context.

**Service deps:** none

---

## 5. Utility Nodes

### util.log

Writes a structured log entry.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `level` | static: `"debug"`, `"info"`, `"warn"`, `"error"` | Yes | Log level |
| `message` | expression → string | Yes | Log message |
| `fields` | map of expressions | No | Structured fields |

**Outputs:** `success`, `error`

**Behavior:** Resolves `message` and `fields`. Writes a structured log entry through Noda's logging pipeline. In dev mode, appears in the live trace. In production, routed via slog to OpenTelemetry. Fires `success` with no data.

**Service deps:** none

---

### util.uuid

Generates a UUID v4.

**Config:** none

**Outputs:** `success`, `error`

**Behavior:** Generates a random UUID v4 string. Fires `success` with the UUID as output data.

**Service deps:** none

---

### util.delay

Pauses execution for a specified duration.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `timeout` | static duration | Yes | How long to wait |

**Outputs:** `success`, `error`

**Behavior:** Waits for the specified duration, respecting the `context.Context` deadline. If the context expires before the delay completes, fires `error` with a `TimeoutError`. Otherwise fires `success` with no data.

**Service deps:** none

---

### util.timestamp

Returns the current timestamp.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `format` | static string | No | Output format. Default: `"iso8601"`. Options: `"iso8601"`, `"unix"`, `"unix_ms"` |

**Outputs:** `success`, `error`

**Behavior:** Produces the current time in the requested format. `"iso8601"` returns a string (`"2024-01-15T10:30:00Z"`). `"unix"` returns seconds as integer. `"unix_ms"` returns milliseconds as integer.

**Service deps:** none

---

## 6. Event Node

### event.emit

Publishes an event to a stream or pubsub service.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `mode` | static: `"stream"`, `"pubsub"` | Yes | Delivery mechanism |
| `topic` | expression → string | Yes | Event topic |
| `payload` | expression → any | Yes | Event data |

**Outputs:** `success`, `error`

**Behavior:** Resolves `topic` and `payload`. Publishes the event to the service matching the configured `mode`. Stream = durable (consumed by workers). PubSub = real-time fan-out. Fires `success` after the event is accepted.

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `stream` | `stream` | Only when mode = `"stream"` |
| `pubsub` | `pubsub` | Only when mode = `"pubsub"` |

---

## 7. File Handling Node

### upload.handle

Processes multipart file uploads with validation and streaming to storage.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `max_size` | static size | Yes | Maximum file size |
| `allowed_types` | static string[] | Yes | Allowed MIME types |
| `max_files` | static int | No | Maximum file count. Default: `1` |
| `path` | expression → string | Yes | Storage path for the uploaded file |

**Outputs:** `success`, `error`

**Behavior:** Reads the file stream from the trigger input (marked via the `files` array on the trigger config). Validates file size and MIME type before fully consuming the stream. Streams the file directly to the destination storage service — no full in-memory buffering. Fires `success` with file metadata: `{ "path", "size", "content_type", "filename" }`. Fires `error` with a `ValidationError` if size or type constraints are violated.

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `destination` | `storage` | Yes |

---

## 8. Real-Time Nodes

### ws.send

Sends a message to clients connected to a Noda WebSocket endpoint.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `channel` | expression → string | Yes | Target channel (supports wildcards) |
| `data` | expression → any | Yes | Message payload |

**Outputs:** `success`, `error`

**Behavior:** Resolves `channel` and `data`. Calls `services["connections"].Send(channel, data)` on the connection manager. The send is buffered — it does not block. Fires `success` with no data.

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `connections` | `ws` | Yes |

---

### sse.send

Sends an event to clients connected to a Noda SSE endpoint.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `channel` | expression → string | Yes | Target channel (supports wildcards) |
| `data` | expression → any | Yes | Event payload |
| `event` | expression → string | No | SSE event type (clients filter with `addEventListener`) |
| `id` | expression → string | No | SSE event ID (for reconnection/replay) |

**Outputs:** `success`, `error`

**Behavior:** Resolves all fields. Calls `services["connections"].SendSSE(channel, event, data, id)`. Buffered, non-blocking. Fires `success` with no data.

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `connections` | `sse` | Yes |

---

## 9. Wasm Interaction Nodes

### wasm.send

Sends data to a running Wasm module. Fire and forget — the workflow does not wait.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `data` | expression → any | Yes | Payload to send to the module |

**Outputs:** `success`, `error`

**Behavior:** Resolves `data`. If the module exports `command`, Noda calls it immediately between ticks. Otherwise, the data is buffered for the next tick's `commands` array. Fires `success` immediately — the workflow does not wait for the module to process the data.

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `runtime` | `wasm` | Yes |

---

### wasm.query

Sends a query to a running Wasm module and waits for a response.

**Config:**

| Field | Type | Required | Description |
|---|---|---|---|
| `data` | expression → any | Yes | Query payload |
| `timeout` | static duration | Yes | Maximum wait time |

**Outputs:** `success`, `error`

**Behavior:** Resolves `data`. Calls the module's `query` export synchronously (serialized with respect to ticks). Waits for the response up to `timeout`. Fires `success` with the module's response data. Fires `error` with `TimeoutError` if the module doesn't respond in time.

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `runtime` | `wasm` | Yes |

---

## 10. Plugin Nodes

Plugin nodes require configured service instances. All have outputs `success` and `error`.

### 10.1 Database Nodes (`db` prefix)

All database nodes share:

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `database` | `db` | Yes |

---

**db.query** — Execute a read query.

| Field | Type | Required | Description |
|---|---|---|---|
| `query` | expression → string | Yes | SQL query |
| `params` | expression[] | No | Parameterized values |

Output: array of row objects.

---

**db.exec** — Execute a write statement (INSERT, UPDATE, DELETE with raw SQL).

| Field | Type | Required | Description |
|---|---|---|---|
| `query` | expression → string | Yes | SQL statement |
| `params` | expression[] | No | Parameterized values |

Output: `{ "rows_affected": int }`.

---

**db.create** — Insert a record.

| Field | Type | Required | Description |
|---|---|---|---|
| `table` | expression → string | Yes | Table name |
| `data` | expression → object | Yes | Row data as key-value map |

Output: the created record (with generated fields like `id`).

---

**db.update** — Update records.

| Field | Type | Required | Description |
|---|---|---|---|
| `table` | expression → string | Yes | Table name |
| `data` | expression → object | Yes | Fields to update |
| `condition` | expression → string | Yes | WHERE clause |
| `params` | expression[] | No | Condition parameters |

Output: `{ "rows_affected": int }`.

---

**db.delete** — Delete records.

| Field | Type | Required | Description |
|---|---|---|---|
| `table` | expression → string | Yes | Table name |
| `condition` | expression → string | Yes | WHERE clause |
| `params` | expression[] | No | Condition parameters |

Output: `{ "rows_affected": int }`.

---

### 10.2 Cache Nodes (`cache` prefix)

All cache nodes share:

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `cache` | `cache` | Yes |

---

**cache.get** — Read a cached value.

| Field | Type | Required |
|---|---|---|
| `key` | expression → string | Yes |

Output: `{ "value": any }`. Fires `error` with `NotFoundError` if the key doesn't exist.

---

**cache.set** — Write a cached value.

| Field | Type | Required |
|---|---|---|
| `key` | expression → string | Yes |
| `value` | expression → any | Yes |
| `ttl` | expression → int | No |

`ttl` in seconds. Omit for no expiration. Output: empty.

---

**cache.del** — Delete a cached value.

| Field | Type | Required |
|---|---|---|
| `key` | expression → string | Yes |

Output: empty.

---

**cache.exists** — Check if a key exists.

| Field | Type | Required |
|---|---|---|
| `key` | expression → string | Yes |

Output: `{ "exists": bool }`.

---

### 10.3 Storage Nodes (`storage` prefix)

All storage nodes share:

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `storage` | `storage` | Yes |

---

**storage.read** — Read a file or object.

| Field | Type | Required |
|---|---|---|
| `path` | expression → string | Yes |

Output: `{ "data": any, "size": int, "content_type": string }`. Fires `error` with `NotFoundError` if the path doesn't exist.

---

**storage.write** — Write a file or object.

| Field | Type | Required |
|---|---|---|
| `path` | expression → string | Yes |
| `data` | expression → any | Yes |
| `content_type` | expression → string | No |

Output: empty.

---

**storage.delete** — Delete a file or object.

| Field | Type | Required |
|---|---|---|
| `path` | expression → string | Yes |

Output: empty.

---

**storage.list** — List files/objects under a prefix.

| Field | Type | Required |
|---|---|---|
| `prefix` | expression → string | Yes |

Output: `{ "paths": string[] }`.

---

### 10.4 Image Nodes (`image` prefix)

All image nodes share:

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `source` | `storage` | Yes |
| `target` | `storage` | Yes |

`source` and `target` can reference the same or different storage instances.

---

**image.resize** — Resize an image.

| Field | Type | Required |
|---|---|---|
| `input` | expression → string | Yes |
| `output` | expression → string | Yes |
| `width` | expression → int | Yes |
| `height` | expression → int | Yes |
| `quality` | expression → int | No |
| `format` | static string | No |

Reads from `source` storage at `input` path, writes to `target` storage at `output` path. Maintains aspect ratio by default. Output: `{ "path", "width", "height", "size" }`.

---

**image.crop** — Crop an image.

| Field | Type | Required |
|---|---|---|
| `input` | expression → string | Yes |
| `output` | expression → string | Yes |
| `width` | expression → int | Yes |
| `height` | expression → int | Yes |
| `gravity` | static string | No |

`gravity`: `"center"`, `"north"`, `"south"`, `"east"`, `"west"`, `"smart"`. Default: `"center"`.

---

**image.watermark** — Add a watermark.

| Field | Type | Required |
|---|---|---|
| `input` | expression → string | Yes |
| `output` | expression → string | Yes |
| `watermark` | expression → string | Yes |
| `opacity` | expression → float | No |
| `position` | static string | No |

`watermark` is a path in `source` storage. `position`: `"center"`, `"top-left"`, `"bottom-right"`, etc.

---

**image.convert** — Convert between formats.

| Field | Type | Required |
|---|---|---|
| `input` | expression → string | Yes |
| `output` | expression → string | Yes |
| `format` | static string | Yes |
| `quality` | expression → int | No |

`format`: `"jpeg"`, `"png"`, `"webp"`, `"avif"`.

---

**image.thumbnail** — Generate a thumbnail.

| Field | Type | Required |
|---|---|---|
| `input` | expression → string | Yes |
| `output` | expression → string | Yes |
| `width` | expression → int | Yes |
| `height` | expression → int | Yes |

Always crops to exact dimensions using smart crop.

---

### 10.5 HTTP Client Nodes (`http` prefix)

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `client` | `http` | Yes |

---

**http.request** — Make an outbound HTTP request.

| Field | Type | Required |
|---|---|---|
| `method` | static string | Yes |
| `url` | expression → string | Yes |
| `headers` | map of expressions | No |
| `body` | expression → any | No |
| `timeout` | static duration | No |

Output: `{ "status": int, "headers": map, "body": any }`.

---

**http.get** — Shorthand for GET request.

| Field | Type | Required |
|---|---|---|
| `url` | expression → string | Yes |
| `headers` | map of expressions | No |
| `timeout` | static duration | No |

Equivalent to `http.request` with `method: "GET"` and no body.

---

**http.post** — Shorthand for POST request.

| Field | Type | Required |
|---|---|---|
| `url` | expression → string | Yes |
| `headers` | map of expressions | No |
| `body` | expression → any | Yes |
| `timeout` | static duration | No |

Equivalent to `http.request` with `method: "POST"`.

---

### 10.6 Email Node (`email` prefix)

**Service deps:**

| Slot | Prefix | Required |
|---|---|---|
| `mailer` | `email` | Yes |

---

**email.send** — Send an email.

| Field | Type | Required |
|---|---|---|
| `to` | expression → string or string[] | Yes |
| `subject` | expression → string | Yes |
| `body` | expression → string | Yes |
| `from` | expression → string | No |
| `cc` | expression → string[] | No |
| `bcc` | expression → string[] | No |
| `reply_to` | expression → string | No |
| `content_type` | static: `"text"`, `"html"` | No |

Default `content_type`: `"html"`. Output: `{ "message_id": string }`.
