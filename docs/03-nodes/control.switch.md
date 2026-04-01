# control.switch

Multi-way branching with case matching.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `expression` | string (expr) | yes | Expression to evaluate |
| `cases` | array of strings | yes | Case values to match (static) |

## Outputs

One per case value + `default`, `error`

## Behavior

Resolves `expression`. Compares the result against each case name as a string. If a match is found, fires that output. If no match, fires `default`. If the expression fails, fires `error`. The output data is the resolved expression value.

Case names are static string literals. They define the node's output ports and must be known at startup.

## Example

```json
{
  "type": "control.switch",
  "config": {
    "expression": "{{ input.action }}",
    "cases": ["create", "update", "delete"]
  }
}
```

### With data flow

A workflow receives a webhook event and routes it based on the event type from a previous node.

```json
{
  "route_event": {
    "type": "control.switch",
    "config": {
      "expression": "{{ nodes.parse_webhook.event_type }}",
      "cases": ["order.created", "order.updated", "order.cancelled"]
    }
  }
}
```

When `nodes.parse_webhook` produced `{"event_type": "order.updated", "payload": {...}}`, the expression resolves to `"order.updated"` and the matching case output fires. Output stored as `nodes.route_event`:
```json
"order.updated"
```

The `order.updated` branch continues; unmatched values fire `default`.
