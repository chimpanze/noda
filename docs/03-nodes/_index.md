# Node Reference

Noda provides 81 built-in node types organized by plugin. Every node returns a named output (`success` or `error` by default) along with its result data.

## Node Categories

| Category | Prefix | Nodes |
|----------|--------|-------|
| Auth | `auth` | create_user, get_user, verify_credentials, create_session, revoke_session, create_token, consume_token, set_password |
| Control Flow | `control` | if, switch, loop |
| Workflow | `workflow` | run, output |
| Transform | `transform` | set, map, filter, merge, delete, validate |
| Response | `response` | json, redirect, error, file |
| Utility | `util` | log, uuid, delay, timestamp, jwt_sign |
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
| LiveKit | `lk` | token, room_create, room_list, room_delete, room_update_metadata, send_data, participant_list, participant_get, participant_remove, participant_update, mute_track, egress_start_room_composite, egress_start_track, egress_stop, egress_list, ingress_create, ingress_list, ingress_delete |
| OIDC | `oidc` | auth_url, exchange, refresh |

## Error Handling

All nodes can route to an `error` output. When a node fails, the error data is available downstream as a plain object with these fields:

| Field | Type | Description |
|-------|------|-------------|
| `code` | string | Error code (e.g., `VALIDATION_ERROR`, `NOT_FOUND`) |
| `error` | string | Diagnostic error message — see warning below |
| `node_id` | string | ID of the failed node |
| `node_type` | string | Type of the failed node |
| `available_nodes` | array of string | IDs of nodes whose output was available for data flow at the point of failure |

> **`error` is a diagnostic field.** It may contain driver, network, or filesystem detail such as
> constraint names, internal hostnames, or file paths. Do not forward it to clients — branch on
> `code` instead, and return your own message.

There is no `message`, `trace_id`, or `details` field on this payload — those belong to the
separate HTTP error response body (see the status-mapping table below), not to the error-edge
data a downstream node reads.

### Branching on the error code

```json
{
  "get_user": {
    "type": "db.findOne",
    "services": { "database": "postgres" },
    "config": {
      "table": "users",
      "where": { "id": "{{ input.user_id }}" },
      "required": true
    }
  },
  "check_error": {
    "type": "control.if",
    "config": {
      "condition": "{{ nodes.get_user.code == \"NOT_FOUND\" }}"
    }
  }
}
```

Wire `get_user`'s `error` output to `check_error`. `nodes.get_user.code` holds the code from the
table above, so this workflow can take a different path for a missing user than for, say, a
`SERVICE_UNAVAILABLE` from a database outage.

### Error Type to HTTP Status Mapping

| Error Type | HTTP Status | Code |
|-----------|-------------|------|
| `ValidationError` | 422 | `VALIDATION_ERROR` |
| `NotFoundError` | 404 | `NOT_FOUND` |
| `ConflictError` | 409 | `CONFLICT` |
| `ServiceUnavailableError` | 503 | `SERVICE_UNAVAILABLE` |
| `TimeoutError` | 504 | `TIMEOUT` |
| Other | 500 | `INTERNAL_ERROR` |
