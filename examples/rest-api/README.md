# REST API Example — Task Management

A simple CRUD API for tasks, based on `docs/use-cases/01-rest-api.md`.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/tasks | Create a task |
| GET | /api/tasks | List tasks (with pagination) |
| GET | /api/tasks/:id | Get a single task |
| PUT | /api/tasks/:id | Update a task |
| DELETE | /api/tasks/:id | Delete a task |

All endpoints require JWT authentication (`auth.jwt` middleware).

## Running

```bash
# Start PostgreSQL
docker compose up -d postgres

# Create the example database and table
docker compose exec postgres psql -U noda -d noda_dev -c "CREATE DATABASE noda_example;"
docker compose exec postgres psql -U noda -d noda_example -c "
CREATE TABLE IF NOT EXISTS tasks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL,
  description TEXT,
  status TEXT NOT NULL DEFAULT 'todo',
  user_id TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);"

# Validate config
noda validate --config examples/rest-api

# Run workflow tests (no database needed)
noda test --config examples/rest-api

# Start the server
noda start --config examples/rest-api
```

## Bugs Found During Development

The following framework bugs were discovered and fixed or documented while building this example:

### Fixed in this branch

1. **Plugin lookup by name vs prefix** (`internal/registry/lifecycle.go`)
   - `InitializeServices` called `plugins.Get(pluginName)` but Get() looks up by prefix.
   - The postgres plugin has Name()="postgres" but Prefix()="db", so service creation failed with "unknown plugin".
   - Fix: Added `GetByName()` method and used it in InitializeServices.

2. **Service config not unwrapped** (`internal/registry/lifecycle.go`)
   - `CreateService(cfg)` received the full service entry `{"plugin":"postgres","config":{"url":"..."}}`
     but plugins expect just the inner config map `{"url":"..."}`.
   - Fix: Unwrap the "config" field before passing to the plugin.

3. **response.json only handles string config values** (`plugins/core/response/json.go`)
   - `status` as a JSON number (e.g., `201`) was ignored (only string expressions handled).
   - `body` as a JSON object (e.g., `{"data": "{{ rows }}"}`) was ignored — only string expressions resolved.
   - Fix: Added type switch for status (handle float64 directly) and body (recursive deep resolution via `resolveDeep`).

4. **response.error only handles string status** (`plugins/core/response/error.go`)
   - Same issue as response.json: numeric `status` values from JSON were ignored.
   - Fix: Same type switch pattern.

### Also fixed in this branch

5. **`noda validate` connects to real databases** (`internal/registry/bootstrap.go`)
   - Fix: Added `BootstrapOptions{DryRun: true}` to skip service creation during validation. Validates node types, service references (against config), and expressions without connecting to databases.

6. **Node IDs clash with expr-lang built-in functions** (`internal/engine/context.go`)
   - Fix: Node outputs are now namespaced under `nodes` map (`{{ nodes.fetch }}` instead of `{{ fetch }}`). This eliminates clashes with `find`, `count`, `filter`, `map`, `len`, `sum`, etc.

7. **Expression context flat namespace** (`internal/engine/context.go`, `internal/engine/compiler.go`)
   - Fix: `buildExprContext()` now puts outputs under `ctx["nodes"]`. `extractIdentifiers()` recognizes `nodes.X` pattern for eviction tracking. All config files updated to use `{{ nodes.X }}`.

8. **Trigger mapping `request.*` prefix** (`internal/server/trigger.go`)
   - Fix: `buildRawRequestContext()` now puts `body`, `query`, `params`, `headers` as top-level keys matching the architecture docs.

9. **`auth.sub` not in trigger context** (`internal/server/trigger.go`)
   - Fix: JWT auth claims are now injected into the trigger mapping context so `{{ auth.sub }}` works in route input mappings.

10. **Silent workflow "success" on unhandled node errors** (`internal/engine/executor.go`)
    - Fix: When a node errors and has error outputs but no error edge in the graph, the workflow now fails with a clear error message instead of silently succeeding.

11. **`transform.validate` requires explicit `data` field** (`plugins/core/transform/validate.go`)
    - Fix: `data` is now optional in the config schema. If omitted, defaults to `{{ input }}`.

12. **String query params break arithmetic** (`internal/server/trigger.go`, `internal/expr/functions.go`)
    - Fix: Trigger mapping auto-coerces numeric string values to numbers. Also added `toInt()` and `toFloat()` expression functions for explicit use.

13. **`plugin list` command is hardcoded** (`cmd/noda/plugin.go`)
    - Fix: Now iterates real plugin/node registries using `corePlugins()` and `serviceOnlyPlugins()`.
