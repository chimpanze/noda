# Cookbook: control nodes

Runnable examples for `control.if`, `control.switch`, and `control.loop`.
Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

```bash
noda start --config examples/node-cookbook/control
```

## control.if — `POST /api/if`

Routes to the `then` edge when the condition is true, `else` otherwise.

```bash
curl -X POST localhost:3000/api/if -H 'Content-Type: application/json' -d '{"value": 10}'
# → 200 {"branch":"high","value":10}
curl -X POST localhost:3000/api/if -H 'Content-Type: application/json' -d '{"value": 3}'
# → 200 {"branch":"low","value":3}
```

## control.switch — `POST /api/switch`

Matches the expression against `cases`; unmatched values take the `default` edge.

```bash
curl -X POST localhost:3000/api/switch -H 'Content-Type: application/json' -d '{"kind": "sms"}'
# → 200 {"channel":"sms"}
curl -X POST localhost:3000/api/switch -H 'Content-Type: application/json' -d '{"kind": "carrier-pigeon"}'
# → 200 {"channel":"unknown"}
```

## control.loop — `POST /api/loop`

Runs the `loop-item` workflow once per element (`$item`); results collect on
the `done` edge as an array.

```bash
curl -X POST localhost:3000/api/loop -H 'Content-Type: application/json' -d '{"nums": [1, 2, 3]}'
# → 200 {"count":3,"first":2}
```
