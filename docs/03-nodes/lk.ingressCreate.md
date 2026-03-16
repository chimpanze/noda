# lk.ingressCreate

Creates a LiveKit ingress endpoint for streaming into a room.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `input_type` | string (expr) | yes | Input type: `"rtmp"`, `"whip"`, or `"url"` |
| `room` | string (expr) | yes | Room to publish into |
| `participant_identity` | string (expr) | yes | Identity for the ingress participant |
| `participant_name` | string (expr) | no | Display name for the ingress participant |
| `url` | string (expr) | no | Source URL (required for `"url"` input type) |

## Outputs

`success`, `error`

Output: `{ingress_id: "...", url: "...", stream_key: "...", room: "...", participant_identity: "...", participant_name: "...", input_type: "..."}`

## Behavior

Creates an ingress endpoint that allows external sources to stream media into a LiveKit room. For `rtmp`, the response includes an RTMP URL and stream key. For `whip`, a WHIP endpoint URL is returned. For `url`, LiveKit pulls media from the specified source URL. Fires `success` with the ingress info.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.ingressCreate",
  "services": { "livekit": "lk" },
  "config": {
    "input_type": "rtmp",
    "room": "{{ input.room_name }}",
    "participant_identity": "{{ input.streamer_id }}",
    "participant_name": "{{ input.streamer_name }}"
  }
}
```
