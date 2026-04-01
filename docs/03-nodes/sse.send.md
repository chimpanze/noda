# sse.send

Sends a Server-Sent Event to a channel.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `channel` | string (expr) | yes | Channel name (supports wildcards) |
| `data` | any (expr) | yes | Event data |
| `event` | string (expr) | no | Event type name |
| `id` | string (expr) | no | Event ID |

## Outputs

`success`, `error`

## Behavior

Resolves all fields. Calls `services["connections"].SendSSE(channel, event, data, id)`. Buffered, non-blocking. Fires `success` with no data.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `connections` | `sse` | Yes |

## Example

```json
{
  "type": "sse.send",
  "services": { "connections": "notifications" },
  "config": {
    "channel": "{{ 'user.' + auth.user_id }}",
    "event": "notification",
    "data": {
      "title": "{{ input.title }}",
      "body": "{{ input.body }}"
    }
  }
}
```

### With data flow

After a new comment is inserted, notify the post author via SSE using data from upstream nodes.

```json
{
  "notify_author": {
    "type": "sse.send",
    "services": { "connections": "notifications" },
    "config": {
      "channel": "{{ 'user.' + nodes.get_post.author_id }}",
      "event": "new_comment",
      "data": {
        "post_id": "{{ nodes.insert_comment.post_id }}",
        "comment_id": "{{ nodes.insert_comment.id }}",
        "commenter": "{{ nodes.insert_comment.author_name }}"
      }
    }
  }
}
```

When `nodes.get_post` produced `{"author_id": "user-77", "title": "My Post"}` and `nodes.insert_comment` produced `{"id": 302, "post_id": 15, "author_name": "Bob"}`, the SSE event is sent to channel `user.user-77`. Output stored as `nodes.notify_author`:
```json
null
```
