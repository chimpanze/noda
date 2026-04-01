# lk.egressStartRoomComposite

Starts a room composite egress (recording).

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name to record |
| `layout` | string (expr) | no | Layout template (default: `"speaker-dark"`) |
| `audio_only` | boolean | no | Record audio only |
| `output` | object | yes | Output storage configuration |

### Output Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | no | Storage type: `"s3"`, `"gcs"`, `"azure"`, `"file"` (default: `"file"`) |
| `filepath` | string | no | Output file path |
| `bucket` | string | for s3/gcs | Storage bucket name |
| `region` | string | for s3 | AWS region |
| `access_key` | string | for s3 | AWS access key |
| `secret` | string | for s3 | AWS secret key |
| `endpoint` | string | no | Custom S3 endpoint |
| `credentials` | string | for gcs | GCP credentials JSON |
| `account_name` | string | for azure | Azure account name |
| `account_key` | string | for azure | Azure account key |
| `container_name` | string | for azure | Azure container name |

## Outputs

`success`, `error`

Output: egress info with `egress_id`, `room_id`, `room_name`, `status`, `started_at`, `ended_at`.

## Behavior

Starts recording all audio/video tracks in the room as a composite layout. The recording is uploaded to the specified storage backend. Use `lk.egressStop` to stop the recording. Fires `success` with egress info including the `egress_id` needed to stop it.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.egressStartRoomComposite",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}",
    "layout": "speaker-dark",
    "output": {
      "type": "s3",
      "bucket": "recordings",
      "region": "us-east-1",
      "filepath": "recordings/{{ input.room_name }}/{{ $timestamp() }}.mp4"
    }
  }
}
```

### With data flow

A start-recording endpoint looks up the meeting, starts a room composite egress, then stores the egress ID for later stopping.

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
  "start_recording": {
    "type": "lk.egressStartRoomComposite",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ nodes.get_meeting.room_name }}",
      "layout": "grid-dark",
      "output": {
        "type": "s3",
        "bucket": "recordings",
        "filepath": "{{ nodes.get_meeting.room_name + '/' + $timestamp() + '.mp4' }}"
      }
    }
  },
  "save_egress": {
    "type": "db.update",
    "services": { "database": "postgres" },
    "config": {
      "table": "meetings",
      "where": { "id": "{{ input.meeting_id }}" },
      "data": { "egress_id": "{{ nodes.start_recording.egress_id }}" }
    }
  }
}
```

Output stored as `nodes.start_recording`:
```json
{ "egress_id": "EG_abc123", "room_id": "RM_xyz", "room_name": "meeting-1", "status": "EGRESS_ACTIVE", "started_at": 1717200000 }
```

Downstream nodes access the egress ID via `nodes.start_recording.egress_id`.
