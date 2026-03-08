# Noda — Use Case: Discord Bot

**Version**: 0.4.0

A Discord moderation bot with slash commands, reaction roles, automated moderation, and a web dashboard. This validates the Wasm runtime end to end — outbound WebSocket connections, tick-based processing, async HTTP, and workflow integration.

---

## What We're Building

A Discord bot that:

- **Connects to Discord** via the gateway WebSocket
- **Responds to commands** — `/ban`, `/warn`, `/stats` via Discord's REST API
- **Reaction roles** — users react to a message to get a role assigned
- **Auto-moderation** — detects spam patterns, triggers warning/ban workflows
- **Web dashboard** — HTTP API for moderators to view logs, configure rules
- **Audit logging** — all moderation actions logged to database via workflows

---

## Services Required

| Instance | Plugin | Purpose |
|---|---|---|
| `main-db` | `postgres` | Audit logs, server config, user warnings |
| `app-cache` | `cache` | Rate limiting state, spam detection counters |
| `main-stream` | `stream` | Durable events for audit log processing |
| `realtime` | `pubsub` | Cross-instance WebSocket sync for dashboard |

---

## Config Structure

```
noda.json                   — services, JWT, Wasm runtime config
routes/
  dashboard.json            — moderator dashboard API
connections/
  dashboard-ws.json         — live dashboard updates
workflows/
  ban-user.json
  warn-user.json
  get-mod-stats.json
  log-action.json
workers/
  process-audit-log.json
wasm/
  discord_bot.wasm          — the bot module
```

---

## Wasm Runtime Configuration

```json
{
  "wasm_runtimes": {
    "discord-bot": {
      "module": "wasm/discord_bot.wasm",
      "tick_rate": 10,
      "encoding": "json",
      "services": ["app-cache", "main-stream"],
      "connections": ["dashboard-updates"],
      "allow_outbound": {
        "http": ["discord.com", "cdn.discordapp.com"],
        "ws": ["gateway.discord.gg"]
      },
      "config": {
        "token": "{{ $env('DISCORD_BOT_TOKEN') }}",
        "guild_id": "{{ $env('DISCORD_GUILD_ID') }}"
      }
    }
  }
}
```

The module has access to cache (for spam counters), stream (for emitting audit events), and the dashboard WebSocket endpoint. It can reach Discord's API and gateway.

---

## Bot Module Lifecycle

### Initialize

1. Read token from config (already resolved by Noda from `$env()`)
2. `noda_call("", "ws_connect", ...)` — connect to `gateway.discord.gg`
3. Receive HELLO from Discord, configure heartbeat via `noda_call("", "ws_configure", ...)`
4. Send IDENTIFY to authenticate
5. `noda_call("", "set_timer", { "name": "cleanup-spam-counters", "interval": 60000 })`

### Tick Processing

Each tick at 10Hz, the module processes:

**`incoming_ws`** — Discord gateway events:

- `MESSAGE_CREATE` — check for commands (`/ban`, `/warn`, `/stats`) and spam patterns
  - Commands: parse arguments, validate permissions in-memory, then either handle directly or trigger a workflow
  - Spam detection: increment counters in cache via `noda_call("app-cache", "set", ...)`, check thresholds
- `MESSAGE_REACTION_ADD` — check if reaction is on a role-assignment message, trigger workflow to assign role
- `GUILD_MEMBER_JOIN` — send welcome message via async HTTP

**`connection_events`** — handle reconnection:

- On `"reconnected"`: send RESUME with session_id and last sequence number

**`responses`** — results from previous async HTTP calls:

- Check if Discord API calls succeeded, log failures

**`timers`** — periodic cleanup:

- `cleanup-spam-counters`: clear stale spam detection state from cache

### Key Module Decisions

**What the module handles directly (in-memory):**
- Spam detection logic (pattern matching, counter thresholds)
- Command parsing and permission checking
- Session state (sequence numbers, session ID for resume)
- Rate limiting for Discord API calls

