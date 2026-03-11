# Node Reference

All 46 node types available in Noda, organized by plugin. Every node returns a named output (`success` or `error` by default) along with its result data.

## Expression Context

All nodes have access to these variables in expressions:

| Variable | Description |
|----------|-------------|
| `input` | Data passed to the workflow |
| `auth` | Auth data: `user_id`, `roles`, `claims` |
| `trigger` | Trigger metadata: `type`, `timestamp`, `trace_id` |
| `nodes.<id>` | Output data from a previously executed node |
| `$item`, `$index` | Loop iteration variables (inside `control.loop`) |

Built-in functions: `len()`, `lower()`, `upper()`, `now()`, `$uuid()`, `$env()`.

---

## Control Flow

### control.if

Conditional branching based on an expression.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `condition` | string (expr) | yes | Expression to evaluate |

**Outputs:** `then`, `else`, `error`

```json
{
  "type": "control.if",
  "config": {
    "condition": "{{ len(nodes.fetch) > 0 }}"
  }
}
```

### control.switch

Multi-way branching with case matching.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `expression` | string (expr) | yes | Expression to evaluate |
| `cases` | array of strings | yes | Case values to match (static) |

**Outputs:** Each case value + `default`, `error`

```json
{
  "type": "control.switch",
  "config": {
    "expression": "{{ input.action }}",
    "cases": ["create", "update", "delete"]
  }
}
```

### control.loop

Iterates a sub-workflow over each item in a collection.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `collection` | string (expr) | yes | Expression resolving to an array |
| `workflow` | string | yes | Sub-workflow ID to execute per item (static) |
| `input` | object | no | Input template — `$item` and `$index` available |

**Outputs:** `done`, `error`

The `done` output receives an array of all iteration results.

```json
{
  "type": "control.loop",
  "config": {
    "collection": "{{ nodes.fetch }}",
    "workflow": "process-item",
    "input": {
      "item_id": "{{ $item.id }}",
      "index": "{{ $index }}"
    }
  }
}
```

---

## Workflow

### workflow.run

Executes a sub-workflow. Outputs are dynamic — they match the sub-workflow's `workflow.output` node names.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `workflow` | string | yes | Sub-workflow ID (static) |
| `input` | object | no | Input data mapping |
| `transaction` | boolean | no | Wrap in database transaction |

**Service Deps:** `database` (prefix: `db`, optional — required if `transaction: true`)

**Outputs:** Dynamic from sub-workflow + `error`

```json
{
  "type": "workflow.run",
  "services": { "database": "postgres" },
  "config": {
    "workflow": "create-order",
    "input": {
      "user_id": "{{ input.user_id }}",
      "items": "{{ input.items }}"
    },
    "transaction": true
  }
}
```

### workflow.output

Terminal node that declares a named output for the workflow. Used in sub-workflows called by `workflow.run`.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `name` | string | yes | Output name |
| `data` | any (expr) | no | Output data |

**Outputs:** None (terminal)

```json
{
  "type": "workflow.output",
  "config": {
    "name": "created",
    "data": "{{ nodes.insert }}"
  }
}
```

---

## Transform

### transform.set

