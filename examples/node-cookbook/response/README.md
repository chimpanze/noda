# Cookbook: response nodes

Runnable examples for `response.json`, `response.error`, `response.redirect`, and `response.file`.
Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

```bash
noda start --config examples/node-cookbook/response
```

## response.json — `GET /api/json`

Builds a JSON response with a custom status code and an arbitrary (nestable) body.

```bash
curl -i localhost:3000/api/json
# → 201 {"message":"created","nested":{"n":1}}
```

## response.error — `GET /api/error`

Builds the standardized error envelope `{"error":{"code","message","trace_id"}}`;
`trace_id` is injected automatically from the execution context.

```bash
curl localhost:3000/api/error
# → 404 {"error":{"code":"NOT_FOUND","message":"Thing not found","trace_id":"..."}}
```

## response.redirect — `GET /api/redirect`

Sets the `Location` header from `url` and responds with the given status (default 302).

```bash
curl -i localhost:3000/api/redirect
# → 302 with header: Location: https://example.com/next
```

## response.file — `GET /api/file`

Sends raw bytes with a `Content-Type` header; a string `data` value is sent as-is,
and setting `filename` adds a `Content-Disposition: attachment` header.

```bash
curl -i localhost:3000/api/file
# → 200 with headers:
#   Content-Type: text/csv
#   Content-Disposition: attachment; filename="demo.csv"
# → body:
# hello,cookbook
# 1,2
```
