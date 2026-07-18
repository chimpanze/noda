# Cookbook: workflow nodes

Runnable examples for `workflow.run` and `workflow.output`.
Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

```bash
noda start --config examples/node-cookbook/workflow
```

## workflow.run and workflow.output — `POST /api/run`

`workflow.run` invokes a sub-workflow synchronously and captures its output. `workflow.output` names the value the sub-workflow returns (`name: "result"` in `callee.json`).

Note the parent-side edge: `workflow.run` exposes `success`/`error` output ports, and the sub-workflow's named output is routed through `success` (with its data preserved) — the edge in `run.json` says `"output": "success"`, not `"output": "result"`. The `name` labels the sub-workflow's terminal output; it does not become an output port on the calling node.

```bash
curl -X POST localhost:3000/api/run -H 'Content-Type: application/json' -d '{"x": 4}'
# → 200 {"result":40}
```
