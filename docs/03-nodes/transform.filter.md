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

### With data flow

After fetching all tasks for a project, `transform.filter` keeps only the overdue ones for a reminder notification.

```json
{
  "overdue_tasks": {
    "type": "transform.filter",
    "config": {
      "collection": "{{ nodes.list_tasks }}",
      "expression": "{{ $item.status != 'done' && $item.due_date < now() }}"
    }
  }
}
```

Output stored as `nodes.overdue_tasks`:
```json
[
  { "id": 3, "title": "Review PR", "status": "open", "due_date": "2026-03-28T00:00:00Z" },
  { "id": 7, "title": "Update docs", "status": "in_progress", "due_date": "2026-03-30T00:00:00Z" }
]
```

Downstream nodes access the filtered array via `nodes.overdue_tasks` or check `len(nodes.overdue_tasks)` in a conditional.
