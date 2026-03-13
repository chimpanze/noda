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
