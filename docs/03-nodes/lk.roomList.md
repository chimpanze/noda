# lk.roomList

Lists LiveKit rooms.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `names` | array | no | Optional room name filter |

## Outputs

`success`, `error`

Output: `{rooms: [...]}`

## Behavior

Lists all active rooms on the LiveKit server. If `names` is provided, only rooms matching those names are returned. Fires `success` with the rooms array.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.roomList",
  "services": { "livekit": "lk" },
  "config": {}
}
```

### With data flow

An admin dashboard endpoint lists all active rooms and returns them with participant counts.

```json
{
  "list_rooms": {
    "type": "lk.roomList",
    "services": { "livekit": "lk" },
    "config": {}
  },
  "respond": {
    "type": "response.json",
    "config": {
      "body": {
        "rooms": "{{ nodes.list_rooms.rooms }}"
      }
    }
  }
}
```

Output stored as `nodes.list_rooms`:
```json
{ "rooms": [{ "sid": "RM_abc", "name": "meeting-1", "num_participants": 3 }, { "sid": "RM_def", "name": "meeting-2", "num_participants": 1 }] }
```

Downstream nodes access the list via `nodes.list_rooms.rooms`.
