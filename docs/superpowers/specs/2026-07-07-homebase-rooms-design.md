# Homebase — Rooms Design (Cycle 2: Screen Streaming + Meetings)

**Date:** 2026-07-07
**Status:** Approved design, pre-implementation
**Location:** `projects/homebase/` (extends cycle 1, shipped as PR #299)
**Cycle 1 spec:** `2026-07-07-homebase-foundation-design.md`

## What this cycle adds

Screen streaming (replace Twitch/YouTube/Discord) and meetings with friends
(replace Discord calls), both as LiveKit rooms created and joined through the
Homebase API. API-only, like cycle 1: endpoints return raw LiveKit
credentials (`livekit_url` + room-scoped JWT); clients are whatever the user
pastes them into (a `meet_url` convenience string is included). No frontend.

## Decisions already made

- **LiveKit Cloud** hosts WebRTC (`LIVEKIT_URL`/`LIVEKIT_API_KEY`/
  `LIVEKIT_API_SECRET` env). Self-hosting is a possible later swap — the
  config is just env vars.
- **API returns raw tokens only.** No redirect flow, no pages. Responses
  include a pre-composed `meet_url`
  (`https://meet.livekit.io/custom?liveKitUrl=<enc>&token=<jwt>`) purely as
  a paste convenience.
- **Owner publishes his screen via browser screenshare** using his own
  owner token — no OBS/RTMP ingress this cycle.
- **Reusable room guest links** (cycle-1 share-link pattern): one revocable,
  optionally-expiring link per room; each friend exchanges it for their own
  fresh LiveKit token. Revoking the link stops new joins; deleting the room
  ends the call.
- **State model A — stateless rooms.** LiveKit owns room lifecycle
  (`empty_timeout` auto-deletes abandoned rooms); room type is encoded in
  the room name. Postgres stores only guest links. **No Redis.**

## Room presets

| | `hb-meet-<slug>` | `hb-stream-<slug>` |
|---|---|---|
| purpose | meeting | owner broadcasts screen |
| max_participants | 10 | 50 |
| empty_timeout | 600 s | 600 s |
| owner grants | canPublish, canSubscribe, canPublishData, roomAdmin | canPublish restricted to `canPublishSources: ["SCREEN_SHARE","SCREEN_SHARE_AUDIO","MICROPHONE"]` (values must match the LiveKit `TrackSource` enum names exactly — unknown strings are silently dropped by `plugins/livekit/helpers.go:applyGrants`, which would lock the owner out of publishing), canSubscribe, canPublishData, roomAdmin |
| guest grants | canPublish, canSubscribe, canPublishData | canSubscribe only (`canPublish: false`) |
| owner token TTL | 12h | 12h |
| guest token TTL | 6h (node default) | 6h |

Slug = first 8 chars of a UUIDv4. Guest identity =
`guest-<sanitized name>-<4 random chars>`; owner identity = `owner`.
Guest display name comes from `?name=`, defaults to `guest`, sanitized
(trimmed, capped at 32 chars, control characters stripped).

Grant enforcement is LiveKit-side: a stream guest cannot publish even with a
custom client, because the JWT says so.

## Data model

**`room_links`** (only new table; no FK — the room lives in LiveKit)

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| room_name | text not null | `hb-meet-a1b2c3d4` |
| room_type | text not null | `meeting` \| `stream` — drives guest grants |
| token | text unique | 64 hex chars (two dash-stripped UUIDv4s), plaintext (cycle-1 reasoning) |
| expires_at | timestamptz null | null = until revoked / room deleted |
| created_at | timestamptz | |

Index on `room_name`. Link rows for dead rooms are inert (joins 404 via the
room-existence check) and are removed by room deletion or link rotation.

## API surface

### Owner endpoints (session auth)

| endpoint | behavior |
|---|---|
| `POST /rooms` | `{type: "meeting"\|"stream", expires_in?}` → slug, `lk.roomCreate` with preset, insert guest link → `201 {room, type, livekit_url, guest_token, guest_url, expires_at}`. Ordering: create room, then insert link; on insert failure delete the room (mirror of cycle-1 upload ordering). |
| `GET /rooms` | `lk.roomList` → expression-filter names to `hb-` prefix → `200 {rooms: [...], links: [active room_links rows]}` (two arrays, no merge logic) |
| `DELETE /rooms/:name` | `lk.roomDelete` + delete link rows for the room → `204`; unknown room → `404` |
| `POST /rooms/:name/token` | owner LiveKit token, grants per type derived from name prefix; room must exist → `200 {livekit_url, token, meet_url}`; unknown room → `404` |
| `POST /rooms/:name/link` | rotate: delete existing link rows, insert fresh (optional `expires_in`); room must exist → `201 {guest_token, guest_url, expires_at}` |
| `DELETE /rooms/:name/link` | revoke guest access without ending the call → `204` (idempotent) |

`expires_in` schema pattern `^[0-9]+(s|m|h)$` (as cycle 1);
`type` schema enum. Room-name path params are validated against
`^hb-(meet|stream)-[a-z0-9]{8}$` before any LiveKit call.

### Public endpoint (limiter + security.headers, no auth)

| endpoint | behavior |
|---|---|
| `GET /j/:token?name=Alice` | `db.findOne` valid link (expiry checked in SQL, `/s/*` clause shape) → `lk.roomList names: [room_name]` confirms room alive → `lk.token` guest grants per `room_type` → `200 {livekit_url, token, meet_url, room, type}` |

**Every failure — unknown token, expired link, dead room, and even
LiveKit-unreachable — returns the identical `404 NOT_FOUND "not found"`**
(each via its own response node). On owner routes, LiveKit-unreachable fails
loudly (unwired error edge → 500): the owner must know his box can't reach
LiveKit.

`livekit_url` comes from `secrets.LIVEKIT_URL`; `meet_url` percent-encodes it
with a `replace()` chain (`:`→`%3A`, `/`→`%2F`).

## Config / deployment changes

- `noda.json`: new service instance named `lk` with `"plugin": "livekit"`
  (plugin name is `livekit`, node prefix is `lk.*` — verified in
  `plugins/livekit/plugin.go`), config `{url, api_key, api_secret}` from
  `$env(...)`.
- `.env.example` + compose `noda` service env: `LIVEKIT_URL`,
  `LIVEKIT_API_KEY`, `LIVEKIT_API_SECRET`. Production values = LiveKit
  Cloud credentials.
- Migration `room_links`.
- **E2E:** a `docker-compose.e2e.yml` override (used only by `e2e/run.sh`)
  adds `livekit/livekit-server` in dev mode with fixed dev credentials
  (the server's built-in dev credentials devkey/secret — the vendored
  livekit protocol enforces no minimum secret length for signing) and
  points the noda
  service's `LIVEKIT_*` at it. The production compose file is untouched.

## Testing

1. **Workflow tests** (`noda test`): all `lk.*`/`db.*` nodes mocked
   (test-runner constraint from cycle 1); happy paths + failure branches
   (unknown room, expired link, dead-room join, invalid type). The
   meeting-vs-stream grant branching runs for real (`control.if` on
   `room_type`).
2. **E2E** (extends cycle-1 suite, real against the dev LiveKit server):
   create meeting → guest joins via link → rooms list shows it → rotate
   link (old 404s, new works) → revoke link (404) → owner token minted →
   create stream (guest join returns type "stream") → delete room (link
   404s, gone from list) → 1s-expiry link dies.
3. **Manual acceptance** (README-documented, not automated): paste owner +
   guest tokens into a LiveKit client once; confirm media flows. E2E proves
   the API contract, not WebRTC.

## Out of scope (this cycle)

- Recording (egress), RTMP/OBS ingress, LiveKit webhooks.
- Presence/notifications into the drops feed.
- Kick/mute endpoints (owner has `roomAdmin` in his token; clients can use it).
- Any frontend.
