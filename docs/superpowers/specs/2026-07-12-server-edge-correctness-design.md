# Server Edge Correctness — Design

**Date:** 2026-07-12
**Issues:** #300 (trusted-proxy support), #301 (numeric server settings from env)
**Branch:** `feat/server-edge-correctness`

## Problem

Both issues come from the homebase final review (PR #299) and both degrade the
deployed homebase instance today:

- **#300:** `internal/server/server.go` builds `fiber.Config` with no
  trusted-proxy settings, so behind a reverse proxy (homebase's Caddy edge
  profile) `c.IP()` is always the proxy container's IP. Every per-IP `limiter`
  bucket collapses into one global bucket per route (30 junk logins/min from
  anyone locks the owner out of `/auth/login`), and `auth_sessions.ip` records
  the proxy IP, making session device lists useless.
- **#301:** `server.body_limit` is read only via a strict `.(float64)`
  assertion (`server.go:137`), and the schema types it `integer` — so
  `"{{ $env('BODY_LIMIT') }}"` is doubly blocked and silently falls back to
  the 5 MB default. Homebase had to hardcode `1073741824`. The same
  silent-fallback disease affects other `server.*` scalars: malformed
  `read_timeout`/`write_timeout` durations are silently ignored today.

## Decisions (user-approved 2026-07-12)

1. **Env-settability + loud validation covers ALL scalar `server.*` settings**,
   not just `body_limit`. One consistent rule; kills the silent-fallback class.
2. **The homebase project config flips to the new features in this tranche**
   (trust_proxy for the edge path, BODY_LIMIT from env, README caveat removed).
   Prod redeploy stays a separate manual step.
3. **`server.trust_proxy` is a full nested object mirroring Fiber's surface**
   (not a minimal CIDR list): the class booleans matter in practice because
   Docker/compose proxy IPs are dynamic, so homebase needs `private: true`
   rather than a pinned CIDR.

## Design

### 1. `server.trust_proxy` (#300)

New object in the root config schema (`internal/config/schemas/root.json`,
`server` section):

```json
"server": {
  "trust_proxy": {
    "enabled": true,
    "proxies": ["10.0.0.0/8"],
    "private": true,
    "loopback": false,
    "link_local": false,
    "header": "X-Forwarded-For"
  }
}
```

| Field | Type | Default | Maps to (Fiber v3.1.0) |
|---|---|---|---|
| `enabled` | boolean | `false` | `fiber.Config.TrustProxy` |
| `proxies` | array of IP/CIDR strings | `[]` | `TrustProxyConfig.Proxies` |
| `loopback` | boolean | `false` | `TrustProxyConfig.Loopback` |
| `link_local` | boolean | `false` | `TrustProxyConfig.LinkLocal` |
| `private` | boolean | `false` | `TrustProxyConfig.Private` |
| `header` | string | `"X-Forwarded-For"` when enabled | `fiber.Config.ProxyHeader` |

Fiber's `unix_socket` flag is deliberately omitted (noda doesn't listen on
unix sockets — YAGNI).

