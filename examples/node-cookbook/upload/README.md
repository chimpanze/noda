# Cookbook: upload nodes

Runnable examples for `upload.handle` against a local-filesystem storage
service. Every request/response below is verified in CI by
[`verify.json`](verify.json) (as `curl -F` multipart requests).

## Run

This project needs a writable directory — CI's cookbook walker always exports
`COOKBOOK_DATA_DIR` (an isolated temp dir) before running the suite. To run it
yourself:

```bash
export COOKBOOK_DATA_DIR=/tmp/noda-upload-cookbook
go run ./cmd/noda start --config examples/node-cookbook/upload
```

## upload.handle — `POST /api/upload`

The route declares the multipart file field via `trigger.files` (mirroring
`examples/saas-backend/routes/upload-attachment.json`) so the raw
`*multipart.FileHeader` reaches the workflow as `input.file`. `upload.handle`
reads the file fully into memory, **detects its MIME type from the actual
content bytes** (`http.DetectContentType` — the client-supplied
`Content-Type` header is not trusted), validates it against `allowed_types`
(`["text/*"]` here), then writes it to the `uploads` storage service at
`docs/{{ $uuid() }}`. On success, its output
`{path, size, content_type, filename}` is returned as the response body.

```bash
curl -X POST localhost:3000/api/upload -F 'file=@notes.txt;type=text/plain'
# → 201 {"path":"docs/<uuid>","size":15,"content_type":"text/plain; charset=utf-8","filename":"notes.txt"}
```

Note `content_type` comes back as `text/plain; charset=utf-8`, not the bare
`text/plain` a caller might expect — `http.DetectContentType` appends a
charset for text content (verified by running the suite).

### Rejected uploads

A disallowed type (declared `Content-Type` is ignored; detection here keys
off the binary content) fires `upload.handle`'s `error` output
(`ValidationError`), wired to a `response.error` node returning `422`:

```bash
curl -X POST localhost:3000/api/upload -F 'file=@evil.bin;type=application/octet-stream'
# → 422 {"error":{"code":"VALIDATION_ERROR","message":"File rejected: size or type constraints violated"}}
```
