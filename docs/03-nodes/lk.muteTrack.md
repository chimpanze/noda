# lk.muteTrack

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

Output: `{muted: <bool>, track_sid: "...", track_name: "...", track_type: "..."}`

## Behavior

Server-side mutes or unmutes a participant's published track. The participant and all subscribers receive a track mute event. Fires `success` with the track info.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.muteTrack",
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
    "type": "lk.participantGet",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ input.room_name }}",
      "identity": "{{ input.user_id }}"
    }
  },
  "mute_audio": {
    "type": "lk.muteTrack",
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

Downstream nodes can check `nodes.mute_audio.muted` or `nodes.mute_audio.track_type`.
