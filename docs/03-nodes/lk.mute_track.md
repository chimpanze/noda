# lk.mute_track

Mutes or unmutes a published track.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name |
| `identity` | string (expr) | yes | Participant identity |
| `track_sid` | string (expr) | yes | Track SID to mute/unmute |
| `muted` | boolean | yes | `true` to mute, `false` to unmute |

## Outputs

`success`, `error`

Output: `{muted: <bool>}` — plus `track_sid`, `track_name`, `track_type` when the LiveKit response includes track info (they are absent otherwise, so guard downstream references).

## Behavior

Server-side mutes or unmutes a participant's published track. The participant and all subscribers receive a track mute event. Fires `success` with the track info.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.mute_track",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}",
    "identity": "{{ input.user_id }}",
    "track_sid": "{{ input.track_sid }}",
    "muted": true
  }
}
```

### With data flow

A moderation endpoint fetches a participant, then mutes their audio track.

```json
{
  "get_participant": {
    "type": "lk.participant_get",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ input.room_name }}",
      "identity": "{{ input.user_id }}"
    }
  },
  "mute_audio": {
    "type": "lk.mute_track",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ input.room_name }}",
      "identity": "{{ nodes.get_participant.identity }}",
      "track_sid": "{{ input.track_sid }}",
      "muted": true
    }
  }
}
```

Output stored as `nodes.mute_audio`:
```json
{ "muted": true, "track_sid": "TR_xyz", "track_name": "microphone", "track_type": "AUDIO" }
```

(The three `track_*` fields appear only when LiveKit returns track info.) Downstream nodes can check `nodes.mute_audio.muted`, and `nodes.mute_audio.track_type` when present.

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/livekit`](../../examples/node-cookbook/livekit/README.md) — its README documents the exact request/response pair the integration suite executes.
