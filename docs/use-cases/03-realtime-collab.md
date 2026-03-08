# Noda — Use Case: Real-Time Collaboration

**Version**: 0.4.0

A collaborative document editing backend with live presence (who's online), real-time updates (edits broadcast to all viewers), and cursor tracking. This validates the WebSocket connection manager, routing table, and the workflow-to-WebSocket pipeline.

---

## What We're Building

A backend for a collaborative workspace (think Notion-like):

- **Documents** — CRUD via REST API
- **Live editing** — clients connect via WebSocket, receive real-time updates when anyone edits
- **Presence** — see who's currently viewing a document
- **Cursor tracking** — see collaborators' cursor positions in real time
- **Edit history** — changes are logged for undo/audit

---

## Services Required

| Instance | Plugin | Purpose |
|---|---|---|
| `main-db` | `postgres` | Documents, users, edit history |
| `app-cache` | `cache` | Presence tracking (who's viewing which doc) |
| `realtime` | `pubsub` | Cross-instance WebSocket sync |

---

## Config Structure

```
noda.json                  — services, JWT, connections config
schemas/
  Document.json
  EditOperation.json
routes/
  documents.json           — CRUD endpoints
connections/
  collaboration.json       — WebSocket endpoint config
workflows/
  create-document.json
  update-document.json
  get-document.json
  ws-on-connect.json
  ws-on-message.json
  ws-on-disconnect.json
```

---

## Connection Configuration

```json
{
  "connections": {
    "sync": {
      "pubsub": "realtime"
    },
    "endpoints": {
      "collab": {
        "type": "websocket",
        "path": "/ws/documents/:doc_id",
        "middleware": ["auth.jwt"],
        "channels": {
          "pattern": "doc.{{ request.params.doc_id }}"
        },
        "max_per_channel": 50,
        "ping_interval": "30s",
        "on_connect": "ws-on-connect",
        "on_message": "ws-on-message",
        "on_disconnect": "ws-on-disconnect"
      }
    }
  }
}
```

Each document gets its own channel: `doc.abc-123`. All clients viewing that document are on the same channel. The connection manager handles multi-instance routing via the Redis routing table.

---

## Key Workflows

### WebSocket Connect — Join Document

**Trigger:** Client connects to `/ws/documents/:doc_id` → workflow `ws-on-connect`

**Input mapping:** `{ "doc_id": "{{ request.params.doc_id }}", "user_id": "{{ auth.sub }}", "channel": "doc.{{ request.params.doc_id }}" }`

**Nodes:**

1. `db.query` — verify document exists and user has access
2. `control.if` — does the document exist?
   - `else` (not found) → workflow ends (connection will be closed)
   - `then` →
3. `cache.set` — add user to presence set: key `presence:{{ input.doc_id }}`, value includes user ID and timestamp
4. `cache.get` — read current presence set for this document
5. `ws.send` — broadcast `{ "type": "user_joined", "user_id": "...", "presence": [...] }` to channel `doc.{{ input.doc_id }}`

**Features exercised:** WebSocket lifecycle events triggering workflows, auth context in WebSocket connections, cache for presence tracking, broadcasting to a channel.

### WebSocket Message — Handle Edit

**Trigger:** Client sends a message → workflow `ws-on-message`

The client sends edit operations: `{ "type": "edit", "operations": [...] }` or cursor updates: `{ "type": "cursor", "position": { "line": 10, "col": 5 } }`.

**Nodes:**

1. `control.switch` on `{{ input.data.type }}`
   - `"edit"` branch:
     1. `transform.validate` — validate operations against EditOperation schema
     2. `db.create` — insert edit operations into history table
     3. `db.exec` — apply operations to document content
     4. `ws.send` — broadcast `{ "type": "edit", "user_id": "...", "operations": [...] }` to the channel (all viewers see the edit)
   - `"cursor"` branch:
     1. `ws.send` — broadcast `{ "type": "cursor", "user_id": "...", "position": {...} }` to the channel
   - `default` → `util.log` (unknown message type)

Cursor updates skip the database entirely — they're ephemeral, broadcast only. Edits go through validation and persistence.

**Features exercised:** `control.switch` for message routing, different processing paths per message type, database write + WebSocket broadcast in sequence, ephemeral data (cursors) vs persistent data (edits).

### WebSocket Disconnect — Leave Document

**Trigger:** Client disconnects → workflow `ws-on-disconnect`

**Nodes:**

1. `cache.del` — remove user from presence set
2. `cache.get` — read updated presence
3. `ws.send` — broadcast `{ "type": "user_left", "user_id": "...", "presence": [...] }` to the channel

**Features exercised:** Cleanup on disconnect, presence update broadcast.

### REST Edit — Non-WebSocket Update

**Trigger:** `PUT /api/documents/:id` → workflow `update-document`

For clients that edit via REST (mobile apps, API integrations) rather than WebSocket:

**Nodes:**

1. `transform.validate` — validate input
2. `db.update` — update document
3. `ws.send` — broadcast the change to all connected WebSocket viewers
4. `response.json` — return 200

The REST endpoint pushes to WebSocket viewers. The workflow doesn't know or care if anyone is connected — `ws.send` delivers to whoever is on the channel (possibly nobody).

**Features exercised:** HTTP endpoint pushing to WebSocket channels, workflows bridging REST and real-time.

---

## Multi-Instance Data Flow

When two users are editing the same document but connected to different Noda instances:

1. User A (on instance 1) sends an edit via WebSocket
2. Instance 1 runs `ws-on-message` workflow, persists to DB, calls `ws.send` to channel `doc.abc-123`
3. The `ws.send` node calls the connection manager, which checks the Redis routing table
4. Routing table shows: `doc.abc-123` has connections on instance 1 AND instance 2
5. Instance 1 delivers locally to its connections AND publishes to instance 2 via PubSub
6. Instance 2 receives the PubSub message and delivers to User B's WebSocket

The routing table enables targeted delivery — only instances holding relevant connections receive the message.

---

## Presence Implementation Detail

Presence uses Redis cache with per-user keys:

- `presence:doc-123:user-abc` → `{ "user_id": "abc", "name": "Alice", "connected_at": "..." }` with TTL = 60s
- Each client's connection refreshes the TTL via the ping interval
- To get all viewers: `cache.get` with a prefix pattern, or maintain a separate set key

If a user's connection drops without a clean disconnect (instance crash), the TTL expires and they automatically disappear from presence.

---

## Architecture Features Validated

| Feature | How it's used |
|---|---|
| WebSocket connection manager | Client connections for live editing |
| Connection lifecycle workflows | `on_connect`, `on_message`, `on_disconnect` |
| Channel-based messaging | Each document = one channel |
| Redis routing table | Cross-instance targeted delivery |
| Auth in WebSocket | JWT middleware on WebSocket endpoint |
| Cache for presence | Ephemeral user presence with TTL |
| `control.switch` | Route different message types |
| REST-to-WebSocket bridge | HTTP edit endpoint broadcasts to WebSocket viewers |
| Implicit parallelism | DB write and WebSocket broadcast can overlap |
| Standardized errors | Invalid edits return structured errors |

---

## What's NOT Needed

No Wasm, no workers, no scheduler, no storage, no image processing, no events/streams. Pure HTTP + WebSocket + database + cache.
