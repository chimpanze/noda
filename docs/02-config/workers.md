# Workers

Files in `workers/*.json`. Each file defines one event-driven worker.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique worker identifier |
| `services` | object | yes | Stream or PubSub service binding |
| `subscribe` | object | yes | Subscription configuration |
| `subscribe.topic` | string | yes | Topic or stream name |
| `subscribe.group` | string | yes | Consumer group name |
| `concurrency` | integer | no | Concurrent message processing (default: 1) |
| `retry` | object | no | Retry configuration |
| `retry.max_attempts` | integer | no | Max delivery attempts |
| `retry.dlq` | string | no | Dead letter queue topic |
| `trigger` | object | yes | Workflow trigger |

```json
{
  "id": "order-processor",
  "services": {
    "stream": "redis-stream"
  },
  "subscribe": {
    "topic": "orders.created",
    "group": "order-processors"
  },
  "concurrency": 5,
  "retry": {
    "max_attempts": 3,
    "dlq": "orders.failed"
  },
  "trigger": {
    "workflow": "process-order",
    "input": {
      "order_id": "{{ message.payload.order_id }}"
    }
  }
}
```
