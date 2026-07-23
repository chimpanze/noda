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

See `FINDINGS.md` for spec-conformance notes and gaps, and run the conformance
suite with:

```bash
make test-integration
```

which includes `TestRealWorldConformance`.
