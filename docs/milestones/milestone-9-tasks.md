# Milestone 9: Database Plugin — Task Breakdown

**Depends on:** Milestone 3 (plugin system), Milestone 8 (HTTP server)
**Result:** PostgreSQL plugin works with all `db.*` nodes. Transactions wrap sub-workflows. Migrations create/apply SQL files. Use Case 1 (Simple REST API) is fully buildable.

---

## Task 9.1: PostgreSQL Plugin Shell

**Description:** Create the database plugin that manages GORM connections.

**Subtasks:**

- [x] Create `plugins/db/plugin.go`:
  - Name: `"postgres"`, Prefix: `"db"`
  - HasServices: true
  - CreateService: parse connection URL from config, create GORM connection pool, configure pool settings (max open, max idle, connection lifetime)
  - HealthCheck: `db.Ping()`
  - Shutdown: close connection pool
  - Nodes: registers `db.query`, `db.exec`, `db.create`, `db.update`, `db.delete`
- [x] Support multiple instances (main-db, analytics-db) with different connection URLs
- [x] Connection pool settings configurable: `max_open`, `max_idle`, `conn_lifetime`

**Tests:**
- [x] Plugin registers with `db` prefix
- [x] CreateService establishes connection to PostgreSQL
- [x] HealthCheck passes on running PostgreSQL
- [x] HealthCheck fails on unreachable PostgreSQL
- [x] Shutdown closes connection pool
- [x] Multiple instances created with different configs

**Acceptance criteria:** PostgreSQL connections managed through the plugin lifecycle.

---

## Task 9.2: `db.query` Node

**Description:** Execute parameterized SELECT queries.

**Subtasks:**

- [x] Create `plugins/db/query.go`
- [x] ConfigSchema: `query` (required expression → string), `params` (optional expression array)
- [x] ServiceDeps: `{ "database": { prefix: "db", required: true } }`
- [x] Execute: resolve `query` and `params`, call `gorm.Raw(query, params...).Scan(&results)` with `[]map[string]any` destination
- [x] Pass `context.Context` to GORM for timeout/cancellation
- [x] Return result rows as array of maps

**Tests:**
- [x] SELECT returns rows as `[]map[string]any`
- [x] Parameterized query binds values correctly ($1, $2 style)
- [x] Empty result returns empty array
- [x] SQL error → node error with message
- [x] Context cancellation stops the query

**Acceptance criteria:** Read queries execute with parameter binding.

---

## Task 9.3: `db.exec` Node

**Description:** Execute parameterized write statements (raw SQL).

**Subtasks:**

- [x] Create `plugins/db/exec.go`
- [x] ConfigSchema: `query` (required expression), `params` (optional expression array)
- [x] Execute: resolve and call `gorm.Exec(query, params...)`, return `{ "rows_affected": N }`

**Tests:**
- [x] INSERT returns rows_affected = 1
- [x] UPDATE multiple rows returns correct count
- [x] DELETE returns correct count
- [x] SQL error → node error

**Acceptance criteria:** Raw SQL write statements execute with result counts.

---

## Task 9.4: `db.create` Node

**Description:** Insert a record using GORM's map interface.

**Subtasks:**

- [x] Create `plugins/db/create.go`
- [x] ConfigSchema: `table` (required expression), `data` (required expression → object)
- [x] Execute: resolve `table` and `data`, call `gorm.Table(table).Create(data)`, return created record with generated fields (id, timestamps)
- [x] Handle: auto-generated ID returned in output

**Tests:**
- [x] Insert record returns created row with generated ID
- [x] Multiple fields inserted correctly
- [x] Constraint violation → ConflictError
- [x] Null fields handled

**Acceptance criteria:** Records created with generated fields returned.

---

## Task 9.5: `db.update` and `db.delete` Nodes

**Description:** Update and delete records by condition.

**Subtasks:**

