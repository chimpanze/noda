# Homebase

Personal private-cloud API built on [Noda](../../README.md) — config only, no
application code. Replaces a paste-everything Discord channel: one
chronological **drops** feed (markdown text and/or files), simple search,
revocable share links for friends, per-device sessions.

Spec: `docs/superpowers/specs/2026-07-07-homebase-foundation-design.md`

## Deploy

```bash
cp .env.example .env       # set SETUP_TOKEN, PUBLIC_BASE_URL (and DOMAIN for the edge)
docker compose up -d --build            # api on :3000 (migrations run automatically)
docker compose --profile edge up -d     # additionally: Caddy with TLS on :443
```

One-time bootstrap (creates the only account):

```bash
curl -X POST "$PUBLIC_BASE_URL/setup" -H 'Content-Type: application/json' \
  -d '{"setup_token":"<SETUP_TOKEN>","email":"you@example.com","password":"..."}'
```

## Deployment notes

- The API container binds to `127.0.0.1:3000` on the host — external traffic must come through the Caddy edge profile (TLS) or an SSH tunnel.
- Behind the Caddy proxy, per-IP rate limiting and the session device-IP list currently see the proxy's IP, not the client's (runtime limitation; tracked upstream).
- **Never run `e2e/run.sh` on a production host** — it runs `docker compose down -v` for this compose project and destroys its volumes (database + files).

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

## Tests

```bash
# workflow tests (no containers)
DATABASE_URL='postgres://x' FILES_PATH=/tmp/hb SETUP_TOKEN=test-setup-token \
  PUBLIC_BASE_URL=http://localhost:3000 go run ./cmd/noda test --config projects/homebase

# full E2E against the compose stack
./projects/homebase/e2e/run.sh
```
