# Variables

## Shared Variables (`vars.json`)

Define named values in a `vars.json` file at the project root to avoid repeating strings across config files:

```json
{
  "MAIN_DB": "main-db",
  "TOPIC_MEMBER_INVITED": "member.invited",
  "TABLE_TASKS": "tasks"
}
```

All values must be strings. Reference them with `$var()` in any config section:

```json
{
  "subscribe": {
    "topic": "{{ $var('TOPIC_MEMBER_INVITED') }}"
  }
}
```

### How it works

- **Standalone** `{{ $var('X') }}` is resolved at **config load time** — the entire field value is replaced before the workflow is loaded
- **Inside expressions** like `{{ "prefix." + $var('TOPIC') }}`, `$var()` is a **runtime function** evaluated by the expression engine
- Config-time resolution works across **all** config sections: root, routes, workflows, workers, schedules, connections, tests, and models
- Resolution happens after `$env()` and before `$ref`, so you can use environment variables inside `vars.json` values but not `$var()` inside `$ref` targets
- An undefined variable name produces a load error (config-time) or runtime error (expression) with the variable name

### When to use `$var()` vs `$env()`

| | `$var()` | `$env()` |
|---|---|---|
| **Source** | `vars.json` (checked into version control) | OS environment / `.env` file |
| **Scope** | All config sections | Root config only |
| **Use case** | Shared logical names (topics, tables, service names) | Secrets and environment-specific values (DSNs, keys) |

### Example

```
vars.json
```
```json
{
  "STREAM_SVC": "redis-stream",
  "TOPIC_TASK_CREATED": "task.created",
  "TOPIC_TASK_FAILED": "task.failed"
}
```

```
workers/process-task.json
```
```json
{
  "id": "process-task",
  "services": { "stream": "{{ $var('STREAM_SVC') }}" },
  "subscribe": { "topic": "{{ $var('TOPIC_TASK_CREATED') }}" },
  "dead_letter": { "topic": "{{ $var('TOPIC_TASK_FAILED') }}" },
  "trigger": { "workflow": "process-task" }
}
```

---

## Environment Variables and Overlays

### `$env()` Function

Use `$env('VAR_NAME')` in any string value in the root config to reference environment variables:

```json
{
  "dsn": "{{ $env('DATABASE_URL') }}"
}
```

**Note:** `$env()` only resolves in `noda.json` (and its overlay). For values needed across all config sections, define them in `vars.json` using `$var()` instead.

### `.env` File

Noda auto-loads `.env` files from the config directory.

### Environment Overlays

Create environment-specific overlays that merge on top of base config:

```bash
noda start --env production
```

This loads `noda.json` first, then deep-merges `noda.production.json` on top.
