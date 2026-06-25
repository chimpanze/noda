# External-Service Node E2E — Findings

Bugs found and fixed while implementing `2026-06-25-external-node-e2e.md`.
One entry per bug: node, symptom, root cause, fix, guarding test.

## Bug 1 — `db.create` does not return server-generated fields

- **Node:** `db.create`
- **Symptom:** The node returns `success` with the input `data` map unchanged; server-generated fields (e.g. `id` from a `serial PRIMARY KEY`, `created_at` defaults) are absent from the output even though the descriptor documents "The created row object including generated fields (id, created_at, etc.)".
- **Root cause:** `create.go` called `db.Table(table).Create(data)` without a `RETURNING` clause. GORM's `Create` with a `map[string]any` argument does not scan generated columns back into the map (unlike struct-based creates where GORM reads `LastInsertId` or a `Returning` clause automatically). The result was that the output map was identical to the input — `id` was nil.
- **Fix:** Added `clause.Returning{}` to the GORM call in `plugins/db/create.go`:
  ```go
  tx := db.WithContext(ctx).Table(table).Clauses(clause.Returning{}).Create(data)
  ```
  Postgres executes `INSERT … RETURNING *` and GORM populates the map with all returned columns, including `id`.
- **Guarding test:** `TestDBCreateAndFind_Engine` in `plugins/db/engine_e2e_integration_test.go` asserts `assert.NotNil(t, row["id"])` after a `db.create` call against a table with a `serial PRIMARY KEY`.

