# RealWorld / Conduit

An implementation of the [RealWorld](https://realworld-docs.netlify.app/) (Conduit)
API spec on Noda — a config-only project (routes, workflows, middleware, and
migrations; no application code) exercising articles, tags, favorites, follows,
and comments on top of the built-in `auth` plugin.

## Running locally

```bash
docker compose up
```

This starts Postgres and the Noda server (`http://localhost:3000`), auto-migrating
the `auth` plugin's own users/sessions/tokens tables plus this project's domain
tables (`migrations/0001_init.up.sql`).

## Conformance

See `FINDINGS.md` for spec-conformance notes and gaps, and `harness/known-failing.json`
for the pinned baseline of known-gap assertions the gate tolerates.

Run the full integration suite (includes `TestRealWorldConformance`) with:

```bash
make test-integration
```

### Running the conformance suite directly

Requires Docker (Postgres via testcontainers) and the `hurl` binary:

```bash
brew install hurl   # local; CI installs the pinned .deb release instead
docker compose up   # start the app being tested, if not using testcontainers-managed boot
go test -tags integration ./internal/testing/realworld/ -run TestRealWorldConformance -v
```

The test prints `N passed, Y known-gap, 0 unexpected` on success. If `hurl` is
missing from `PATH`, the test `t.Skip`s rather than failing.
