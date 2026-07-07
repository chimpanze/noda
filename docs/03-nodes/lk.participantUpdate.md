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
| `recorder` | boolean | Mark as a recorder instance (deprecated upstream) |

Permissions are **merged**: the node reads the participant's current permission
set (one extra `GetParticipant` call) and overlays only the keys you provide,
so omitted keys keep their current values. Unknown or non-boolean keys are
rejected with an error. The read and the write are two calls — a concurrent
permission change in between can be lost.

## Outputs

`success`, `error`

Output: updated participant object.

## Behavior

Updates the specified participant's metadata and/or permissions. Other participants receive update events. At least one of `metadata` or `permissions` should be provided. When
`permissions` is present the node first fetches the participant's current
permissions and merges your overrides into them (LiveKit replaces the whole
permission set on update — before this merge, omitting a key silently revoked
it). Fires `success` with the updated participant object.

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

### With data flow

A promote-to-speaker endpoint fetches the participant's current info, then grants publish permissions.

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
  "promote": {
    "type": "lk.participantUpdate",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ input.room_name }}",
      "identity": "{{ nodes.get_participant.identity }}",
      "metadata": "{{ toJSON({role: 'speaker'}) }}",
      "permissions": {
        "canPublish": true,
        "canSubscribe": true,
        "canPublishData": true
      }
    }
  }
}
```

Output stored as `nodes.promote`:
```json
{ "sid": "PA_abc", "identity": "usr_42", "name": "Jane", "metadata": "{\"role\":\"speaker\"}", "state": "ACTIVE" }
```

Downstream nodes access the updated participant via `nodes.promote.metadata`.
