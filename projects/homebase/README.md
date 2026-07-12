# Homebase

Personal private-cloud API built on [Noda](../../README.md) — config only, no
application code. Replaces a paste-everything Discord channel: one
chronological **drops** feed (markdown text and/or files), simple search,
revocable share links for friends, per-device sessions.

Spec: `docs/superpowers/specs/2026-07-07-homebase-foundation-design.md`

## Deploy

```bash
cp .env.example .env       # set SETUP_TOKEN, PUBLIC_BASE_URL (and DOMAIN for the edge)
docker compose up -d                    # pulls ghcr.io/chimpanze/noda; api on :3000 (migrations run automatically)
docker compose -f docker-compose.yml -f docker-compose.edge.yml up -d   # additionally: Caddy with TLS on :443 (requires DOMAIN)
```

To stop the edge stack, pass the same file pair: `docker compose -f docker-compose.yml -f docker-compose.edge.yml down` — a plain `docker compose down` won't see the caddy service.

One-time bootstrap (creates the only account):

```bash
curl -X POST "$PUBLIC_BASE_URL/setup" -H 'Content-Type: application/json' \
  -d '{"setup_token":"<SETUP_TOKEN>","email":"you@example.com","password":"..."}'
```

## Deployment notes

- The API container binds to `127.0.0.1:3000` on the host — external traffic must come through the Caddy edge override (TLS) or an SSH tunnel.
- With `NODA_ENV=production` set (see `.env.example`), the `noda.production.json` overlay enables `server.trust_proxy` so per-IP rate limiting and the session device-IP list see real client IPs behind Caddy. Requires a noda image newer than 0.0.4 — bump `NODA_VERSION` when the next release is published.
- `e2e/run.sh` runs in its own isolated compose project (`homebase-e2e`) and tears down only that project's volumes — it cannot touch a production stack running from this directory under the default project name. Still, prefer not to run it on a production host.

## Use

```bash
# log a machine in (store the token on that machine)
TOKEN=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"..."}' | jq -r .token)
AUTH="Authorization: Bearer $TOKEN"

curl -H "$AUTH" -X POST "$BASE/drops" -d '{"text":"- [ ] buy milk"}' -H 'Content-Type: application/json'
curl -H "$AUTH" -F file=@report.pdf -F text="the report" "$BASE/drops/upload"
curl -H "$AUTH" "$BASE/drops?q=milk"                 # search
curl -H "$AUTH" -X PATCH "$BASE/drops/<id>" -d '{"text":"- [x] buy milk"}' -H 'Content-Type: application/json'
curl -H "$AUTH" -OJ "$BASE/drops/<id>/file"          # download
curl -H "$AUTH" -X POST "$BASE/drops/<id>/share" -d '{"expires_in":"168h"}' -H 'Content-Type: application/json'
# → {"url": ".../s/<token>", ...}; friends need nothing but that URL
curl -H "$AUTH" "$BASE/auth/sessions"                # devices
curl -H "$AUTH" -X DELETE "$BASE/auth/sessions/<id>" # lost-laptop kill switch
```

| Endpoint | Auth | Purpose |
|---|---|---|
| `POST /setup` | setup token | one-time admin bootstrap |
| `POST /auth/login`, `POST /auth/logout`, `GET /auth/me` | — / session | sessions |
| `GET /auth/sessions`, `DELETE /auth/sessions/:id` | session | device list / revoke |
| `POST /drops`, `POST /drops/upload` | session | text drop / file drop |
| `GET /drops?q=&before=`, `GET /drops/:id`, `GET /drops/:id/file` | session | list+search / detail / download |
| `PATCH /drops/:id`, `DELETE /drops/:id` | session | edit text / delete |
| `POST /drops/:id/share`, `GET /shares`, `DELETE /shares/:id` | session | share links |
| `GET /s/:token`, `GET /s/:token/file` | none | friend access |

## Rooms (meetings & screen streaming)

Meetings and screen streams are LiveKit rooms. Set `LIVEKIT_URL`,
`LIVEKIT_API_KEY`, `LIVEKIT_API_SECRET` in `.env` (LiveKit Cloud → project
→ Keys). Rooms auto-close 10 minutes after the last person leaves.

```bash
# create a meeting (or "stream"); share the guest_url with friends
curl -H "$AUTH" -X POST "$BASE/rooms" -d '{"type":"meeting"}' -H 'Content-Type: application/json'
# → {"room":"hb-meet-…","guest_url":"…/j/<token>","livekit_url":"wss://…",...}

curl "$BASE/j/<token>?name=Alice"          # friend: no auth → {livekit_url, token, meet_url}
curl -H "$AUTH" -X POST "$BASE/rooms/<room>/token"   # your own (publisher) token
curl -H "$AUTH" "$BASE/rooms"              # active rooms + guest links
curl -H "$AUTH" -X POST "$BASE/rooms/<room>/link"    # rotate the guest link
curl -H "$AUTH" -X DELETE "$BASE/rooms/<room>/link"  # revoke (room stays up)
curl -H "$AUTH" -X DELETE "$BASE/rooms/<room>"       # end the room for everyone
```

Meetings: everyone can publish camera/mic/screen (up to 10 people). Streams:
only you can publish (screen + screen-audio + mic, up to 50 viewers). Paste
`meet_url` into a browser, or use `livekit_url` + `token` in any LiveKit
client.

**Manual acceptance check** (E2E covers the API, not WebRTC): create a
meeting, open `meet_url` for both the owner token and a guest token in two
browser tabs, confirm audio/video flows.

## Tests

```bash
# workflow tests (no containers)
DATABASE_URL='postgres://x' FILES_PATH=/tmp/hb SETUP_TOKEN=test-setup-token \
  PUBLIC_BASE_URL=http://localhost:3000 LIVEKIT_URL=ws://localhost:7880 \
  LIVEKIT_API_KEY=devkey LIVEKIT_API_SECRET=secret \
  go run ./cmd/noda test --config projects/homebase

# full E2E against the compose stack
./projects/homebase/e2e/run.sh
```
