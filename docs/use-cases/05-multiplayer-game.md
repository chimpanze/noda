# Noda — Use Case: Multiplayer Game Server

**Version**: 0.4.0

A real-time multiplayer game with a tick-based game loop, player input processing, physics simulation, state broadcasting, matchmaking, leaderboards, and a spectator mode. This is the most demanding use case — it validates the full Wasm tick model under real-time pressure.

---

## What We're Building

A 2D multiplayer arena game (up to 50 players per lobby):

- **Game loop** — 20Hz tick-based simulation (physics, collision, game logic)
- **Player input** — movement and actions via WebSocket, processed per tick
- **State broadcasting** — game state delta sent to all players each tick
- **Matchmaking** — HTTP API to create/join lobbies, assigned to game instances
- **Leaderboard** — top scores queryable via HTTP API
- **Spectator mode** — read-only WebSocket viewers see the same state updates
- **Persistence** — game state snapshots, player stats saved to database via workflows

---

## Services Required

| Instance | Plugin | Purpose |
|---|---|---|
| `main-db` | `postgres` | Player accounts, stats, match history |
| `app-cache` | `cache` | Leaderboard cache, lobby state |
| `game-storage` | `storage` | Game state snapshots (S3) |
| `main-stream` | `stream` | Match events for stats processing |
| `realtime` | `pubsub` | Cross-instance WebSocket routing |

---

## Config Structure

```
noda.json                    — services, JWT, connections, Wasm runtime
routes/
  matchmaking.json           — create/join/leave lobby
  leaderboard.json           — top scores API
  player.json                — player profile API
connections/
  game.json                  — WebSocket endpoint for players
workflows/
  create-lobby.json
  join-lobby.json
  leave-lobby.json
  get-leaderboard.json
  save-match-result.json
  update-player-stats.json
workers/
  process-match-events.json
wasm/
  game_server.wasm           — the game engine module
```

---

## Wasm Runtime Configuration

```json
{
  "wasm_runtimes": {
    "game": {
      "module": "wasm/game_server.wasm",
      "tick_rate": 20,
      "encoding": "msgpack",
      "services": ["app-cache", "game-storage", "main-stream"],
      "connections": ["game-play"],
      "config": {
        "max_players_per_lobby": 50,
        "world_width": 2000,
        "world_height": 2000,
        "tick_rate": 20
      }
    }
  }
}
```

Key decisions:
- **20Hz tick rate** — 50ms per tick, standard for action games
- **MessagePack encoding** — reduced serialization overhead at 20Hz with 50 player inputs per tick
- **No outbound HTTP/WS** — the game module only talks to Noda services, no external APIs
- **Cache for leaderboard** — fast reads for `wasm.query`, updated periodically
- **Storage for snapshots** — async writes via `noda_call_async`
- **Stream for match events** — stats processing done by workers

---

## Connection Configuration

```json
{
  "connections": {
    "sync": { "pubsub": "realtime" },
    "endpoints": {
      "game-play": {
        "type": "websocket",
        "path": "/ws/game/:lobby_id",
        "middleware": ["auth.jwt"],
        "channels": {
          "pattern": "game.{{ request.params.lobby_id }}"
        },
        "max_per_channel": 100,
        "ping_interval": "15s",
        "max_message_size": "4kb",
        "on_connect": "join-lobby",
        "on_message": "ws-game-input",
        "on_disconnect": "leave-lobby"
      }
    }
  }
}
```

Each lobby is a channel: `game.lobby-abc`. Players and spectators share the channel. The `on_connect` triggers the `join-lobby` workflow, which validates the player and notifies the Wasm module.

---

## Game Module Lifecycle

### Initialize

1. Read game config (world size, max players)
2. Allocate ECS (Entity Component System) in Wasm linear memory
3. Load any saved state: `noda_call("game-storage", "read", { "path": "lobbies/active.json" })`
4. Set timers:
   - `save-snapshot` every 5 seconds
   - `update-leaderboard` every 10 seconds

### Tick (20Hz)

Every 50ms, the module receives and processes:

**`connection_events`** — player joins/leaves:

- `"connect"` → create player entity in ECS, assign spawn position, broadcast `player_joined` to channel
- `"disconnect"` → remove player entity, broadcast `player_left`, if lobby empty → trigger cleanup workflow

**`client_messages`** — player inputs:

Each message: `{ "endpoint": "game-play", "channel": "game.lobby-abc", "user_id": "p1", "data": { "keys": ["W", "SPACE"], "mouse": { "x": 500, "y": 300 } } }`

- Map each input to the player's entity in ECS
- Store inputs for processing in the simulation step

**`commands`** — workflow commands:

- Admin actions (e.g., "kick player" from dashboard → `wasm.send`)
- Matchmaking results (e.g., "start match" after all players ready)

**`responses`** — async call results:

- Snapshot save confirmations
- Error handling for failed saves

**Simulation step:**

1. Apply player inputs to entities
2. Run physics — movement, collision detection, projectiles
3. Run game logic — scoring, power-ups, respawns, cooldowns
4. Compute state delta — what changed since last tick

**Broadcast:**

- `noda_call("game-play", "send", { "channel": "game.lobby-abc", "data": state_delta })`
- Delta includes: entity positions, new/removed entities, score changes, effects
- Players' game clients interpolate between deltas for smooth rendering

**Periodic tasks (timers):**

