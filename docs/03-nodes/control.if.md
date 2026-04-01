# control.if

Conditional branching based on an expression.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `condition` | string (expr) | yes | Expression to evaluate |

## Outputs

`then`, `else`, `error`

## Behavior

Resolves `condition`. If truthy, fires `then`. If falsy, fires `else`. If the expression fails to evaluate, fires `error`. The output data is the resolved condition value.

## Example

```json
{
  "type": "control.if",
  "config": {
    "condition": "{{ len(nodes.fetch) > 0 }}"
  }
}
```

### With data flow

A database query returns a list of users; the condition branches on whether results exist.

```json
{
  "check_results": {
    "type": "control.if",
    "config": {
      "condition": "{{ nodes.find_users.total > 0 }}"
    }
  }
}
```

When `nodes.find_users` produced `{"rows": [...], "total": 3}`, the condition resolves to `true` and the `then` output fires. Output stored as `nodes.check_results`:
```json
true
```

The `then` branch receives this value; the `else` branch would receive `false`.
