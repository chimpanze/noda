# Cookbook: http nodes

Runnable examples for `http.get`, `http.post`, and `http.request`.
Every request/response below is verified in CI by [`verify.json`](verify.json).

This project targets **its own** `/api/echo` endpoint as the outbound HTTP target, so the
example runs standalone with no external network access: the `web` service (an `http` plugin
instance) calls back into the same running server. The harness runs this project over a real
TCP listener (`listen: true` in `verify.json`) and exports `COOKBOOK_BASE_URL` with the
listener's `http://127.0.0.1:<port>` base — read inside workflow node configs via
`{{ secrets.COOKBOOK_BASE_URL }}` (NOT `{{ $env(...) }}`, which only resolves in the root
`noda.json` document, not inside routes/workflows).

Response headers from `http.get`/`http.post`/`http.request` come back with **lowercase keys**
(e.g. `content-type`, not `Content-Type`) — always index `nodes.<id>.headers` with lowercase
names.

Because the target is `127.0.0.1`, the `web` service must set `"allow_private_networks": true`
in `noda.json` — the http plugin's outbound SSRF guard (`netguard`) denies requests to private/
loopback IPs by default (see `plugins/http/transport.go`), and this project deliberately calls
back into its own loopback listener. **Do not copy this setting into production configs** — it
disables the http plugin's SSRF guard, which exists to stop workflows from reaching internal
networks; it is enabled here only because this example deliberately calls its own loopback
server.

## Run

```bash
COOKBOOK_BASE_URL=http://127.0.0.1:3000 noda start --config examples/node-cookbook/http
```

(`COOKBOOK_BASE_URL` isn't needed to serve the app — the workflows only need it when the http
nodes fire — but it must be set in the environment for config validation/loading to resolve the
`secrets.COOKBOOK_BASE_URL` references used in the `url` fields below.)

## Supporting endpoint — `/api/echo` (GET, POST, PUT)

Not one of the three node types itself — this is the in-project target the `http.*` nodes call
out to. It echoes back whatever body and method it received, plus a `marker` field so the
proxy workflows below can prove the round trip actually happened.

```bash
curl -X POST localhost:3000/api/echo -H 'Content-Type: application/json' -d '{"hello":"world"}'
# → 200 {"echo":{"hello":"world"},"method":"POST","marker":"cookbook-echo"}
```

## http.get — `GET /api/proxy-get`

Calls `GET {{ secrets.COOKBOOK_BASE_URL }}/api/echo` through the `web` http service, then
reflects the response status, marker, and (lowercase) `content-type` header back to the caller.

```bash
curl localhost:3000/api/proxy-get
# → 200 {"status":200,"marker":"cookbook-echo","content_type":"application/json"}
```

## http.post — `POST /api/proxy-post`

Takes `{"message": "..."}` from the request body, POSTs `{"message": "..."}` to `/api/echo`
via `http.post`, and returns the echoed message extracted from the nested echo response.

```bash
curl -X POST localhost:3000/api/proxy-post -H 'Content-Type: application/json' -d '{"message": "hello from post"}'
# → 200 {"echoed":"hello from post"}
```

The `body` config field deep-resolves expression templates nested inside a literal JSON
object, so `"body": {"message": "{{ input.message }}"}` works directly — no need to wrap the
whole object in a single expr map-literal expression.

## http.request — `GET /api/proxy-request`

Uses `http.request` with an explicit `method: "PUT"`, a custom `X-Cookbook` header, and a
`{"via": "request"}` body, sent to `/api/echo` (whose PUT route accepts it via the same echo
workflow). Returns the response status and the echoed `via` field.

```bash
curl localhost:3000/api/proxy-request
# → 200 {"status":200,"echo_via":"request"}
```