**Semantics** (Fiber's, verified in vendored `fiber/v3@v3.1.0/app.go`): when
`TrustProxy` is on and the request's socket remote IP is in the trusted set,
`c.IP()` returns the value from `ProxyHeader`; otherwise `c.IP()` stays the
socket remote IP. Direct clients spoofing `X-Forwarded-For` therefore gain
nothing. `ProxyHeader` is only set on `fiber.Config` when `enabled: true` —
with trust_proxy absent/disabled, behavior is byte-for-byte today's.

**Fail-fast validation at startup (`NewServer` returns error):**

- `enabled: true` with empty `proxies` AND all class booleans false → config
  error (would silently trust nothing; the header would never be honored).
- Any entry in `proxies` that parses as neither an IP nor a CIDR → config
  error. We validate ourselves with `net.ParseIP`/`net.ParseCIDR` rather than
  relying on Fiber to complain.
- Implementation note: verify Fiber's own behavior for both cases against the
  vendored source during implementation and align (per the standing
  verify-against-vendored-source convention).

`proxies` entries and `header` are strings, so `$env()` works in them for
free (resolution happens before `NewServer` reads the config).

### 2. Env-settable scalar server settings (#301)

`$env()` resolution (`internal/secrets/resolve.go`, pipeline step 5) already
covers `server.*` — it just always produces strings. Two changes:

**Schema:** `port`, `body_limit`, `expression_memory_budget` become
integer-or-string (`oneOf`), keeping the existing `minimum`/`maximum`
constraints on the integer branch. Range constraints on string-provided
values are enforced at parse time instead.

**Reader:** a coercion helper in `internal/server` (shape roughly
`coerceIntSetting(serverCfg, key) (int, bool, error)`) accepts `float64`
(JSON number) or string (post-`$env()`; `strconv`). All `server.*` scalar
readers route through it or its duration sibling. Known readers to convert
(the plan enumerates the full set by grep — `expression_memory_budget`,
`response_timeout`, `shutdown_deadline` etc. are read outside `server.go`):

- `port`, `body_limit` (ints), `read_timeout`, `write_timeout` (durations) in
  `internal/server/server.go NewServer`
- any other `server.*` scalar consumer found by
  `grep -rn "serverCfg\[\|\"server\"\]" internal/` — same treatment.

**Loud failure:** any invalid value — non-numeric string after resolution,
out-of-range port (must be 1–65535), negative body_limit, malformed duration
string — is a startup error naming the key and offending value. This
replaces today's silent fall-back-to-default. **Deliberate behavior change**:
configs that today limp along on defaults will now refuse to start;
CHANGELOG entry under [Unreleased] → Changed, mirroring the fail-loudly
precedent from tranche B.

### 3. Homebase flip

In `projects/homebase`:

- `noda.json`: `body_limit` becomes `"{{ $env('BODY_LIMIT') }}"`; `.env` /
  compose gain `BODY_LIMIT` (default 1073741824 preserved). Verify during
  implementation whether `$env()` supports a default-value form; if not, the
  compose env default carries it.
- `trust_proxy` (`enabled: true, private: true`, X-Forwarded-For) goes in via
  the config-overlay mechanism scoped to the edge/prod deployment — NOT the
  base config, so plain dev compose (clients hitting noda directly) never
  trusts spoofable headers. The plan picks the concrete overlay vehicle
  (NODA_ENV overlay file) after inspecting how homebase currently wires env.
- README trusted-proxy caveat removed, replaced by a short note on the new
  config.
- **Caveat:** prod runs the pinned ghcr noda image (0.0.4); the flip takes
  effect only after a new image is built and the VPS redeployed. Redeploy is
  the user's manual step, out of scope here.

### 4. Docs

`docs/02-config/noda-json.md`:

- New `trust_proxy` subsection (fields table + Caddy/docker example +
  spoofing semantics).
- The integer-or-env-string rule for numeric server settings, with the
  fail-fast note.
- Security note from the review: Fiber buffers request bodies in memory up to
  `body_limit` BEFORE auth runs, so a large limit is a cheap unauthenticated
  memory-pressure vector — set the edge proxy's body limit too.

### 5. Testing

Per project convention, tests read real JSON fixtures from `testdata/`.

- **Unit (internal/server):** fiber.Config mapping for every trust_proxy
  field; coercion helper accepting float64 and numeric string; each
  loud-failure case (garbage string, out-of-range port, bad duration,
  enabled-with-empty-trust-set, bad CIDR) asserts `NewServer` errors and the
  message names the key.
- **Behavioral (httptest via `app.Test`):** with trust_proxy enabled and a
  trusted source, `c.IP()` returns the X-Forwarded-For value; with an
  untrusted source (and with trust_proxy disabled), the header is ignored.
  Polarity check: the untrusted-source assertion must FAIL against a build
  that blindly trusts the header.
- **Limiter keying:** if cheaply reachable, a test that two X-Forwarded-For
  identities from a trusted proxy land in different limiter buckets.
- Existing server tests must pass unchanged with no `trust_proxy` configured.

## Execution shape

Standing conventions: worktree `.worktrees/server-edge-correctness`, branch
`feat/server-edge-correctness` off main; this spec + the plan `git add -f`'d
onto the branch; subagent-driven per task (implementer → spec-compliance
reviewer → code-quality reviewer); whole-branch review before PR. PR closes
#300 and #301.

## Out of scope

- Prod VPS redeploy / noda image release.
- Rate-limit or session-listing behavior changes beyond the IP fix.
- Trusted-proxy awareness anywhere other than Fiber `c.IP()` consumers
  (e.g. no changes to connmgr or wasm host APIs).
