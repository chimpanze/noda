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
