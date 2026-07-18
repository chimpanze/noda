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

Service-backed families (db, cache, storage, upload, image, email, events,
realtime, http, wasm, auth, oidc, livekit) arrive in later tranches.

## Running one project

```bash
noda start --config examples/node-cookbook/<family>
```

## Verifying locally

```bash
go test -tags=integration ./internal/testing/cookbook/ -run TestCookbook -v
```
