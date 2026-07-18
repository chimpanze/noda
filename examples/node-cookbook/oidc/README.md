# Cookbook: oidc nodes

Runnable examples for all three `oidc.*` primitives — `auth_url`,
`exchange`, and `refresh` — against a real [Dex](https://dexidp.io) OIDC
provider (a lightweight, spec-compliant IdP good for testing against a real
authorization-code + refresh dance, rather than a mock). Every
request/response below is verified in CI by [`verify.json`](verify.json).

These nodes are **service-free** — unlike most other cookbook families,
`oidc.*` nodes take the issuer/client configuration directly in their own
`config` (`issuer_url`, `client_id`, `client_secret`, `redirect_uri`), not via
a `services` slot. This project's `noda.json` therefore declares no services
at all; every workflow below reads the provider's coordinates from
`{{ secrets.DEX_* }}` (the workflow-expression pattern for reading process
environment variables set by the harness/deployment).

## Run

This project needs a real Dex instance — CI's cookbook walker starts one via
testcontainers automatically (see `deps: ["dex"]` in `verify.json`), sets
`DEX_ISSUER`/`DEX_CLIENT_ID`/`DEX_CLIENT_SECRET`/`DEX_REDIRECT_URI`, and
pre-seeds a single-use authorization code as `${dex_code}` by driving Dex's
password-connector login page over plain HTTP (see
`internal/testing/containers/dex.go`).

To run it yourself, start Dex with a static client and password connector,
then click through the login page by hand:

```bash
cat > /tmp/dex.yaml <<'EOF'
issuer: http://127.0.0.1:5556/dex
storage:
  type: memory
web:
  http: 0.0.0.0:5556
oauth2:
  skipApprovalScreen: true
  responseTypes: ["code"]
staticClients:
  - id: cookbook-client
    secret: cookbook-secret
    name: Cookbook
    redirectURIs:
      - http://127.0.0.1:18888/callback
enablePasswordDB: true
staticPasswords:
  - email: admin@example.com
    hash: "$2a$10$2b2cU8CPhOTaGrs1HRQuAueS7JTT5ZHsHSzYiFPm1leZck7Mc8T4W"  # "password"
    username: admin
    userID: cookbook-user-1
EOF
docker run -d --name cookbook-dex -p 5556:5556 -v /tmp/dex.yaml:/etc/dex/cfg/dex.yaml \
  ghcr.io/dexidp/dex:v2.41.1 dex serve /etc/dex/cfg/dex.yaml

export DEX_ISSUER='http://127.0.0.1:5556/dex'
export DEX_CLIENT_ID='cookbook-client'
export DEX_CLIENT_SECRET='cookbook-secret'
export DEX_REDIRECT_URI='http://127.0.0.1:18888/callback'
go run ./cmd/noda start --config examples/node-cookbook/oidc
```

Then, in a browser:

1. `GET localhost:3000/api/oidc/login` — copy the returned `url` and open it.
2. Log in with `admin@example.com` / `password` on Dex's login form.
3. Dex 303s to `http://127.0.0.1:18888/callback?code=...&state=...` (nothing
   is listening there — that's fine, just copy the `code` query param from
   the browser's address bar before it errors).
4. `code`s are **single-use** — a code only works for one exchange; log in
   again to mint a fresh one for a second attempt.

## oidc.auth_url — `GET /api/oidc/login`

Performs OIDC discovery against `issuer_url` and builds the provider's
authorization URL. Per `docs/03-nodes/oidc.auth_url.md` Outputs, the success
shape is an **object** — `{ url, state } `— not a bare string, so the route
below echoes the whole node output as the response body and callers read
`.url` out of it.

`scopes` is requested as `["openid", "profile", "email", "offline_access"]`
— `offline_access` is what makes Dex issue a `refresh_token` on the
subsequent `exchange` (see below); without it, `exchange`'s `refresh_token`
field is absent from Dex's token response.

```bash
curl localhost:3000/api/oidc/login
# → 200 {"url":"http://127.0.0.1:5556/dex/auth?client_id=cookbook-client&...&state=cookbook-state","state":"cookbook-state"}
```

## oidc.exchange — `POST /api/oidc/exchange`

Exchanges an authorization code for tokens at Dex's token endpoint, then
verifies the returned ID token against Dex's JWKS. A bad/expired/reused code
routes to the node's `error` output, which this cookbook maps to `401`
(`response.error` with code `EXCHANGE_FAILED`) rather than leaking discovery
or token-endpoint error detail to the caller.

```bash
curl -X POST localhost:3000/api/oidc/exchange -H 'Content-Type: application/json' \
  -d '{"code": "<code-from-the-login-dance>"}'
# → 200 {"access_token":"...","id_token":"...","refresh_token":"...","claims":{...},"expires_at":...}

curl -X POST localhost:3000/api/oidc/exchange -H 'Content-Type: application/json' \
  -d '{"code": "bogus"}'
# → 401 {"error":{"code":"EXCHANGE_FAILED","message":"code exchange failed"}}
```

## oidc.refresh — `POST /api/oidc/refresh`

Exchanges a refresh token for new tokens at Dex's token endpoint. An
invalid/expired/revoked refresh token routes to `error`, mapped to `401`
here the same way as `exchange`.

```bash
curl -X POST localhost:3000/api/oidc/refresh -H 'Content-Type: application/json' \
  -d '{"refresh_token": "<refresh-token-from-exchange>"}'
# → 200 {"access_token":"...","refresh_token":"...","id_token":"...","claims":{...},"expires_at":...}
```

## Uncertainties resolved (with evidence)

- **`auth_url`'s `scopes` config field name** — `scopes` (an array of
  strings), confirmed against `docs/03-nodes/oidc.auth_url.md` Config table;
  no alternate name (e.g. `scope`) exists.
- **`auth_url`'s output shape** — an object `{ url, state }`, not a bare
  string, confirmed against `docs/03-nodes/oidc.auth_url.md` Outputs
  ("On success, outputs `{ url, state }`"). The route/workflow and
  `verify.json` both target `url` as a sub-path of the JSON body rather than
  asserting the whole body is a URL string.
- **Whether Dex actually returns a `refresh_token`** — yes, confirmed both
  by the docs (`oidc.exchange`'s Outputs table: "Refresh token (if provided
  by the IdP)") and empirically: the test harness's `DexAuthCode` helper
  (`internal/testing/containers/dex.go`) requests
  `scope=openid profile email offline_access` in the authorization request,
  and this cookbook's workflows request the same `offline_access` scope in
  `auth_url`'s config, matching the actual login dance CI performs against
  Dex. `offline_access` is the scope that makes an OIDC provider issue a
  `refresh_token` at all — without it most providers (Dex included) omit the
  field entirely rather than erroring, so the assertion was kept (not
  dropped) once this was confirmed against both the docs and the dance's
  actual scope request.
