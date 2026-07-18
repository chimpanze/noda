# Node Cookbook

One small, runnable Noda project per plugin family. Every endpoint's
request/response pair is executed against the real server in CI
(`make test-integration` → `internal/testing/cookbook`), driven by each
project's `verify.json`.

| Project | Nodes |
|---------|-------|
| [control](control/) | `control.if`, `control.switch`, `control.loop` |
| [transform](transform/) | `transform.set`, `transform.map`, `transform.filter`, `transform.merge`, `transform.delete`, `transform.validate` |
| [response](response/) | `response.json`, `response.error`, `response.redirect`, `response.file` |
| [util](util/) | `util.log`, `util.uuid`, `util.timestamp`, `util.delay`, `util.jwt_sign` |
| [workflow](workflow/) | `workflow.run`, `workflow.output` |
| [db](db/) | `db.create`, `db.find`, `db.findOne`, `db.update`, `db.upsert`, `db.delete`, `db.count`, `db.query`, `db.exec` |
| [cache](cache/) | `cache.get`, `cache.set`, `cache.del`, `cache.exists` |
| [storage](storage/) | `storage.write`, `storage.read`, `storage.list`, `storage.delete` |
| [upload](upload/) | `upload.handle` |
| [image](image/) | `image.resize`, `image.crop`, `image.convert`, `image.watermark`, `image.thumbnail` |
| [email](email/) | `email.send` |

Remaining service-backed families (events, realtime, http, wasm, auth, oidc,
livekit) arrive in later tranches.

Projects whose `verify.json` lists a `deps` entry (`db`, `cache`, `email`)
need Docker running locally — `go test -tags=integration ./internal/testing/cookbook/ -run TestCookbook -v`
starts the required Postgres/Redis/Mailpit containers automatically. The
`storage`, `upload`, and `image` projects use `COOKBOOK_DATA_DIR`, which the
test harness provisions automatically; set it manually (to any writable
directory) when running `noda start` on one of these projects by hand.

## Running one project

```bash
noda start --config examples/node-cookbook/<family>
```

## Verifying locally

```bash
go test -tags=integration ./internal/testing/cookbook/ -run TestCookbook -v
```
