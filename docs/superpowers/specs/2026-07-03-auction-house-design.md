# Auction House — Dogfooding Validation API (Design)

**Date:** 2026-07-03
**Status:** Approved (brainstorm complete)
**Location:** `projects/auction-house/`

## Purpose

Build a realistic, self-contained live auction API with Noda to (a) confirm the runtime
works end-to-end across most subsystems and (b) surface problems and gaps as a user
would hit them. The deliverable is twofold: a working API and a triaged findings log.

Decisions from the brainstorm:

- **Realistic product, built honestly** — a genuine app built the way a user would
  (docs + MCP tools + CLI only), not a contrived kitchen sink. Broad feature usage
  falls out of the domain; gaps show up as real friction.
- **Fully self-contained** — `docker compose up` brings up everything (Postgres,
  Redis, Mailpit, local storage). No external services or hardware.
- **Lives in `projects/`** — a dogfooding project alongside `adventure-stream`, not a
  shipped example. May be promoted to `examples/` later once gaps are fixed.
- **API-only + test suite** — no frontend. Validation via Noda's workflow test runner,
  an e2e lifecycle script, and WS/SSE smoke clients.
- **Findings: friction log + GitHub issues** — `FINDINGS.md` during the build, triaged
  into issues / doc fixes at the end (the AI-usability #223–#232 pattern).

## Product Scope

**Bidhub** — a live auction house.

1. **Accounts & roles** — register/login with JWT. Roles: `user` (buys and sells) and
   `admin` (moderation). Casbin enforces: sellers edit only their own listings; admins
   can suspend listings/users.
2. **Listings** — title, description, starting price, bid increment, end time, 1–3
   photos (upload → storage service → thumbnail via image plugin). Lifecycle:
   `draft → active → closed/cancelled`.
3. **Live bidding** — bids via HTTP; workflow-expression validation (auction open,
   amount ≥ current + increment, not own listing). Rate limiting via cache plugin.
   Accepted bids emit to a Redis stream and broadcast over a per-auction WebSocket
   channel; a site-wide SSE ticker streams price changes. Users can **watch** a
   listing (`POST /auctions/:id/watch`); watches feed the ticker filter and the
   daily digest.
4. **Proxy bidding (Wasm)** — a user sets a max bid; when outbid, a Wasm module
   computes counter-bids (increment rules, tie-breaking). Genuinely custom logic that
   config cannot express — an honest Wasm use case.
5. **Anti-sniping** — a bid in the final 2 minutes extends the end time (workflow
   logic; observable, so bugs are loud).
6. **Auction closing (scheduler)** — a cron scans for expired auctions, closes them,
   determines winners, emits events. The subsystem existing examples barely touch.
7. **Notifications (workers + email)** — stream consumers send outbid / won /
   auction-ended emails to Mailpit. Retries + DLQ exercised with a deliberately
   failing case.
8. **Mock payment** — winner pays via an outbound-HTTP call to a mock payment
   provider that is itself a Noda route in the same app (`/mock/psp/charge`), which
   calls back with a signed webhook. Exercises outbound HTTP + webhook signature
   verification with zero external dependencies.
9. **Admin/ops** — moderation endpoints plus a daily-digest cron (second scheduler
   use): one email per user summarizing their watched and own listings (current
   price, time remaining, ended-since-yesterday results).

### Feature coverage matrix

| Noda subsystem | Where it's exercised |
|---|---|
| Server, routes, route groups, middleware | All endpoints; auth'd API group vs. open webhook/mock routes |
| JWT auth | Register/login, all authenticated routes |
| Casbin | Seller-owns-listing, admin moderation |
| DB plugin + migrations | Full schema, transactional bid writes |
| Cache plugin | Bid rate-limiting, hot listing state |
| Stream plugin + workers + DLQ | `bid.placed` / `auction.closed` / `payment.settled`; failing-event DLQ case |
| PubSub plugin | WS/SSE fan-out |
| Scheduler | Auction close scan (10s), daily digest |
| WebSocket (connmgr) | Per-auction live bid channel |
| SSE (connmgr) | Site-wide price ticker |
| Uploads + storage plugin | Listing photos |
| Image plugin | Thumbnails |
| Email plugin | Outbid/won/ended notifications (Mailpit) |
| HTTP plugin (outbound) | Charge call to mock PSP |
| Wasm runtime + pdk | Proxy-bid engine |
| Expressions | Validation, transforms throughout |
| Workflow test runner | Per-workflow test suites |
| Dev mode + hot reload | Used throughout the build |
| Core nodes | control.if/switch/loop, transform.*, response.*, util.*, event.emit, upload.handle, ws.send, sse.send, wasm.* |

## Architecture

### Services (docker-compose)

| Instance | Plugin | Purpose |
|---|---|---|
| `main-db` | `db` (Postgres) | Users, listings, bids, proxy-bid maxes, orders, audit log — source of truth |
| `app-cache` | `cache` (Redis) | Bid rate-limiting, hot listing state for ticker reads |
| `main-stream` | `stream` (Redis Streams) | Durable domain events → workers, DLQ |
| `realtime` | `pubsub` (Redis PubSub) | WS/SSE fan-out across instances |
| `files` | `storage` (local afero) | Listing photos + thumbnails |
| `mail` | `email` → Mailpit | Notification delivery |
| `psp` | `http` (outbound) | Calls the mock payment provider route in this same app |

### Config layout

```
projects/auction-house/
  noda.json            — services, JWT, wasm runtime, casbin model/policies
  migrations/          — users, listings, bids, proxy_bids, watches, orders, audit
  routes/              — auth, listings CRUD, bids, watch, admin, mock-psp, psp-webhook
  workflows/           — one per route + close-auction, digest, notification handlers
  workers/             — notify (outbid/won/ended emails), audit-log, payment-settle
  schedules/           — close-expired-auctions (every 10s), daily-digest
  connections/         — per-auction WS channel, SSE ticker
  schemas/             — request validation schemas
  wasm/                — proxy-bid engine (built via pdk/)
  tests/               — workflow test-runner suites, e2e script, concurrency probes
  FINDINGS.md          — the friction log
```

### Data model

`users`, `listings`, `bids`, `proxy_bids` (user max-bids per listing), `watches`
(user ↔ listing), `orders` (winner, amount, payment state), `audit_log`. All via migrations; map-based GORM
access per Noda convention (no structs).

### Critical flow — a bid

1. `POST /auctions/:id/bids` → JWT middleware → rate-limit check (cache) → workflow
   validates in a transaction against `main-db` (auction open, amount ≥ high +
   increment, not own listing).
2. On accept: write bid; apply anti-snipe extension if < 2 minutes remain; invoke the
   Wasm proxy-bid engine with the new bid + stored max-bids; persist any returned
   auto-counter-bids (loop node).
3. Emit one `bid.placed` event per persisted bid to `main-stream`; `ws.send`
   broadcasts the new price to the auction's WS channel and the SSE ticker.
4. The `notify` worker consumes `bid.placed`, determines who was outbid, sends the
   email. A malformed test event exercises retry → DLQ.

### Closing / payment flow

Scheduler (10s cron) → close workflow marks expired auctions closed, picks winner,
creates an `order`, emits `auction.closed` → worker emails winner and seller → winner
calls `POST /orders/:id/pay` → outbound HTTP to `/mock/psp/charge` → mock PSP route
responds and later posts a signed webhook to `/webhooks/psp` → settle workflow
verifies the signature, marks the order paid, emits `payment.settled`.

### Concurrency (the honest stress)

Two bids racing; a bid racing the close cron; anti-snipe extension racing expiry.
Where Noda's config surface cannot express the needed atomicity (e.g. compare-and-set
on the high bid), that limitation is itself a finding and goes in `FINDINGS.md`.

