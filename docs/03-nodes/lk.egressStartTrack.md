# lk.egressStartTrack

Starts a single track egress (recording).

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name |
| `track_sid` | string (expr) | yes | Track SID to record |
| `output` | object | yes | Output storage configuration (same format as `lk.egressStartRoomComposite`) |

## Outputs

`success`, `error`

Output: egress info with `egress_id`, `room_id`, `room_name`, `status`, `started_at`, `ended_at`.

## Behavior

Records a single audio or video track to the specified storage backend. Useful for recording individual participants or specific tracks. Use `lk.egressStop` to stop. Fires `success` with egress info.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.egressStartTrack",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}",
    "track_sid": "{{ input.track_sid }}",
    "output": {
      "type": "s3",
      "bucket": "recordings",
      "filepath": "tracks/{{ input.track_sid }}.ogg"
    }
  }
}
```

### With data flow

A record-track endpoint fetches a participant to verify they exist, then starts recording their specific track.

```json
{
  "get_participant": {
    "type": "lk.participantGet",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ input.room_name }}",
      "identity": "{{ input.user_id }}"
    }
  },
  "record_track": {
    "type": "lk.egressStartTrack",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ input.room_name }}",
      "track_sid": "{{ input.track_sid }}",
      "output": {
        "type": "s3",
        "bucket": "recordings",
        "filepath": "{{ 'tracks/' + nodes.get_participant.identity + '/' + input.track_sid + '.ogg' }}"
      }
    }
  }
}
```

Output stored as `nodes.record_track`:
```json
{ "egress_id": "EG_track1", "room_id": "RM_xyz", "room_name": "meeting-1", "status": "EGRESS_ACTIVE", "started_at": 1717200000 }
```

Downstream nodes access the egress ID via `nodes.record_track.egress_id`.
