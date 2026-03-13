# control.loop

Iterates a sub-workflow over each item in a collection.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `collection` | string (expr) | yes | Expression resolving to an array |
| `workflow` | string | yes | Sub-workflow ID to execute per item (static) |
| `input` | object | no | Input template -- `$item` and `$index` available |

## Outputs

`done`, `error`

The `done` output receives an array of all iteration results.

## Behavior

Resolves `collection` to an array. For each element, invokes the sub-workflow with `$.input` populated from the `input` map (`$item` = current element, `$index` = zero-based index). Iterations run sequentially. Collects the output from each iteration's `workflow.output` node into an array. When all iterations complete, fires `done` with the collected array. If any iteration fails (sub-workflow errors with no error edge), fires `error` immediately -- remaining iterations are skipped.

Recursion limit: `control.loop` and `workflow.run` share a maximum recursion depth of 64. Exceeding this limit returns a `RECURSION_DEPTH_EXCEEDED` error. This prevents stack exhaustion from deeply nested or runaway loops.

## Example

```json
{
  "type": "control.loop",
  "config": {
    "collection": "{{ nodes.fetch }}",
    "workflow": "process-item",
    "input": {
      "item_id": "{{ $item.id }}",
      "index": "{{ $index }}"
    }
  }
}
```