**What the module delegates to workflows (via `noda_call("", "trigger_workflow", ...)`):**
- `ban-user` — database write (audit log), Discord API call (might need retry logic), notification to dashboard
- `warn-user` — database write, increment warning count, check if auto-ban threshold reached
- `log-action` — write to audit log database, emit event for dashboard

**Why this split:** The module is fast and ephemeral — it processes events and makes decisions. Workflows are durable and composable — they handle database transactions, retries, and multi-step operations.

---

## Workflow Integration

### ban-user Workflow

**Triggered by:** Wasm module via `noda_call("", "trigger_workflow", { "workflow": "ban-user", "input": {...} })`

**Input:** `{ "guild_id", "user_id", "moderator_id", "reason" }`

**Nodes:**

1. `workflow.run` (transaction: true) — sub-workflow `log-and-ban`:
   - `db.create` — insert audit log entry
   - `db.update` — update user record with ban status
   - `workflow.output` (name: "done")
2. `event.emit` — emit `moderation.action` to stream for dashboard workers
3. `ws.send` — push `{ "type": "ban", ... }` to dashboard WebSocket for live updates

**Features exercised:** Wasm triggering workflows, database transactions via sub-workflow, event emission, WebSocket push to dashboard.

### get-mod-stats Workflow

**Triggered by:** `GET /api/dashboard/stats` (HTTP) AND `wasm.query` from the bot module

The same workflow serves both the dashboard API and the bot's `/stats` command. It queries the database for moderation statistics and returns them.

When triggered via HTTP: `response.json` sends the data to the client.
When triggered via `wasm.query`: the workflow returns data to the bot module, which formats it as a Discord embed and sends it via async HTTP.

**Features exercised:** Same workflow reused across different trigger types, `wasm.query` for synchronous data access.

---

## Dashboard

The web dashboard is a standard HTTP + WebSocket setup:

- REST API routes for viewing audit logs, configuring rules, managing warnings
- WebSocket endpoint (`dashboard-updates`) pushes live moderation events
- The Wasm module pushes to this endpoint whenever a moderation action occurs via `noda_call("dashboard-updates", "send", ...)`
- Workers processing audit log events also push updates

---

## Data Flow: Spam Detection → Auto-Ban

1. User sends a message in Discord
2. Discord gateway delivers `MESSAGE_CREATE` to Noda
3. Noda buffers it, delivers in next tick's `incoming_ws`
4. Bot module processes the message:
   - Checks content against spam patterns (in-memory regex/rules)
   - Increments spam counter: `noda_call("app-cache", "set", { "key": "spam:user-123", "value": 5, "ttl": 60 })`
   - Counter exceeds threshold (5 messages in 60s)
5. Bot triggers auto-ban: `noda_call("", "trigger_workflow", { "workflow": "ban-user", "input": { "reason": "auto-spam" } })`
6. Ban workflow runs asynchronously — writes audit log, bans user via Discord API
7. Bot sends warning to channel: `noda_call_async("", "http_request", { POST discord channel message })`
8. Dashboard receives live update via WebSocket push

---

## Architecture Features Validated

| Feature | How it's used |
|---|---|
| Wasm tick-based execution | Bot processes Discord events at 10Hz |
| Outbound WebSocket (managed) | Discord gateway connection with auto-heartbeat |
| `noda_call_async` HTTP | Discord REST API calls (send messages, ban users) |
| `noda_call` cache | Spam detection counters with TTL |
| `noda_call` stream emit | Audit events for dashboard workers |
| `noda_call` trigger_workflow | Complex operations delegated to durable workflows |
| Connection events | Handle Discord gateway reconnection (RESUME) |
| Timers | Periodic spam counter cleanup |
| `wasm.query` | Dashboard queries bot for live stats |
| `wasm.send` | Dashboard sends config updates to bot |
| `$env()` resolution | Bot token injected from environment |
| Workflow reuse | Stats workflow used by both HTTP and wasm.query |
| Workers | Audit log processing |
| WebSocket push from Wasm | Bot pushes moderation events to dashboard |

---

## What's NOT Needed

No SSE, no scheduler, no storage, no image processing, no file uploads. The bot is cache + stream + HTTP + WebSocket.
