# transform.map

Transforms each item in an array using an expression.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `collection` | string (expr) | yes | Expression resolving to array |
| `expression` | string (expr) | yes | Expression applied to each item |

`$item` and `$index` are available in the expression.

## Outputs

`success`, `error`

## Behavior

Resolves `collection` to an array. For each element, evaluates `expression` with `$item` as the current element and `$index` as the index. Produces a new array of the results. Fires `success` with the mapped array.

## Example

```json
{
  "type": "transform.map",
  "config": {
    "collection": "{{ nodes.fetch }}",
    "expression": "{{ { 'id': $item.id, 'name': upper($item.name) } }}"
  }
}
```

### With data flow

After fetching a list of users from the database, `transform.map` reshapes each row into a lighter API response format.

```json
{
  "format_users": {
    "type": "transform.map",
    "config": {
      "collection": "{{ nodes.list_users }}",
      "expression": "{{ { 'id': $item.id, 'display_name': $item.first_name + ' ' + $item.last_name, 'email': $item.email } }}"
    }
  }
}
```

Output stored as `nodes.format_users`:
```json
[
  { "id": 1, "display_name": "Jane Doe", "email": "jane@example.com" },
  { "id": 2, "display_name": "Bob Smith", "email": "bob@example.com" }
]
```

Downstream nodes access the mapped array via `nodes.format_users` or individual items via `nodes.format_users[0].display_name`.
