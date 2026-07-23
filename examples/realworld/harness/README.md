# RealWorld Hurl Conformance Suite (vendored)

This directory vendors the RealWorld ("Conduit") project's own external HTTP
conformance suite so it can be run against the Noda implementation in
`examples/realworld/` without any code changes to the suite itself.

## Provenance

- **Upstream repo:** `gothinkster/realworld`
- **Upstream path:** `specs/api/hurl/`
- **Pinned commit:** `98f29fb3f8bcb1dd614b91f2851371bf22c34775` (was `main` HEAD
  at the time of fetch)
- **Fetch date:** 2026-07-23

Upstream migrated its official conformance suite from Postman/Newman to
[Hurl](https://hurl.dev/) — a single static binary that runs plain-text
`.hurl` files, no Node.js or Docker required. `hurl/` here is that suite,
fetched byte-for-byte at the pinned commit above and **not modified**. Treat
every file in `hurl/*.hurl` as an external contract: do not edit them. If
upstream changes the suite, re-vendor at a new pinned commit and record the
new SHA and fetch date here.

Files vendored:

| File | Purpose |
| --- | --- |
| `auth.hurl` | registration, login, current user, update user |
| `articles.hurl` | article CRUD |
| `comments.hurl` | comments on articles |
| `favorites.hurl` | favorite/unfavorite articles |
| `feed.hurl` | the personalized article feed |
| `pagination.hurl` | list endpoints' limit/offset behavior |
| `profiles.hurl` | user profiles, follow/unfollow |
| `tags.hurl` | tag listing |
| `errors_articles.hurl` | article endpoint error cases |
| `errors_auth.hurl` | auth endpoint error cases |
| `errors_authorization.hurl` | missing/invalid-token error cases |
| `errors_comments.hurl` | comment endpoint error cases |
| `errors_profiles.hurl` | profile endpoint error cases |
| `run-hurl-tests.sh` | upstream's own convenience runner (vendored for provenance only — Task 10's gate does not shell out to this script; it drives `hurl` itself) |

## Variable convention

The suite uses exactly two Hurl variables, passed on the command line:

- `host` — the base URL of the running server, e.g. `http://127.0.0.1:PORT`.
  The `.hurl` files already append `/api/...` to every request path, so
  `host` must **not** include a trailing `/api` — pass just the scheme+host
  (+port), e.g. `http://127.0.0.1:8080`, not `http://127.0.0.1:8080/api`.
- `uid` — a unique per-run suffix (e.g. a timestamp or random token) mixed
  into usernames/emails/slugs so repeated runs against a persistent database
  don't collide on uniqueness constraints.

Example invocation (what the gate does, conceptually):

```sh
hurl --test --jobs 1 \
  --variable "host=http://127.0.0.1:8080" \
  --variable "uid=$(date +%s)$$" \
  examples/realworld/harness/hurl/*.hurl
```

## Consumers

- **Task 10** adds `TestRealWorldConformance` (see `internal/testing/realworld`),
  the CI gate that boots the `examples/realworld` app and runs this suite
  against it.
- **`known-failing.json`** (this directory) is the documented-gap baseline
  the gate consults: entries there are Hurl checks known to fail because of a
  specific, tracked Noda gap (a FINDINGS.md row + a GitHub issue), so the
  gate can distinguish "known, tracked gap" from "new regression." It starts
  empty — see that file's `_comment` for the update policy.