- `save-snapshot` → `noda_call_async("game-storage", "write", { "path": "lobbies/lobby-abc.json", "data": serialized_state })`
- `update-leaderboard` → `noda_call("app-cache", "set", { "key": "leaderboard:lobby-abc", "value": top_scores, "ttl": 30 })`

### Query (from `wasm.query` workflow nodes)

The leaderboard HTTP API uses `wasm.query` to read live data from the game module's memory:

- `{ "type": "get_leaderboard", "lobby_id": "abc", "limit": 10 }` → returns current scores from in-memory state
- `{ "type": "get_player", "player_id": "p1" }` → returns player stats, position, status
- `{ "type": "get_lobby_status" }` → returns player count, match time, game phase

Queries read directly from Wasm linear memory — no database, no serialization of the full game world. Sub-millisecond response times.

---

## Workflow Integration

### join-lobby (triggered by WebSocket `on_connect`)

**Input:** `{ "lobby_id": "{{ request.params.lobby_id }}", "user_id": "{{ auth.sub }}" }`

**Nodes:**

1. `db.query` — load player profile (name, rank, stats)
2. `control.if` — is the lobby full? (query the Wasm module via `wasm.query`)
   - `then` (full) → close connection gracefully (workflow ends, connection closed)
   - `else` →
3. `wasm.send` — notify game module: `{ "type": "player_ready", "user_id": "...", "profile": {...} }`

The actual player entity creation happens in the Wasm module when it processes the `connection_events` connect event. The workflow just validates and enriches the data.

### get-leaderboard (HTTP API)

**Trigger:** `GET /api/leaderboard/:lobby_id`

**Nodes:**

1. `cache.get` — try cache first (key: `leaderboard:lobby-abc`)
2. `control.if` — cache hit?
   - `then` → `response.json` with cached data
   - `else` → `wasm.query` (get_leaderboard) → `response.json`

Cache is updated by the game module every 10 seconds via timer. Most requests hit cache, reducing load on the Wasm module.

### save-match-result (triggered by game module)

When a match ends, the game module calls `noda_call("", "trigger_workflow", { "workflow": "save-match-result", "input": { match_data } })`.

**Nodes:**

1. `workflow.run` (transaction: true) — sub-workflow:
   - `db.create` — insert match record
   - `control.loop` — for each player: `db.update` player stats (wins, losses, rank)
   - `workflow.output` (name: "saved")
2. `event.emit` — emit `match.completed` to stream for analytics worker

**Features exercised:** Wasm triggers workflow, database transaction wrapping sub-workflow, `control.loop` for per-player updates, event emission for async analytics.

---

## Tick Budget Analysis

At 20Hz, the tick budget is 50ms. Breakdown of a typical tick:

| Phase | Expected time | Notes |
|---|---|---|
| Deserialize tick input (msgpack) | <0.5ms | 50 player inputs ≈ 5KB |
| Process connection events | <0.1ms | Rare per tick |
| Apply player inputs | <0.5ms | Map inputs to entities |
| Physics simulation | 1-5ms | Depends on entity count |
| Game logic | 1-3ms | Scoring, power-ups, respawns |
| Compute state delta | 0.5-1ms | Diff against previous state |
| `noda_call` cache set (leaderboard) | <1ms | Only every 200th tick |
| `noda_call` ws send (broadcast) | <0.5ms | Buffered, non-blocking |
| `noda_call_async` storage write | <0.1ms | Returns immediately |
| Serialize output (msgpack) | <0.1ms | Minimal return data |
| **Total** | **~5-10ms** | **Well within 50ms budget** |

The heaviest operations (storage write, stream emit) are async — they don't count against the tick budget. Synchronous calls (cache, WebSocket send) are sub-millisecond.

---

## Multi-Instance Game Scaling

Each Noda instance runs its own game module with its own lobbies. Scaling:

- **Lobby assignment** — the matchmaking workflow assigns players to a specific Noda instance's lobby. The lobby ID includes the instance identifier.
- **No cross-instance game state** — each lobby runs entirely on one instance. No shared game state between instances.
- **Cross-instance leaderboard** — the global leaderboard aggregates across instances via the database or cache. Workers process match events from all instances.
- **Spectator routing** — spectators connect to the same instance as the lobby via the routing table. The matchmaking API returns the correct WebSocket URL.

---

## Architecture Features Validated

| Feature | How it's used |
|---|---|
| Wasm tick-based execution | 20Hz game loop with physics simulation |
| MessagePack encoding | Reduced serialization overhead at high frequency |
| `client_messages` with user identity | Player inputs with authenticated user ID |
| `connection_events` | Player join/leave detection |
| Timers | Periodic snapshots and leaderboard updates |
| `noda_call` sync (cache, ws) | Fast in-tick operations |
| `noda_call_async` (storage) | State snapshots without blocking ticks |
| `wasm.query` | HTTP API reads live game state from module memory |
| `wasm.send` | Workflows send commands to game module |
| Workflow triggers from Wasm | Match results saved via durable workflows |
| Database transactions | Atomic match result + player stat updates |
| `control.loop` | Per-player stat updates in transaction |
| Cache for read offloading | Leaderboard cached, most HTTP requests skip Wasm |
| Stream events | Match analytics processed by workers |
| Redis routing table | Spectators routed to correct instance |
| Tick budget monitoring | Performance validation during development |

---

## What's NOT Needed

No SSE, no scheduler, no image processing, no email, no file uploads, no outbound HTTP. Pure Wasm + WebSocket + database + cache + storage.
