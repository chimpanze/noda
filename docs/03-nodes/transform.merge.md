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

### With data flow

Two separate queries fetch users and their department info. `transform.merge` joins them by matching `department_id` to enrich user records.

```json
{
  "enrich_users": {
    "type": "transform.merge",
    "config": {
      "mode": "match",
      "inputs": ["{{ nodes.list_users }}", "{{ nodes.list_departments }}"],
      "match": {
        "type": "enrich",
        "fields": { "left": "department_id", "right": "id" }
      }
    }
  }
}
```

Output stored as `nodes.enrich_users`:
```json
[
  { "id": 1, "name": "Jane Doe", "department_id": 10, "department_name": "Engineering" },
  { "id": 2, "name": "Bob Smith", "department_id": 20, "department_name": "Marketing" }
]
```

Downstream nodes access the merged data via `nodes.enrich_users` or individual fields like `nodes.enrich_users[0].department_name`.
