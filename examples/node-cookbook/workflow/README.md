# Cookbook: workflow nodes

Runnable examples for `workflow.run` and `workflow.output`.
Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

```bash
noda start --config examples/node-cookbook/workflow
```

## workflow.run and workflow.output — `POST /api/run`

`workflow.run` invokes a sub-workflow synchronously and captures its output. `workflow.output` names the value that the caller receives.

```bash
curl -X POST localhost:3000/api/run -H 'Content-Type: application/json' -d '{"x": 4}'
# → 200 {"result":40}
```
