# Cookbook: livekit nodes

Runnable examples for Noda's `lk.*` nodes against a real LiveKit server:
rooms, tokens, participants, data messaging, egress (recording), and ingress
(streaming in).

> **Status: part 2 of 2 (egress + ingress).** This file now covers 18 of
> the family's nodes: the 11 from part 1 (rooms, token, participants, data)
> plus the 7 egress/ingress nodes below. The `lk.*` webhook trigger is out
> of scope for this cookbook (it's a route trigger, not a node) — see
> `internal/server/livekit_webhook.go` and `testdata/livekit-example` for
> that integration.

Every request/response below is verified in CI by [`verify.json`](verify.json).

## API-level verification

This cookbook drives the LiveKit **server APIs** only (see the WebRTC
caveat below), and the CI dev container is a bare `livekit-server --dev`
with no egress or ingress worker attached. That shapes what "verified"
means per node:

> **Why the ~20s egress steps stay in CI:** the two `egressStart*` calls
> genuinely block ~20s while the server waits for a worker before returning
> its real `EGRESS_UNAVAILABLE` error. `lk.*` nodes have no per-node timeout
> config, and a workflow-level `timeout` would replace the real API error
> with Noda's generic cancellation — less honest, not faster. The wait is
> the price of asserting the true error path.

| Node | Verified | Observed against the dev server |
|------|----------|----------------------------------|
| `lk.roomCreate` | success path | 201, room created |
| `lk.roomList` | success path | 200, room appears/disappears |
| `lk.roomUpdateMetadata` | success path | 200, metadata replaced |
| `lk.roomDelete` | success path | 200, room removed |
| `lk.token` | success path (token issuance only) | 200, well-formed JWT — never redeemed |
| `lk.participantList` | success path (empty-list case) | 200, honestly empty (no client joined) |
| `lk.sendData` | success path (no-recipient case) | 200, fan-out with nobody to deliver to |
| `lk.participantGet` | error path | 404 `NO_SUCH_PARTICIPANT` |
| `lk.participantUpdate` | error path | 404 `NO_SUCH_PARTICIPANT` |
| `lk.participantRemove` | error path | 404 `NO_SUCH_PARTICIPANT` |
| `lk.muteTrack` | error path | 404 `NO_SUCH_PARTICIPANT` (~3s internal RPC timeout) |
| `lk.egressStartRoomComposite` | error path | 502 — dev server has no egress worker attached |
| `lk.egressStartTrack` | error path | 502 — dev server has no egress worker attached |
| `lk.egressList` | error path | 502 — listing also requires the egress worker |
| `lk.egressStop` | error path | 404 — no such egress exists |
| `lk.ingressCreate` | error path | 502 — dev server has no ingress worker attached |
| `lk.ingressList` | error path | 502 — listing also requires the ingress worker |
| `lk.ingressDelete` | error path | 404 — no such ingress exists |

**What full verification would need:**
- A live WebRTC participant (browser/SDK client, e.g. `examples/video-rooms`
  or the LiveKit CLI) actually joined to `cookbook-room`, to exercise the
  success paths of `lk.participantGet/Update/Remove` and `lk.muteTrack`,
  and to make `lk.token`'s issued JWT actually redeemable.
- A running `livekit-egress` worker (with Redis message bus configured on
  the server) to exercise the success paths of `lk.egressStartRoomComposite`,
  `lk.egressStartTrack`, `lk.egressList`, and `lk.egressStop`.
- A running `livekit-ingress` worker (RTMP/WHIP/URL-pull service, also
  Redis-backed) to exercise the success paths of `lk.ingressCreate`,
  `lk.ingressList`, and `lk.ingressDelete`.

None of that infrastructure is available in the CI container or expected
of a cookbook reader — every workflow below routes both the `success` and
`error` outputs of the node it exercises, so the *observed* behavior
(whichever branch actually fires) is what's asserted in `verify.json`.
That is itself the point of this cookbook: it shows real, honest API
responses rather than a mocked happy path.

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

## Egress and ingress: a second, dedicated room

`verify.json` deletes `cookbook-room` and confirms it's gone from the list
*before* touching egress/ingress at all, then creates a second room,
`cookbook-egress-room`, to exercise the 7 nodes below. This isn't just
tidiness: the two egress-start calls each take ~20s to time out against a
workerless dev server, and running them against the same room used for the
earlier participant/mute-track assertions was observed to occasionally
destabilize that room's later `lk.roomDelete` call (an intermittent `500`).
Giving egress/ingress their own room keeps the two narratives — "room
lifecycle + participants" and "recording + streaming" — independent, so a
quirk in one can't bleed into the other.

## lk.egressStartRoomComposite — `POST /api/egress/room-composite`

Starts a room composite recording (all audio/video tracks, `file` output to
`/out/recording.mp4`). **Observed:** on a bare `--dev` server with no egress
worker attached, the request eventually fails — LiveKit accepts the twirp
call, waits for an egress worker to pick up the job, and after roughly
20 seconds gives up. The node's `error` output fires; the workflow routes
it to `502 EGRESS_UNAVAILABLE`.

```bash
curl -X POST localhost:3000/api/egress/room-composite \
  -H 'Content-Type: application/json' -d '{"room": "cookbook-egress-room"}'
# → 502 {"error":{"code":"EGRESS_UNAVAILABLE","message":"Room composite egress could not be started","trace_id":"..."}}
```

## lk.egressStartTrack — `POST /api/egress/track`

Starts a single-track recording (same `output` shape, same egress-worker
dependency). **Observed:** identical to room composite egress — ~20s wait,
then `502 EGRESS_UNAVAILABLE`.

```bash
curl -X POST localhost:3000/api/egress/track \
  -H 'Content-Type: application/json' \
  -d '{"room": "cookbook-egress-room", "track_sid": "TR_fake"}'
# → 502 {"error":{"code":"EGRESS_UNAVAILABLE","message":"Track egress could not be started","trace_id":"..."}}
```

## lk.egressList — `GET /api/egress`

Lists egress recordings. **Observed:** even listing requires the egress
worker to be reachable on this LiveKit version — it errors the same way
the two start-egress calls do, rather than returning an empty array.

```bash
curl localhost:3000/api/egress
# → 502 {"error":{"code":"EGRESS_UNAVAILABLE","message":"Egress recordings could not be listed","trace_id":"..."}}
```

## lk.egressStop — `POST /api/egress/:egress_id/stop`

Stops an active egress. **Observed:** against a nonexistent egress ID,
this one *does* return promptly — LiveKit can reject an unknown egress ID
from room-state lookup without needing the worker, so it 404s in ~3s
rather than timing out at ~20s like the start/list calls.

```bash
curl -X POST localhost:3000/api/egress/EG_nonexistent/stop
# → 404 {"error":{"code":"EGRESS_NOT_FOUND","message":"Egress not found","trace_id":"..."}}
```

## lk.ingressCreate — `POST /api/ingress`

Creates an ingress endpoint (`url` input type, pulling from an external
`.m3u8` source). **Observed:** unlike room/participant state, ingress
creation on this dev server requires an ingress worker too — the twirp
call fails immediately (no multi-second wait) and the workflow routes the
node's `error` output to `502 INGRESS_UNAVAILABLE`.

```bash
curl -X POST localhost:3000/api/ingress -H 'Content-Type: application/json' \
  -d '{"input_type": "url", "room": "cookbook-egress-room", "participant_identity": "streamer", "url": "https://example.com/stream.m3u8"}'
# → 502 {"error":{"code":"INGRESS_UNAVAILABLE","message":"Ingress could not be created","trace_id":"..."}}
```

## lk.ingressList — `GET /api/ingress`

Lists ingress endpoints. **Observed:** same as egress list — requires the
worker, errors rather than returning an empty array.

```bash
curl localhost:3000/api/ingress
# → 502 {"error":{"code":"INGRESS_UNAVAILABLE","message":"Ingress endpoints could not be listed","trace_id":"..."}}
```

## lk.ingressDelete — `DELETE /api/ingress/:ingress_id`

Deletes an ingress endpoint. **Observed:** like egress-stop, a delete on a
nonexistent ID resolves from state lookup without the worker, so it 404s
rather than timing out.

```bash
curl -X DELETE localhost:3000/api/ingress/IN_nonexistent
# → 404 {"error":{"code":"INGRESS_NOT_FOUND","message":"Ingress not found","trace_id":"..."}}
```

## Test

```bash
go test -tags=integration ./internal/testing/cookbook/ -run 'TestCookbook/livekit' -v
```
