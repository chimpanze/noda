# transform.merge

Merges multiple arrays using different strategies.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `mode` | string | yes | `"append"`, `"match"`, or `"position"` (static) |
| `inputs` | array of strings | yes | Expressions resolving to arrays |
| `match` | object | no | For `match` mode |
| `match.type` | string | no | `"inner"`, `"outer"`, or `"enrich"` |
| `match.fields` | object | no | `left` and `right` join key field names |

## Outputs

`success`, `error`

## Behavior

- **append** -- concatenates all input arrays into a single array. Works with any number of inputs.
- **match** -- joins two inputs by matching field values. Requires exactly two inputs. `inner` keeps only matching rows. `outer` keeps all rows from both. `enrich` keeps all rows from the first input and adds matching data from the second.
- **position** -- combines inputs by index. Row 0 from input A merges with row 0 from input B. Requires inputs of equal length.

## Example

```json
{
  "type": "transform.merge",
  "config": {
    "mode": "match",
    "inputs": ["{{ nodes.users }}", "{{ nodes.profiles }}"],
    "match": {
      "type": "inner",
      "fields": { "left": "id", "right": "user_id" }
    }
  }
}
```
