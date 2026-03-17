# Node Reference

Noda provides 68 built-in node types organized by plugin. Every node returns a named output (`success` or `error` by default) along with its result data.

## Node Categories

| Category | Prefix | Nodes |
|----------|--------|-------|
| Control Flow | `control` | if, switch, loop |
| Workflow | `workflow` | run, output |
| Transform | `transform` | set, map, filter, merge, delete, validate |
| Response | `response` | json, redirect, error |
| Utility | `util` | log, uuid, delay, timestamp |
| Event | `event` | emit |
| Upload | `upload` | handle |
| WebSocket | `ws` | send |
| SSE | `sse` | send |
| Wasm | `wasm` | send, query |
| Storage | `storage` | read, write, delete, list |
| Database | `db` | query, exec, create, find, findOne, count, update, delete, upsert |
| Cache | `cache` | get, set, del, exists |
| HTTP | `http` | request, get, post |
| Email | `email` | send |
| Image | `image` | resize, crop, watermark, convert, thumbnail |
| LiveKit | `lk` | token, roomCreate, roomList, roomDelete, roomUpdateMetadata, sendData, participantList, participantGet, participantRemove, participantUpdate, muteTrack, egressStartRoomComposite, egressStartTrack, egressStop, egressList, ingressCreate, ingressList, ingressDelete |

## Error Handling

All nodes can route to an `error` output. When a node fails, the error data is available downstream as `ErrorData`:

| Field | Type | Description |
|-------|------|-------------|
| `code` | string | Error code (e.g., `VALIDATION_ERROR`, `NOT_FOUND`) |
| `message` | string | Human-readable error message |
| `node_id` | string | ID of the failed node |
| `node_type` | string | Type of the failed node |
| `trace_id` | string | Request trace ID |
| `details` | any | Additional error details |

### Error Type to HTTP Status Mapping

| Error Type | HTTP Status | Code |
|-----------|-------------|------|
| `ValidationError` | 422 | `VALIDATION_ERROR` |
| `NotFoundError` | 404 | `NOT_FOUND` |
| `ConflictError` | 409 | `CONFLICT` |
| `ServiceUnavailableError` | 503 | `SERVICE_UNAVAILABLE` |
| `TimeoutError` | 504 | `TIMEOUT` |
| Other | 500 | `INTERNAL_ERROR` |
