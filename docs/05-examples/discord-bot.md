# Noda — Use Case: Discord Bot

**Version**: 0.4.0

A Discord gateway bot that answers chat commands (`!ping`, `!roll`), implemented entirely as a Wasm module. This validates the Wasm runtime end to end — an outbound WebSocket connection with automatic heartbeats and resume state, tick-based event processing, and async host calls.

The runnable project lives in [`examples/discord-bot/`](../../examples/discord-bot).

---

## What We're Building

A bot that:

- **Connects to Discord** via the gateway WebSocket (`wss://gateway.discord.gg`), identifying with a bot token
- **Keeps the session alive** — heartbeats are configured once and then sent automatically by Noda's WS gateway; the module tracks the sequence number and session ID for resume
- **Responds to commands** — `!ping` replies "Pong!"; `!roll` rolls a die and posts the result via Discord's REST API
- **Demonstrates async host calls** — `!roll` fires an async call and sends the reply only when the response arrives in a later tick

There is no HTTP API, no database, and no dashboard in this example — it is deliberately Wasm-only, so the entire project is one `noda.json` plus the module source.

---

## Config Structure

```
noda.json                   — the wasm_runtimes block (nothing else)
docker-compose.yml          — runs Noda with DISCORD_BOT_TOKEN passed through
wasm/
  bot/                      — Go module source (tinygo)
  bot.wasm                  — compiled module
```

---

## Wasm Runtime Configuration

```json
{
  "wasm_runtimes": {
    "discord-bot": {
      "module": "wasm/bot.wasm",
      "tick_rate": 2,
      "encoding": "json",
      "config": {
        "token": "{{ $env('DISCORD_BOT_TOKEN') }}"
      },
      "allow_outbound": {
        "ws": ["gateway.discord.gg"],
        "http": ["discord.com"]
      }
    }
  }
}
```

A 2 Hz tick is plenty for a chat bot. `allow_outbound` whitelists exactly two hosts: the gateway WebSocket and Discord's REST API. The module has no `services` or `connections` grants — it needs neither.

---

## Bot Module Lifecycle

### Initialize

1. `noda.GetInitInput()` — read the bot token from config (already resolved by Noda from `$env()`)
2. `noda.WSConnect("discord", "wss://gateway.discord.gg/?v=10&encoding=json", nil)` — open the outbound gateway connection

### Gateway Handshake (in tick)

1. Discord sends HELLO with a `heartbeat_interval`
2. The module calls `noda.WSConfigure("discord", heartbeatInterval, {"op": 1, "d": lastSequence})` — from then on, **Noda's gateway sends the heartbeat frames automatically** at that interval
3. The module sends IDENTIFY with the token via `noda.WSSend`

### Tick Processing

Each tick the module processes:

- **`incoming_ws`** — gateway frames: dispatch events (tracking the sequence number and session ID), including `MESSAGE_CREATE`, where it matches `!ping` and `!roll` (ignoring messages from bots)
- **`responses`** — results of previous `noda.CallAsync` calls: for `!roll`, the pending reply is looked up by its label and only then sent to the channel via Discord's REST API
- **`connection_events`** — reconnects, where the tracked resume state matters

### Key Module Decisions

- **State lives in Wasm linear memory** — the sequence number, session ID, and pending async replies are plain Go globals; they persist across ticks because the module instance is long-lived.
- **Async labels are correlated manually** — each `!roll` gets a unique label (`roll-<n>`); the response for that label arrives in a later tick's `responses` map.
- **Heartbeats are Noda's job** — `WSConfigure` hands the keepalive to the host, so a slow tick can't kill the gateway session.

---

## Architecture Features Validated

| Feature | How it's used |
|---|---|
| Wasm tick-based execution | Bot processes Discord events at 2 Hz |
| Outbound WebSocket (managed) | Discord gateway connection with auto-heartbeat |
| `allow_outbound` whitelisting | Only `gateway.discord.gg` (WS) and `discord.com` (HTTP) are reachable |
| Async host calls + `responses` | `!roll` reply sent only after its async call completes |
| Connection events | Discord gateway reconnect handling (RESUME state) |
| `$env()` resolution | Bot token injected from the environment at config load |

---

## Running It

```bash
cd examples/discord-bot
DISCORD_BOT_TOKEN=<your bot token> docker compose up --build
```

Invite the bot to a server (with the Message Content intent enabled), then type `!ping` or `!roll` in any channel it can read.

---

## Extending It

The natural next steps all use pieces this example deliberately leaves out:

- **Moderation workflows** — have the module call `noda.TriggerWorkflow("ban-user", {...})` and implement audit-logged moderation as normal Noda workflows backed by a `db` service
- **A moderator dashboard** — add `routes/` for a REST API and a `connections/` WebSocket endpoint, and grant the module `connections: ["dashboard"]` so it can push live events
- **Spam detection** — grant the module a `cache` service and keep sliding-window counters per user

See the [Wasm development guide](../04-guides/wasm-development.md) for the full host API.
