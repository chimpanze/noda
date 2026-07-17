# Discord Bot — outbound-WebSocket Wasm example

A Wasm module that connects to the Discord Gateway from inside Noda's Wasm
runtime and answers `!roll` (dice roll) messages. Demonstrates
`wasm_runtimes` with `allow_outbound` (WebSocket + HTTP egress allowlists),
tick-driven gateway handling, and async host calls.

## Prerequisites

1. A Discord application with a **bot token** — <https://discord.com/developers/applications> → New Application → Bot → Reset Token.
2. Enable the **Message Content Intent** (Bot → Privileged Gateway Intents), or the bot cannot read `!roll` messages.
3. Invite the bot to a server: OAuth2 → URL Generator → scope `bot` → permissions "Send Messages" → open the generated URL.

## Running

```bash
export DISCORD_BOT_TOKEN=your-bot-token
noda start --config .            # local binary
# or, containerized:
DISCORD_BOT_TOKEN=your-bot-token docker compose up --build
```

Then type `!roll` in a channel the bot can read.

## Rebuilding the Wasm module

```bash
cd wasm/bot
tinygo build -o ../bot.wasm -target wasi -buildmode=c-shared .
```

## Without a token

There are no HTTP routes in this example — everything happens over the
gateway connection. Starting with an invalid token still exercises most of
the plumbing (module load, initialize, outbound WS connect, HELLO,
IDENTIFY) before Discord rejects the session; watch the logs.
