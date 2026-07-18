# Cookbook: util nodes

Runnable examples for `util.log`, `util.uuid`, `util.timestamp`, `util.delay`, and `util.jwt_sign`.
Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

```bash
noda start --config examples/node-cookbook/util
```

## util.log — `POST /api/log`

Writes a message to the application log.

```bash
curl -X POST localhost:3000/api/log -H 'Content-Type: application/json' -d '{"who": "cookbook"}'
# → 200 {"logged":true}
```

## util.uuid — `GET /api/uuid`

Generates a random UUID v4.

```bash
curl localhost:3000/api/uuid
# → 200 {"id":"550e8400-e29b-41d4-a716-446655440000"}
```

## util.timestamp — `GET /api/timestamp`

Returns the current Unix timestamp (seconds since epoch).

```bash
curl localhost:3000/api/timestamp
# → 200 {"unix":1721270400}
```

## util.delay — `GET /api/delay`

Pauses execution for a specified duration.

```bash
curl localhost:3000/api/delay
# → 200 {"waited":"50ms"}
```

## util.jwt_sign — `POST /api/jwt-sign`

Signs and returns a JWT token. The demo secret `cookbook-demo-secret` is intentionally literal for this self-contained example;
real projects should use `{{ secrets.NAME }}` per [`docs/02-config/variables.md`](../../docs/02-config/variables.md).

```bash
curl -X POST localhost:3000/api/jwt-sign -H 'Content-Type: application/json' -d '{"uid": "user-1"}'
# → 200 {"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLTEifQ.5Q1dEPpDpFz_dS1n_x_X_x_X_x_X_x_X"}
```

The token shown is illustrative — the real payload also carries an `exp` claim, since the workflow sets `expiry: 1h`.
