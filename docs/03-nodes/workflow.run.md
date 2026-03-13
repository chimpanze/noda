# workflow.run

Executes a sub-workflow. Outputs are dynamic -- they match the sub-workflow's `workflow.output` node names.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `workflow` | string | yes | Sub-workflow ID (static) |
| `input` | object | no | Input data mapping |
| `transaction` | boolean | no | Wrap in database transaction |

## Outputs

Dynamic from sub-workflow + `error`

## Behavior

Resolves `input` expressions and populates the sub-workflow's `$.input`. Executes the sub-workflow. Whichever `workflow.output` node fires determines which output this node emits in the parent, along with that output node's data. If the sub-workflow fails without reaching a `workflow.output`, fires `error`. When `transaction: true`, the `services.database` slot must be filled -- the engine wraps execution in a database transaction and swaps the connection for all `db.*` nodes inside.

Recursion limit: Recursive workflow calls (direct or indirect) are limited to a depth of 64, shared with `control.loop`. Exceeding this limit returns a `RECURSION_DEPTH_EXCEEDED` error.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `database` | `db` | Only when `transaction: true` |

## Example

```json
{
  "type": "workflow.run",
  "services": { "database": "postgres" },
  "config": {
    "workflow": "create-order",
    "input": {
      "user_id": "{{ input.user_id }}",
      "items": "{{ input.items }}"
    },
    "transaction": true
  }
}
```
