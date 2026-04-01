# lk.egressStop

Stops an active egress (recording).

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `egress_id` | string (expr) | yes | Egress ID to stop |

## Outputs

`success`, `error`

Output: final egress info with `egress_id`, `room_id`, `room_name`, `status`, `started_at`, `ended_at`.

## Behavior

Stops an active egress recording. The final output file is uploaded to the configured storage backend. Fires `success` with the final egress info once the recording has been finalized.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.egressStop",
  "services": { "livekit": "lk" },
  "config": {
    "egress_id": "{{ input.egress_id }}"
  }
}
```

### With data flow

A stop-recording endpoint reads the stored egress ID from the meeting record and stops it.

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
  "stop_recording": {
    "type": "lk.egressStop",
    "services": { "livekit": "lk" },
    "config": {
      "egress_id": "{{ nodes.get_meeting.egress_id }}"
    }
  }
}
```

Output stored as `nodes.stop_recording`:
```json
{ "egress_id": "EG_abc123", "room_id": "RM_xyz", "room_name": "meeting-1", "status": "EGRESS_COMPLETE", "started_at": 1717200000, "ended_at": 1717203600 }
```

Downstream nodes access the final status via `nodes.stop_recording.status` or `nodes.stop_recording.ended_at`.
