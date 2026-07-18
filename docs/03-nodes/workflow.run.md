# workflow.run

Executes a sub-workflow.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `workflow` | string | yes | Sub-workflow ID (static) |
| `input` | object | no | Input data mapping |
| `transaction` | boolean | no | Wrap in database transaction |

## Outputs

Exactly two ports: `success` and `error`. The sub-workflow's `workflow.output` name is not surfaced as a port -- any name other than `error` routes through `success`, with that output node's data preserved. Branch on the data, not the port.

## Behavior

Resolves `input` expressions and populates the sub-workflow's `$.input`. Executes the sub-workflow. Whichever `workflow.output` node fires determines the data this node emits in the parent; if that output's `name` is `"error"` it routes to this node's `error` port, otherwise it routes to `success` (see `workflow.output.md`). If the sub-workflow fails without reaching a `workflow.output`, fires `error`. When `transaction: true`, the `services.database` slot must be filled -- the engine wraps execution in a database transaction and swaps the connection for all `db.*` nodes inside.

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

### With data flow

A parent workflow validates input and delegates creation to a sub-workflow, passing data from upstream nodes.

```json
{
  "run_checkout": {
    "type": "workflow.run",
    "services": { "database": "postgres" },
    "config": {
      "workflow": "checkout",
      "input": {
        "cart_id": "{{ nodes.lookup_cart.id }}",
        "items": "{{ nodes.lookup_cart.items }}",
        "shipping_address": "{{ nodes.get_profile.address }}"
      },
      "transaction": true
    }
  }
}
```

When `nodes.lookup_cart` produced `{"id": 88, "items": [{"sku": "X1", "qty": 1}]}` and `nodes.get_profile` produced `{"address": {"city": "Berlin"}}`, those values populate the sub-workflow's `input`. The sub-workflow's `workflow.output` node reached had `name: "confirmed"`, so `run_checkout` routes to its `success` port with that node's data. Output stored as `nodes.run_checkout`:
```json
{ "order_id": 501, "status": "confirmed" }
```

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/workflow`](../../examples/node-cookbook/workflow/README.md) — its README documents the exact request/response pair the integration suite executes.
