# Video Rooms — LiveKit Example

Room management and join-token issuing backed by a LiveKit server, plus a
static browser frontend (`frontend/index.html`).

## Endpoints

| Method | Path | Body | Description |
|--------|------|------|-------------|
| POST | `/api/rooms` | `{"name": "standup"}` | Create a room |
| GET | `/api/rooms` | — | List rooms |
| POST | `/api/rooms/:room/join` | `{"name": "display name"}` | Get a LiveKit join token (identity is generated server-side) |
| GET | `/api/rooms/:room/participants` | — | List participants |

## Running

```bash
cp .env.example .env      # local defaults target the compose LiveKit below
docker compose up -d      # LiveKit dev server (placeholder keys devkey/secret)
noda start --config .     # API on http://localhost:8080
```

## Try it

```bash
curl -X POST localhost:8080/api/rooms -H 'Content-Type: application/json' \
  -d '{"name":"standup"}'
# → 201 {"room":{"name":"standup","sid":"RM_...",...}}

curl localhost:8080/api/rooms
# → 200 {"rooms":[...]}

curl -X POST localhost:8080/api/rooms/standup/join \
  -H 'Content-Type: application/json' -d '{"name":"marten"}'
# → 200 {"identity":"<uuid>","name":"marten","room":"standup","token":"eyJ..."}
# NOTE: "name" is required — omitting it fails the lk.token node.

curl localhost:8080/api/rooms/standup/participants
# → 200 {"participants":[]}
```

## Frontend

With the API and LiveKit running, open `frontend/index.html` in a browser
(CORS is `*` in this example). Create or join a room from the lobby; media
goes to the LiveKit server from the compose file.
