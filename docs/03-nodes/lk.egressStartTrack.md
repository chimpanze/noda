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
