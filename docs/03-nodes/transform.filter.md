# transform.filter

Filters an array by a predicate expression.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `collection` | string (expr) | yes | Expression resolving to array |
| `expression` | string (expr) | yes | Predicate -- keeps items where truthy |

`$item` and `$index` are available in the expression.

## Outputs

`success`, `error`

## Behavior

Resolves `collection`. For each element, evaluates `expression`. Keeps items where the result is truthy. Fires `success` with the filtered array.

## Example

```json
{
  "type": "transform.filter",
  "config": {
    "collection": "{{ nodes.fetch }}",
    "expression": "{{ $item.status == 'active' }}"
  }
}
```
