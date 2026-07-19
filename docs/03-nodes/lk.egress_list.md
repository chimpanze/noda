# lk.egress_list

Lists egress recordings.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | no | Optional room name filter |

## Outputs

`success`, `error`

Output: `{items: [...]}`

Each item contains `egress_id`, `room_id`, `room_name`, `status`, `started_at`, `ended_at`.

## Behavior

Lists all egress recordings. If `room` is provided, only egress recordings for that room are returned. Fires `success` with the items array.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.egress_list",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}"
  }
}
```

### With data flow

A recordings dashboard endpoint lists all egress recordings for a specific meeting room.

```json
{
  "get_meeting": {
    "type": "db.findOne",
    "services": { "database": "postgres" },
    "config": {
      "table": "meetings",
      "where": { "id": "{{ input.meeting_id }}" },
      "required": true
    }
  },
  "list_recordings": {
    "type": "lk.egress_list",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ nodes.get_meeting.room_name }}"
    }
  }
}
```

Output stored as `nodes.list_recordings`:
```json
{ "items": [{ "egress_id": "EG_abc", "room_name": "meeting-1", "status": "EGRESS_COMPLETE", "started_at": 1717200000, "ended_at": 1717203600 }] }
```

Downstream nodes access the recordings via `nodes.list_recordings.items`.

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/livekit`](../../examples/node-cookbook/livekit/README.md) — its README documents the exact request/response pair the integration suite executes.
