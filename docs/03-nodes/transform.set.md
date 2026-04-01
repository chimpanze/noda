# transform.set

Creates a new object with resolved field expressions.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `fields` | object | yes | Key-value map of field names to expressions |

## Outputs

`success`, `error`

## Behavior

Resolves each expression in `fields` and produces an output object with the resulting key-value pairs. If any expression fails to resolve, fires `error`.

## Example

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "full_name": "{{ input.first_name + ' ' + input.last_name }}",
      "created_at": "{{ now() }}",
      "role": "user"
    }
  }
}
```

### With data flow

After fetching a user from the database, `transform.set` reshapes the data into an API response format before passing it to a response node.

```json
{
  "build_profile": {
    "type": "transform.set",
    "config": {
      "fields": {
        "id": "{{ nodes.get_user.id }}",
        "name": "{{ nodes.get_user.first_name + ' ' + nodes.get_user.last_name }}",
        "email": "{{ nodes.get_user.email }}",
        "member_since": "{{ nodes.get_user.created_at }}",
        "task_count": "{{ nodes.count_tasks.count }}"
      }
    }
  }
}
```

Output stored as `nodes.build_profile`:
```json
{
  "id": 3,
  "name": "Jane Doe",
  "email": "jane@example.com",
  "member_since": "2025-01-10T08:00:00Z",
  "task_count": 17
}
```

Downstream nodes access fields via `nodes.build_profile.name` or `nodes.build_profile.task_count`.
