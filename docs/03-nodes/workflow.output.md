# workflow.output

Terminal node that declares a named output for the workflow. Used in sub-workflows called by `workflow.run`.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Output name |
| `data` | any (expr) | no | Output data |

## Outputs

None (terminal node).

## Behavior

When reached, the sub-workflow completes with this output name and the resolved `data`. The parent's `workflow.run` node fires the output matching this `name`.

All `workflow.output` nodes in a sub-workflow must have unique `name` values. They must be on mutually exclusive branches (validated at startup).

## Example

```json
{
  "type": "workflow.output",
  "config": {
    "name": "created",
    "data": "{{ nodes.insert }}"
  }
}
```

### With data flow

A sub-workflow's final step assembles output from multiple preceding nodes and returns it to the parent.

```json
{
  "output_success": {
    "type": "workflow.output",
    "config": {
      "name": "success",
      "data": {
        "order_id": "{{ nodes.insert_order.id }}",
        "total": "{{ nodes.calculate_total.amount }}",
        "receipt_url": "{{ nodes.generate_receipt.url }}"
      }
    }
  }
}
```

When `nodes.insert_order` produced `{"id": 501}`, `nodes.calculate_total` produced `{"amount": 149.99}`, and `nodes.generate_receipt` produced `{"url": "/receipts/501.pdf"}`, the parent's `workflow.run` node receives:
```json
{ "order_id": 501, "total": 149.99, "receipt_url": "/receipts/501.pdf" }
```

This becomes the output of the parent's `workflow.run` node on its `success` port.