Creates a new object with resolved field expressions.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `fields` | object | yes | Key-value map of field names to expressions |

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "full_name": "{{ input.first_name + ' ' + input.last_name }}",
      "created_at": "{{ now() }}",
      "role": "user"
    }
  }
}
```

### transform.map

Transforms each item in an array using an expression.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `collection` | string (expr) | yes | Expression resolving to array |
| `expression` | string (expr) | yes | Expression applied to each item |

`$item` and `$index` are available in the expression.

```json
{
  "type": "transform.map",
  "config": {
    "collection": "{{ nodes.fetch }}",
    "expression": "{{ { 'id': $item.id, 'name': upper($item.name) } }}"
  }
}
```

### transform.filter

Filters an array by a predicate expression.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `collection` | string (expr) | yes | Expression resolving to array |
| `expression` | string (expr) | yes | Predicate — keeps items where truthy |

```json
{
  "type": "transform.filter",
  "config": {
    "collection": "{{ nodes.fetch }}",
    "expression": "{{ $item.status == 'active' }}"
  }
}
```

### transform.merge

Merges multiple arrays using different strategies.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `mode` | string | yes | `"append"`, `"match"`, or `"position"` (static) |
| `inputs` | array of strings | yes | Expressions resolving to arrays |
| `match` | object | no | For `match` mode |
| `match.type` | string | no | `"inner"`, `"outer"`, or `"enrich"` |
| `match.fields` | object | no | `left` and `right` join key field names |

```json
{
  "type": "transform.merge",
  "config": {
    "mode": "match",
    "inputs": ["{{ nodes.users }}", "{{ nodes.profiles }}"],
    "match": {
      "type": "inner",
      "fields": { "left": "id", "right": "user_id" }
    }
  }
}
```

### transform.delete

Removes fields from an object.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `data` | string (expr) | yes | Expression resolving to object |
| `fields` | array of strings | yes | Field names to remove |

```json
{
  "type": "transform.delete",
  "config": {
    "data": "{{ nodes.fetch[0] }}",
    "fields": ["password", "internal_notes"]
  }
}
```

### transform.validate

Validates data against a JSON Schema.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `data` | string (expr) | no | Data to validate (default: `{{ input }}`) |
| `schema` | object | yes | JSON Schema definition |

On validation failure, routes to the `error` output with detailed validation errors.

```json
{
  "type": "transform.validate",
  "config": {
    "schema": {
      "type": "object",
      "properties": {
        "email": { "type": "string", "format": "email" },
        "age": { "type": "integer", "minimum": 18 }
      },
      "required": ["email"]
    }
  }
}
```

---

## Response

### response.json

Builds an HTTP JSON response.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `status` | integer/expr | yes | HTTP status code (default: 200) |
| `body` | any | yes | Response body |
| `headers` | object | no | Response headers |
| `cookies` | string (expr) | no | Expression resolving to cookies array |

```json
{
  "type": "response.json",
  "config": {
    "status": 200,
    "body": {
      "data": "{{ nodes.fetch }}",
      "total": "{{ nodes.count[0].total }}"
    },
    "headers": {
      "X-Request-Id": "{{ trigger.trace_id }}"
    }
  }
}
```

### response.redirect

Builds an HTTP redirect response.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `url` | string (expr) | yes | Redirect target URL |
| `status` | integer | no | HTTP status code (default: 302) |

```json
{
  "type": "response.redirect",
  "config": {
    "url": "{{ '/api/tasks/' + string(nodes.insert.id) }}",
    "status": 301
  }
}
```

### response.error

Builds a standardized error response.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `status` | integer/expr | yes | HTTP status code (default: 500) |
| `code` | string (expr) | yes | Error code |
| `message` | string (expr) | yes | Error message |
| `details` | string (expr) | no | Additional details |

```json
{
  "type": "response.error",
  "config": {
    "status": 404,
    "code": "NOT_FOUND",
    "message": "Task not found"
  }
}
```

---

## Utility

### util.log

Logs a structured message.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `level` | string | yes | `"debug"`, `"info"`, `"warn"`, `"error"` (static) |
| `message` | string (expr) | yes | Log message |
| `fields` | object | no | Additional structured fields (expressions) |

```json
{
  "type": "util.log",
  "config": {
    "level": "info",
    "message": "Order created: {{ nodes.insert.id }}",
    "fields": {
      "user_id": "{{ auth.user_id }}",
      "total": "{{ input.total }}"
    }
  }
}
```

### util.uuid

Generates a UUID v4. No configuration required.

**Output:** UUID string.

```json
{
  "type": "util.uuid",
  "config": {}
}
```

### util.delay

Pauses execution for a specified duration. Respects context cancellation.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `timeout` | string | yes | Duration: `"5s"`, `"100ms"`, `"1m"` |

```json
{
  "type": "util.delay",
  "config": {
    "timeout": "2s"
  }
}
```

### util.timestamp

Returns the current UTC timestamp.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `format` | string | no | `"iso8601"` (default), `"unix"`, `"unix_ms"` |

```json
{
  "type": "util.timestamp",
  "config": {
    "format": "unix_ms"
  }
}
```

---

## Event

### event.emit

Publishes an event to a stream or pub/sub channel.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `mode` | string | yes | `"stream"` or `"pubsub"` (static) |
| `topic` | string (expr) | yes | Topic or stream name |
| `payload` | any (expr) | yes | Event payload |

**Service Deps:** `stream` (for stream mode) or `pubsub` (for pubsub mode)

**Output:** `{message_id}` for stream mode, `{ok: true}` for pubsub mode.

```json
{
  "type": "event.emit",
  "services": { "stream": "redis-stream" },
  "config": {
    "mode": "stream",
    "topic": "orders.created",
    "payload": {
      "order_id": "{{ nodes.insert.id }}",
      "user_id": "{{ auth.user_id }}"
    }
  }
}
```

---

## Upload

### upload.handle

Handles multipart file uploads with validation.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `max_size` | integer | yes | Max file size in bytes |
| `allowed_types` | array | yes | MIME type patterns (supports wildcards) |
| `path` | string (expr) | yes | Storage destination path |
| `max_files` | integer | no | Max files (default: 1) |
| `field` | string | no | Form field name (default: `"file"`) |

**Service Deps:** `destination` (prefix: `storage`, required)

**Output:** Single file: `{path, size, content_type, filename}`. Multiple files: `{files: [...]}`.

```json
{
  "type": "upload.handle",
  "services": { "destination": "files" },
  "config": {
    "max_size": 5242880,
    "allowed_types": ["image/*"],
    "path": "{{ 'avatars/' + auth.user_id + '/' + $uuid() }}",
    "max_files": 1
  }
}
```

---

## WebSocket & SSE

### ws.send

Sends data to WebSocket connections on a channel.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `channel` | string (expr) | yes | Channel name |
| `data` | any (expr) | yes | Data to send |

**Service Deps:** `connections` (prefix: `ws`, required)

```json
{
  "type": "ws.send",
  "services": { "connections": "chat-ws" },
  "config": {
    "channel": "{{ 'chat.' + input.room_id }}",
    "data": {
      "type": "message",
      "from": "{{ auth.user_id }}",
      "text": "{{ input.text }}"
    }
  }
}
```

### sse.send

Sends a Server-Sent Event to a channel.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `channel` | string (expr) | yes | Channel name |
| `data` | any (expr) | yes | Event data |
| `event` | string (expr) | no | Event type name |
| `id` | string (expr) | no | Event ID |

**Service Deps:** `connections` (prefix: `sse`, required)

```json
{
  "type": "sse.send",
  "services": { "connections": "notifications" },
  "config": {
    "channel": "{{ 'user.' + auth.user_id }}",
    "event": "notification",
    "data": {
      "title": "{{ input.title }}",
      "body": "{{ input.body }}"
    }
  }
}
```

---

## Wasm

### wasm.send

Sends a fire-and-forget command to a Wasm module.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `data` | any (expr) | yes | Command data |

**Service Deps:** `runtime` (prefix: `wasm`, required)

```json
{
  "type": "wasm.send",
  "services": { "runtime": "game-server" },
  "config": {
    "data": {
      "action": "player_move",
      "player_id": "{{ auth.user_id }}",
      "position": "{{ input.position }}"
    }
  }
}
```

### wasm.query

Sends a synchronous query to a Wasm module and awaits the response.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `data` | any (expr) | yes | Query data |
| `timeout` | string | no | Query timeout (default: `"5s"`) |

**Service Deps:** `runtime` (prefix: `wasm`, required)

```json
{
  "type": "wasm.query",
  "services": { "runtime": "game-server" },
  "config": {
    "data": {
      "query": "get_state",
      "player_id": "{{ auth.user_id }}"
    },
    "timeout": "2s"
  }
}
```

---

## Storage

### storage.read

Reads a file from storage.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `path` | string (expr) | yes | File path |

**Service Deps:** `storage` (prefix: `storage`, required)

**Output:** `{data, size, content_type}`

```json
{
  "type": "storage.read",
  "services": { "storage": "files" },
  "config": {
    "path": "{{ 'documents/' + input.file_id }}"
  }
}
```

### storage.write

Writes data to storage.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `path` | string (expr) | yes | File path |
| `data` | string/bytes (expr) | yes | Data to write |
| `content_type` | string (expr) | no | MIME type |

**Service Deps:** `storage` (prefix: `storage`, required)

```json
{
  "type": "storage.write",
  "services": { "storage": "files" },
  "config": {
    "path": "{{ 'exports/' + $uuid() + '.json' }}",
    "data": "{{ nodes.generate }}",
    "content_type": "application/json"
  }
}
```

### storage.delete

Deletes a file from storage.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `path` | string (expr) | yes | File path |

**Service Deps:** `storage` (prefix: `storage`, required)

### storage.list

Lists files under a prefix.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `prefix` | string (expr) | yes | Path prefix |

**Service Deps:** `storage` (prefix: `storage`, required)

**Output:** `{paths: [...]}`

```json
{
  "type": "storage.list",
  "services": { "storage": "files" },
  "config": {
    "prefix": "{{ 'users/' + auth.user_id + '/documents/' }}"
  }
}
```

---

## Database

### db.query

Executes a SELECT query and returns result rows.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `query` | string (expr) | yes | SQL SELECT statement |
| `params` | array | no | Positional query parameters ($1, $2, ...) |

**Service Deps:** `database` (prefix: `db`, required)

**Output:** Array of row objects (empty array if no results).

```json
{
  "type": "db.query",
  "services": { "database": "postgres" },
  "config": {
    "query": "SELECT * FROM tasks WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2",
    "params": ["{{ auth.user_id }}", "{{ input.limit ?? 20 }}"]
  }
}
```

### db.exec

Executes an INSERT, UPDATE, or DELETE statement.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `query` | string (expr) | yes | SQL statement |
| `params` | array | no | Positional query parameters |

**Output:** `{rows_affected: <count>}`

### db.create

Inserts a row into a table.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `data` | object | yes | Column values (expressions) |

**Output:** The inserted row data. Returns `ConflictError` on duplicate key.

```json
{
  "type": "db.create",
  "services": { "database": "postgres" },
  "config": {
    "table": "tasks",
    "data": {
      "title": "{{ input.title }}",
      "user_id": "{{ auth.user_id }}",
      "completed": false
    }
  }
}
```

### db.update

Updates rows matching a condition.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `data` | object | yes | Fields to update (expressions) |
| `condition` | string (expr) | yes | WHERE clause |
| `params` | array | no | Condition parameters |

**Output:** `{rows_affected: <count>}`

```json
{
  "type": "db.update",
  "services": { "database": "postgres" },
  "config": {
    "table": "tasks",
    "data": {
      "completed": "{{ input.completed }}",
      "updated_at": "{{ now() }}"
    },
    "condition": "id = $1 AND user_id = $2",
    "params": ["{{ input.id }}", "{{ auth.user_id }}"]
  }
}
```

### db.delete

Deletes rows matching a condition.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `condition` | string (expr) | yes | WHERE clause |
| `params` | array | no | Condition parameters |

**Output:** `{rows_affected: <count>}`

---

## Cache

### cache.get

Retrieves a value from the cache.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `key` | string (expr) | yes | Cache key |

**Output:** `{value: <any>}` (value is `nil` if key not found)

### cache.set

Sets a value in the cache.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `key` | string (expr) | yes | Cache key |
| `value` | any (expr) | yes | Value to store |
| `ttl` | integer | no | Time-to-live in seconds (0 = no expiry) |

**Output:** `{ok: true}`

```json
{
  "type": "cache.set",
  "services": { "cache": "redis" },
  "config": {
    "key": "{{ 'session:' + auth.user_id }}",
    "value": "{{ nodes.session_data }}",
    "ttl": 3600
  }
}
```

### cache.del

Deletes a key from the cache.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `key` | string (expr) | yes | Cache key |

**Output:** `{ok: true}`

### cache.exists

Checks if a key exists in the cache.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `key` | string (expr) | yes | Cache key |

**Output:** `{exists: true/false}`

---

## HTTP Client

### http.request

Makes an HTTP request.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `method` | string (expr) | yes | HTTP method |
| `url` | string (expr) | yes | Request URL |
| `headers` | object | no | Request headers (expressions) |
| `body` | any (expr) | no | Request body (auto-encodes maps as JSON) |
| `timeout` | string | no | Per-request timeout override |

**Service Deps:** `client` (prefix: `http`, required)

**Output:** `{status, headers, body}`

```json
{
  "type": "http.request",
  "services": { "client": "external-api" },
  "config": {
    "method": "POST",
    "url": "/webhooks/notify",
    "headers": {
      "X-Webhook-Secret": "{{ $env('WEBHOOK_SECRET') }}"
    },
    "body": {
      "event": "order.created",
      "data": "{{ nodes.order }}"
    },
    "timeout": "10s"
  }
}
```

### http.get

Shorthand for GET requests. Same as `http.request` with `method: "GET"`.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `url` | string (expr) | yes | Request URL |
| `headers` | object | no | Request headers |
| `timeout` | string | no | Request timeout |

### http.post

Shorthand for POST requests. Same as `http.request` with `method: "POST"`.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `url` | string (expr) | yes | Request URL |
| `headers` | object | no | Request headers |
| `body` | any (expr) | yes | Request body |
| `timeout` | string | no | Request timeout |

---

## Email

### email.send

Sends an email via SMTP.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `to` | string (expr) | yes | Recipient(s) |
| `subject` | string (expr) | yes | Subject line |
| `body` | string (expr) | yes | Email body |
| `from` | string (expr) | no | Sender (overrides service default) |
| `cc` | string (expr) | no | CC recipients |
| `bcc` | string (expr) | no | BCC recipients |
| `reply_to` | string (expr) | no | Reply-To address |
| `content_type` | string | no | `"html"` (default) or `"text"` |

**Service Deps:** `mailer` (prefix: `email`, required)

**Output:** `{message_id}`

```json
{
  "type": "email.send",
  "services": { "mailer": "smtp" },
  "config": {
    "to": "{{ input.email }}",
    "subject": "Welcome to our platform!",
    "body": "<h1>Welcome, {{ input.name }}!</h1>",
    "content_type": "html"
  }
}
```

---

## Image Processing

All image nodes require `source` and `destination` storage service deps.

### image.resize

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `width` | number (expr) | yes | Target width |
| `height` | number (expr) | yes | Target height |
| `quality` | number | no | JPEG quality (1-100) |
| `format` | string | no | Output format: jpeg, png, webp |

### image.crop

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `width` | number (expr) | yes | Crop width |
| `height` | number (expr) | yes | Crop height |
| `gravity` | string | no | Position: `center` (default), `top-left`, `top-right`, `bottom-left`, `bottom-right` |

### image.watermark

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `watermark` | string (expr) | yes | Watermark image path |
| `opacity` | number | no | Opacity 0-1 (default: 1.0) |
| `position` | string | no | `center` (default), `top-left`, `top-right`, `bottom-left`, `bottom-right` |

### image.convert

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `format` | string | yes | Target format: jpeg, png, webp, gif |
| `quality` | number | no | JPEG quality (1-100) |

### image.thumbnail

Smart crop + resize to exact dimensions.

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `width` | number (expr) | yes | Thumbnail width |
| `height` | number (expr) | yes | Thumbnail height |

```json
{
  "type": "image.thumbnail",
  "services": {
    "source": "uploads",
    "destination": "thumbnails"
  },
  "config": {
    "input": "{{ input.image_path }}",
    "output": "{{ 'thumbs/' + input.image_id + '.webp' }}",
    "width": 200,
    "height": 200
  }
}
```

---

## Error Handling

All nodes can route to an `error` output. When a node fails, the error data is available downstream as `ErrorData`:

| Field | Type | Description |
|-------|------|-------------|
| `code` | string | Error code (e.g., `VALIDATION_ERROR`, `NOT_FOUND`) |
| `message` | string | Human-readable error message |
| `node_id` | string | ID of the failed node |
| `node_type` | string | Type of the failed node |
| `trace_id` | string | Request trace ID |
| `details` | any | Additional error details |

### Error Type to HTTP Status Mapping

| Error Type | HTTP Status | Code |
|-----------|-------------|------|
| `ValidationError` | 422 | `VALIDATION_ERROR` |
| `NotFoundError` | 404 | `NOT_FOUND` |
| `ConflictError` | 409 | `CONFLICT` |
| `ServiceUnavailableError` | 503 | `SERVICE_UNAVAILABLE` |
| `TimeoutError` | 504 | `TIMEOUT` |
| Other | 500 | `INTERNAL_ERROR` |