## Testing

1. **Workflow tests** (Noda test runner, `tests/`) — bid validation edge cases (below
   increment, own listing, closed auction), anti-snipe boundary (exactly 2:00
   remaining), proxy-bid engine outcomes, close-workflow winner selection, PSP
   webhook signature rejection. Doubles as real exercise of the test-runner
   subsystem.
2. **End-to-end script** (`tests/e2e.sh` against `docker compose up`) — full auction
   lifecycle: register two users → create listing with photo → verify thumbnail →
   bid war with a proxy bid → snipe inside the final 2 minutes → verify extension →
   wait for scheduler close → verify winner order + Mailpit emails → pay → verify
   signed webhook settled the order. Plus a WS/SSE smoke client asserting live price
   frames arrive.
3. **Concurrency probes** — scripts firing simultaneous bids and bid-vs-close races,
   then checking invariants in Postgres: monotonic bid amounts, exactly one winner,
   no bids after close. The most likely gap-finders.

## Build Method (the "honest" rules)

- Build using only `docs/`, the noda MCP tools (`scaffold`, `get_node_schema`,
  `validate_config`, …), and the CLI — as a user would. No reading `internal/` to
  figure out behavior.
- When stuck, log the friction **before** the workaround: what was expected, what
  happened, which doc or error message failed. Only then peek at source — and the
  need to peek is itself a finding.
- Dev mode + hot reload used throughout.

## Findings Protocol

- `FINDINGS.md` entries as they happen:
  `[F-##] severity (bug/doc/dx/gap) — expected vs. actual — where`.
  Severity: blocker / major / minor / paper-cut.
- End-of-build triage: confirmed bugs and feature gaps → GitHub issues; doc problems
  → candidate doc PRs; user-error entries closed with a note on what would have
  prevented the confusion.

## Success Criteria

- `docker compose up` → e2e script passes green.
- Every subsystem in the coverage matrix demonstrably exercised.
- `FINDINGS.md` fully triaged into issues / doc fixes / closed notes.
