# lk.token

Generates a LiveKit access token with configurable grants.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `identity` | string (expr) | yes | Participant identity |
| `room` | string (expr) | yes | Room name to grant access to |
| `name` | string (expr) | no | Participant display name |
| `metadata` | string (expr) | no | Participant metadata |
| `ttl` | string (expr) | no | Token time-to-live (default: `"6h"`) |
| `grants` | object | no | Map of grant booleans |

### Grant Keys

| Key | Type | Description |
|-----|------|-------------|
| `roomJoin` | boolean | Allow joining the room (default: true) |
| `roomCreate` | boolean | Allow creating rooms |
| `roomList` | boolean | Allow listing rooms |
| `roomAdmin` | boolean | Full room admin access |
| `canPublish` | boolean | Allow publishing tracks |
| `canSubscribe` | boolean | Allow subscribing to tracks |
| `canPublishData` | boolean | Allow publishing data messages |
| `canPublishSources` | array | Allowed track source types |
| `canUpdateOwnMetadata` | boolean | Allow updating own metadata |
| `hidden` | boolean | Hide participant from others |
| `recorder` | boolean | Mark as recorder participant |

## Outputs

`success`, `error`

Output: `{token: "<jwt>", identity: "...", room: "..."}`

## Behavior

Creates a signed JWT access token using the LiveKit service credentials. The token includes a `VideoGrant` scoped to the specified room with `roomJoin: true` by default. Additional grants can be set via the `grants` config. Clients use this token to connect directly to LiveKit for media transport.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.token",
  "services": { "livekit": "lk" },
  "config": {
    "identity": "{{ auth.user_id }}",
    "room": "{{ input.room_name }}",
    "name": "{{ auth.claims.name }}",
    "ttl": "2h",
    "grants": {
      "canPublish": true,
      "canSubscribe": true,
      "canPublishData": true
    }
  }
}
```
