# examples/node-cookbook/response

A [Noda](https://github.com/chimpanze/noda) cookbook project demonstrating the response node family: `response.json`, `response.error`, `response.redirect`, and `response.file`.

## Getting Started

```bash
# Validate config
noda validate --config .

# Run cookbook tests
go test -tags=integration ./internal/testing/cookbook/ -run 'TestCookbook/response' -v
```

## Endpoints

### `GET /api/json` — JSON response with custom status

Returns a 201 Created response with structured JSON data:

```bash
curl http://localhost:3000/api/json
# → HTTP 201
# → {"message":"created","nested":{"n":1}}
```

The `response.json` node accepts a `status` code and a `body` object that can nest values. The body is evaluated as an expression, so you can use workflow node outputs: `"body": { "id": "{{ nodes.create_row.id }}" }`.

### `GET /api/error` — Standardized error response

Returns a 404 with a structured error envelope:

```bash
curl http://localhost:3000/api/error
# → HTTP 404
# → {"error":{"code":"NOT_FOUND","message":"Thing not found","trace_id":"..."}}
```

The `response.error` node produces a standard error format with:
- `error.code` — the error code (required)
- `error.message` — the error message (required)
- `error.trace_id` — automatically injected from the execution context

Optional `details` field adds context (e.g., validation errors).

### `GET /api/redirect` — HTTP redirect

Returns a 302 redirect to another URL:

```bash
curl -i http://localhost:3000/api/redirect
# → HTTP 302 Found
# → Location: https://example.com/next
```

The `response.redirect` node sets the `Location` header and returns the specified HTTP status (default 302). Supports both relative (`/new-path`) and absolute URLs (`https://example.com/next`). Protocol-relative (`//example.com`) and invalid URLs are rejected.

### `GET /api/file` — Raw binary file response

Returns raw bytes with appropriate headers for download:

```bash
curl -i http://localhost:3000/api/file
# → HTTP 200
# → Content-Type: text/csv
# → Content-Disposition: attachment; filename="demo.csv"
# → 
# → hello,cookbook
# → 1,2
```

The `response.file` node sends raw bytes (or strings converted to bytes) with a `Content-Type` header. When `filename` is set, it adds a `Content-Disposition: attachment` header for browser downloads.

The `data` field accepts:
- Bytes from `storage.read` nodes: `"data": "{{ nodes.read_file.data }}"`
- String literals: `"data": "hello,world\n"` (sent as-is, converted to bytes)
- Expressions that resolve to bytes or strings

## Project Structure

```
noda.json           — main configuration (server, services)
routes/             — HTTP route definitions (json, error, redirect, file)
workflows/          — workflow definitions (json, error, redirect, file)
verify.json         — cookbook test suite
README.md           — this file
```
