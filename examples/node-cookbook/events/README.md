# Cookbook: events (`event.emit`)

Runnable examples for `event.emit` in both delivery modes — `stream`
(durable, consumed by a worker) and `pubsub` (real-time fan-out). Every
request/response below is verified in CI by [`verify.json`](verify.json).

## Run

This project needs Redis and a writable scratch directory. CI's cookbook
walker starts a Redis container and exports `REDIS_URL`/`COOKBOOK_DATA_DIR`
automatically. To run it yourself:

```bash
export REDIS_URL=redis://localhost:6379/0
export COOKBOOK_DATA_DIR=/tmp/noda-events-cookbook
go run ./cmd/noda start --config examples/node-cookbook/events
```

## Pipeline: stream mode, end to end

```
POST /api/jobs → event.emit(mode=stream) → [Redis Stream: cookbook.jobs]
   → worker "process-job-worker" (group cookbook-workers) → workflow
   "process-job" → storage.write("done/<job_id>") → GET /api/jobs/:job_id
   polls storage.read until it finds the marker → 200 {done: true}
```

`workflows/emit-stream.json` emits onto the `jobs-stream` service
(`services.stream`, per `docs/03-nodes/event.emit.md`). The worker
(`workers/process-job.json`) subscribes to the same topic/service and
triggers `workflows/process-job.json` on each message.

**Worker → workflow input mapping**, confirmed by reading
`internal/worker/runtime.go`'s `ParseWorkerConfigs` and mirroring
`examples/saas-backend/workers/generate-thumbnail.json` exactly: a worker
config needs a `trigger: {workflow, input}` object (not a flat `workflow`
string — the JSON schema at `internal/config/schemas/worker.json` requires
`trigger.workflow`). Fields in `trigger.input` are expressions evaluated
against the incoming message, so the emitted payload's `job_id` is reached
via `{{ message.payload.job_id }}` — matching how
`generate-thumbnail-worker` reads `message.payload.attachment_id`.

**Disclosed supporting reuse:** the worker's own end-effect (so the round
trip is externally observable) is a `storage.write` into
`{{ $env('COOKBOOK_DATA_DIR') }}/scratch`, read back via `storage.read` on
the `GET /api/jobs/:job_id` route — the same `db`-family precedent used by
the `storage` cookbook family, not a new node under test here. `verify.json`
polls that endpoint with `retry_timeout: "10s"` so the assertion genuinely
waits for the worker's stream round trip rather than racing it; deleting
`workers/process-job.json` and re-running the suite reproduces a real
timeout (404 `PENDING`) confirming the poll is load-bearing.

```bash
curl -X POST localhost:3000/api/jobs -H 'Content-Type: application/json' \
  -d '{"job_id": "job-1"}'
# → 202 {"queued":true,"message_id":"<redis-stream-id>"}

curl localhost:3000/api/jobs/job-1
# → 404 {"error":{"code":"PENDING", ...}} until the worker catches up, then
# → 200 {"done":true}
```

## `event.emit` — pubsub mode — `POST /api/notify`

Per `docs/03-nodes/event.emit.md`'s Service Dependencies table (confirmed
by reading `plugins/core/event/emit.go`), pubsub mode looks up its service
under the **`pubsub`** slot, not `stream` — the two modes use separate,
independently-optional slots (`stream: {Required: false}`, `pubsub:
{Required: false}`) on the same node, so `workflows/emit-pubsub.json` wires
`services.pubsub` to the `notify-pubsub` service.

The node's observable output (run and inspected directly, matching the
docs): stream mode returns `{message_id}`, pubsub mode returns `{ok: true}`.
`workflows/emit-pubsub.json` echoes `nodes.emit.ok` back in the response
body to make that shape explicit.

```bash
curl -X POST localhost:3000/api/notify -H 'Content-Type: application/json' \
  -d '{"text": "hello"}'
# → 202 {"ok":true}
```

**Honest scope note:** this family proves pubsub *emission* at the node
boundary only — `event.emit` accepts the call and the underlying
`PubSubService.Publish` returns without error. It does not itself prove a
subscriber receives the message; pubsub *delivery* is exercised end-to-end
by the realtime cookbook family, whose WebSocket fan-out synchronizes
through the same pubsub service. Stream-mode delivery, by contrast, *is*
proven end-to-end right here, via the worker → `storage.write` → polled
`storage.read` round trip above.
