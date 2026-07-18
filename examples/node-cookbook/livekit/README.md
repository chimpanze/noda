# Cookbook: livekit nodes

Runnable examples for Noda's `lk.*` nodes against a real LiveKit server:
rooms, tokens, participants, and data messaging.

> **Status: part 1 of 2.** This file currently covers the 11 nodes below
> (rooms, token, participants, data). A follow-up tranche appends egress,
> ingress, and webhook nodes — extend the tables/sections here rather than
> starting a new file.

Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

This project needs a real LiveKit server — CI's cookbook walker starts one
via testcontainers automatically (see `deps: ["livekit"]` in `verify.json`).
To run it yourself:

```bash
docker run -d --name cookbook-livekit -p 7880:7880 -p 7881:7881 -p 7882:7882/udp \
  livekit/livekit-server:v1.9 --dev --bind 0.0.0.0

export LIVEKIT_URL='ws://localhost:7880'
export LIVEKIT_API_KEY='devkey'
export LIVEKIT_API_SECRET='secret'

go run ./cmd/noda validate --config examples/node-cookbook/livekit
go run ./cmd/noda start --config examples/node-cookbook/livekit
```

## Important caveat: no connected WebRTC participants

This cookbook drives the LiveKit **server APIs** only — it never opens an
actual WebRTC connection (there's no browser/SDK client in the loop). That
means:

- `lk.token` issues a real, usable JWT, but nothing ever redeems it.
- `lk.participantList` on a freshly created room legitimately returns an
  **empty array** — that's the honest state, not a bug.
- `lk.participantGet`, `lk.participantUpdate`, `lk.participantRemove`, and
  `lk.muteTrack` all target a nonexistent identity (`"ghost"`) in a real,
  created room. These four are **API-level error-path demonstrations**;
  exercising their success path requires a live WebRTC participant (e.g. the
  `examples/video-rooms` frontend, or the LiveKit CLI/SDK) actually joined to
  the room.

## lk.token — `POST /api/rooms/:room/token`

Issues a signed LiveKit access token for a room/identity pair.

```bash
curl -X POST localhost:3000/api/rooms/cookbook-room/token \
  -H 'Content-Type: application/json' \
  -d '{"identity": "alice", "name": "Alice"}'
# → 200 {"token":"eyJ...","identity":"alice","room":"cookbook-room"}
```

## lk.roomCreate — `POST /api/rooms`

Creates a room (or returns the existing one if the name is already taken).

```bash
curl -X POST localhost:3000/api/rooms -H 'Content-Type: application/json' \
  -d '{"name": "cookbook-room"}'
# → 201 {"sid":"RM_...","name":"cookbook-room","empty_timeout":600,...}
```

## lk.roomList — `GET /api/rooms`

Lists all active rooms.

```bash
curl localhost:3000/api/rooms
# → 200 {"rooms":[{"sid":"RM_...","name":"cookbook-room",...}]}
```

## lk.roomUpdateMetadata — `PUT /api/rooms/:room/metadata`

Replaces a room's metadata.

```bash
curl -X PUT localhost:3000/api/rooms/cookbook-room/metadata \
  -H 'Content-Type: application/json' \
  -d '{"metadata": "{\"purpose\":\"updated\"}"}'
# → 200 {"sid":"RM_...","name":"cookbook-room","metadata":"{\"purpose\":\"updated\"}",...}
```

## lk.roomDelete — `DELETE /api/rooms/:room`

Deletes a room; all participants would be disconnected.

```bash
curl -X DELETE localhost:3000/api/rooms/cookbook-room
# → 200 {"deleted":true}
```

## lk.participantList — `GET /api/rooms/:room/participants`

Lists connected participants. With no WebRTC client joined, this is
legitimately empty.

```bash
curl localhost:3000/api/rooms/cookbook-room/participants
# → 200 {"participants":[]}
```

## lk.sendData — `POST /api/rooms/:room/data`

Broadcasts a data message on the room's data channel. **Observed:**
broadcasting into a room with zero connected participants still succeeds
(it's a fan-out with nobody to deliver to, not an error) — the node reports
`{"sent":true}` regardless.

```bash
curl -X POST localhost:3000/api/rooms/cookbook-room/data \
  -H 'Content-Type: application/json' \
  -d '{"data": {"kind": "cookbook-ping"}, "topic": "cookbook"}'
# → 200 {"sent":true}
```

## lk.participantGet / lk.participantUpdate / lk.participantRemove / lk.muteTrack — error-path demos

All four target identity `"ghost"` in a real, created room. Since no such
participant is (or can be) connected, the underlying LiveKit twirp RPC fails
and the node's `error` output fires; each workflow routes that to a
`response.error` with `404 NO_SUCH_PARTICIPANT`.

**Observed twirp error mapping:**

| Node | Route | Observed latency | Observed status |
|------|-------|-------------------|------------------|
| `lk.participantGet` | `GET /api/rooms/:room/participants/:identity` | < 1ms | 404 `NO_SUCH_PARTICIPANT` |
| `lk.participantUpdate` | `PUT /api/rooms/:room/participants/:identity` | < 1ms | 404 `NO_SUCH_PARTICIPANT` |
| `lk.participantRemove` | `DELETE /api/rooms/:room/participants/:identity` | < 1ms | 404 `NO_SUCH_PARTICIPANT` |
| `lk.muteTrack` | `POST /api/rooms/:room/participants/:identity/tracks/:track_sid/mute` | **~3s** | 404 `NO_SUCH_PARTICIPANT` |

`lk.muteTrack` against a nonexistent participant is markedly slower than the
other three (~3 seconds vs. sub-millisecond) — LiveKit's server appears to
attempt an internal RPC to the participant's (nonexistent) signal connection
and only fails after an internal ~3s timeout, rather than rejecting
immediately from room-state lookup like the others do. `verify.json` sets
`"listen": true` specifically to give this step a real-transport HTTP client
(no fixed 1s in-process test timeout) so this slow, but legitimate, response
isn't mistaken for a hang.

```bash
curl localhost:3000/api/rooms/cookbook-room/participants/ghost
# → 404 {"error":{"code":"NO_SUCH_PARTICIPANT","message":"Participant not found in room","trace_id":"..."}}
```

## Test

```bash
go test -tags=integration ./internal/testing/cookbook/ -run 'TestCookbook/livekit' -v
```