- [x] Create `plugins/db/update.go`:
  - ConfigSchema: `table`, `data` (fields to update), `condition` (WHERE clause), `params`
  - Execute: `gorm.Table(table).Where(condition, params...).Updates(data)`, return `{ "rows_affected": N }`
- [x] Create `plugins/db/delete.go`:
  - ConfigSchema: `table`, `condition`, `params`
  - Execute: `gorm.Table(table).Where(condition, params...).Delete(nil)`, return `{ "rows_affected": N }`

**Tests:**
- [x] Update specific rows by condition
- [x] Delete specific rows by condition
- [x] Condition with params binds correctly
- [x] No matching rows → rows_affected = 0

**Acceptance criteria:** Update and delete with parameterized conditions.

---

## Task 9.6: Transaction Support in `workflow.run`

**Description:** Implement `transaction: true` on `workflow.run` — wrap sub-workflow in a GORM transaction.

**Subtasks:**

- [x] Extend `workflow.run` executor (from M5):
  - When `transaction: true`: resolve the `database` service slot to get the GORM connection
  - Call `gorm.Transaction(func(tx *gorm.DB) error { ... })`
  - Inside the transaction: create a modified service registry where the `database` slot points to the transaction `tx` instead of the connection pool
  - Execute the sub-workflow with this modified registry
  - If sub-workflow succeeds → transaction commits automatically
  - If sub-workflow fails → transaction rolls back automatically
- [x] The sub-workflow's `db.*` nodes receive the transaction connection transparently — they don't know they're inside a transaction
- [x] Nested transactions: if a sub-workflow contains another `workflow.run` with `transaction: true`, GORM handles savepoints

**Tests:**
- [x] Success path: all DB operations commit
- [x] Failure path: all DB operations roll back
- [x] Multiple db.create inside transaction → all or nothing
- [x] Sub-workflow db nodes use transaction connection (verify with rollback test)
- [x] Nested transaction with savepoint

**Acceptance criteria:** Database transactions wrap sub-workflows atomically.

---

## Task 9.7: Migration CLI

**Description:** Implement `noda migrate` commands for database migration management.

**Subtasks:**

- [x] Create `internal/migrate/` package
- [x] `noda migrate create [name]`:
  - Generate timestamped file pair: `migrations/YYYYMMDDHHMMSS_name.up.sql` and `.down.sql`
  - Files contain a comment placeholder: `-- Write your migration SQL here`
- [x] `noda migrate up`:
  - Read all migration files in order
  - Track applied migrations in a `schema_migrations` table
  - Apply all pending `.up.sql` files in order
  - Print each applied migration
- [x] `noda migrate down`:
  - Roll back the last applied migration using its `.down.sql` file
  - Update `schema_migrations` table
- [x] `noda migrate status`:
  - Show all migrations with applied/pending status
- [x] Wire all commands into Cobra CLI

**Tests:**
- [x] `migrate create` generates correct file names
- [x] `migrate up` applies pending migrations in order
- [x] `migrate down` rolls back the last migration
- [x] `migrate status` shows correct applied/pending
- [x] Already-applied migrations are skipped on `migrate up`
- [x] `schema_migrations` table created automatically

**Acceptance criteria:** Full migration lifecycle works from CLI.

---

## Task 9.8: End-to-End Tests — Use Case 1

**Description:** Build and test the Simple REST API use case.

**Subtasks:**

- [x] Create test project matching Use Case 1: task CRUD API
- [x] Test: `POST /api/tasks` → creates task in database, returns 201
- [x] Test: `GET /api/tasks` → queries tasks with pagination
- [x] Test: `GET /api/tasks/:id` → returns task or 404
- [x] Test: `PUT /api/tasks/:id` → updates task
- [x] Test: `DELETE /api/tasks/:id` → deletes task
- [x] Test: JWT auth required on all routes
- [x] Test: Parallel queries in list endpoint (count + fetch)
- [x] Test: Transaction in create workflow (if applicable)
- [x] Write `noda test` files for all task workflows

**Acceptance criteria:** Use Case 1 works end-to-end with real PostgreSQL.
