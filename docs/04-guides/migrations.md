# Database Migrations

Noda manages your database schema with plain SQL migration files. Migrations are **not** a Noda config type — they are `.sql` files in the `migrations/` directory, applied with the `noda migrate` CLI. There is no `migration` schema in `noda_get_config_schema`, and `noda_validate_config` does not read them.

## File Format and Naming

Each migration is a **pair** of files in `migrations/`:

```
migrations/
  20260101120000_create_notes.up.sql     # forward migration
  20260101120000_create_notes.down.sql   # rollback
```

The filename is `<version>_<name>.<direction>.sql`:

- **`version`** — a 14-digit UTC timestamp `YYYYMMDDHHMMSS`. Migrations are applied in ascending version order, so the timestamp determines ordering.
- **`name`** — a human-readable label (everything after the first underscore). It may contain underscores.
- **`direction`** — `up` (apply) or `down` (roll back).

A file that does not match `<14-digit-version>_<name>.(up|down).sql` is skipped with a warning. Discovery scans for `*.up.sql` files; each must have a matching `.down.sql`.

> Generate the pair with `noda migrate create <name>` rather than hand-naming files — it stamps the current UTC timestamp for you (see below).

`up.sql` holds the forward DDL; `down.sql` holds the inverse so the migration can be rolled back:

```sql
-- 20260101120000_create_notes.up.sql
CREATE TABLE notes (
  id         UUID PRIMARY KEY,
  user_id    TEXT NOT NULL,
  body       TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_notes_user_id ON notes (user_id);
```

```sql
-- 20260101120000_create_notes.down.sql
DROP TABLE notes;
```

## Applying Migrations

Migrations are applied **only** through the CLI — the server does **not** run them automatically at startup. Each command operates on the database service named by `--service` (default `db`), resolved from `noda.json` `services`:

```bash
# Create a new migration pair (stamps a UTC timestamp)
noda migrate create create_notes
# → Created:
#     migrations/20260101120000_create_notes.up.sql
#     migrations/20260101120000_create_notes.down.sql

# Apply all pending migrations, in version order
noda migrate up

# Roll back the single most recently applied migration
noda migrate down

# Show which migrations are applied vs pending
noda migrate status
#   [applied] 20260101120000_create_notes
#   [pending] 20260102090000_add_notes_pinned

# Target a non-default database service
noda migrate up --service analytics-db
```

Each `up` runs inside a transaction and, on success, records its version in a `schema_migrations` table that Noda maintains in your database. Already-applied versions are skipped on the next `up`. `down` rolls back exactly one migration — the latest applied — and removes its row from `schema_migrations`; run it repeatedly to step further back.

## Notes

- **No auto-apply.** `noda dev` and `noda start` do not apply migrations. Run `noda migrate up` yourself (or in your deploy step) before starting the server against a fresh database.
- **The `db` service must exist.** `noda migrate` resolves `--service` (default `db`) from `noda.json` `services` and expects a `postgres`/`db` plugin instance. See the Service Wiring Guide (`noda://docs/services`).
- **Tables aren't created for you.** Examples and the quick-start tutorial write to tables like `users` or `tasks`; you must create those tables with a migration first.
