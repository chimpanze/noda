# Cookbook: storage nodes

Runnable examples for `storage.write`, `storage.read`, `storage.list`, and
`storage.delete` against a local-filesystem storage service.
Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

This project needs a writable directory — CI's cookbook walker always exports
`COOKBOOK_DATA_DIR` (an isolated temp dir) before running the suite. To run it
yourself:

```bash
export COOKBOOK_DATA_DIR=/tmp/noda-storage-cookbook
go run ./cmd/noda start --config examples/node-cookbook/storage
```

## storage.write — `POST /api/files`

Writes `data` to `path` under the storage root and echoes the path back.

```bash
curl -X POST localhost:3000/api/files -H 'Content-Type: application/json' \
  -d '{"path": "docs/a.txt", "data": "alpha"}'
# → 201 {"path":"docs/a.txt"}
```

## storage.read — `GET /api/files/read?path=X`

Per `docs/03-nodes/storage.read.md`, the node's `success` output is
`{data, size, content_type}` — `data` comes back as raw bytes (`[]byte`).
JSON has no byte-string type, so putting `nodes.load.data` straight into a
`response.json` body serializes it as a **standard-base64 string** (verified
by running the suite: writing `"alpha"` and reading it back yields
`"YWxwaGE="`, not the literal text). This workflow returns the field as
`content_base64` to make that encoding explicit, alongside `content_type` and
`size` from the same node.

```bash
curl 'localhost:3000/api/files/read?path=docs/a.txt'
# → 200 {"content_base64":"YWxwaGE=","content_type":"text/plain; charset=utf-8","size":5}
```

## storage.list — `GET /api/files?prefix=X`

**Config note:** the node's config field is `prefix`, not `path` — see
`docs/03-nodes/storage.list.md` (`storage.write`/`read`/`delete` all use
`path`; `list` is the one exception). Output is `{paths: [...]}`, projected
here into `files`.

```bash
curl 'localhost:3000/api/files?prefix=docs/'
# → 200 {"files":["docs/a.txt","docs/b.txt"]}
```

## storage.delete — `DELETE /api/files?path=X`

```bash
curl -X DELETE 'localhost:3000/api/files?path=docs/a.txt'
# → 204 (empty body)
```
