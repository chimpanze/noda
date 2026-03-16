# lk.participantUpdate

Updates a participant's metadata or permissions.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name |
| `identity` | string (expr) | yes | Participant identity |
| `metadata` | string (expr) | no | New metadata value |
| `permissions` | object | no | Permission overrides |

### Permission Keys

| Key | Type | Description |
|-----|------|-------------|
| `canPublish` | boolean | Allow publishing tracks |
| `canSubscribe` | boolean | Allow subscribing to tracks |
| `canPublishData` | boolean | Allow publishing data messages |
| `hidden` | boolean | Hide participant from others |

## Outputs

`success`, `error`

Output: updated participant object.

## Behavior

Updates the specified participant's metadata and/or permissions. Other participants receive update events. At least one of `metadata` or `permissions` should be provided. Fires `success` with the updated participant object.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.participantUpdate",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}",
    "identity": "{{ input.user_id }}",
    "metadata": "{{ toJSON(input.user_metadata) }}",
    "permissions": {
      "canPublish": true,
      "canSubscribe": true
    }
  }
}
```
