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
