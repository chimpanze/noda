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
| `timeout` | duration string | no | Per-message processing timeout (default: `5m`) |
| `retry` | object | no | Pending-reclaim and poison-cap configuration |
| `retry.min_idle` | duration string | no | How long a pending message must be idle before the reaper reclaims it (default: `timeout`) |
| `retry.max_attempts` | integer | no | Hard cap on delivery attempts when no `dead_letter` topic is configured (default: 10) |
| `dead_letter` | object | no | Dead letter queue configuration |
| `dead_letter.topic` | string | no | Stream to publish failed messages to |
| `dead_letter.after` | integer | no | Dead-letter the message after this many delivery attempts |
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
  "timeout": "30s",
  "retry": {
    "min_idle": "90s",
    "max_attempts": 10
  },
  "dead_letter": {
    "topic": "orders.failed",
    "after": 3
  },
  "trigger": {
    "workflow": "process-order",
    "input": {
      "order_id": "{{ message.payload.order_id }}"
    }
  }
}
```

## Retry and poison-message handling

When a worker fails to process a message (workflow error or panic), the message stays in the Redis Streams pending-entries list (PEL). A per-worker reaper goroutine periodically calls `XAUTOCLAIM` to reclaim entries that have been idle longer than `retry.min_idle` and reprocesses them through the normal disposition path.

### `retry.min_idle`

`min_idle` is the minimum time a pending entry must be idle before the reaper reclaims it. It is automatically clamped up to at least the worker `timeout` value (with a 60-second floor), so the reaper never steals a message that a live consumer is still processing. For example, with a 30-second handler timeout, `min_idle` cannot be set below 60 seconds.

### Dead-lettering with `dead_letter.after`

When `dead_letter` is configured, `dead_letter.after` counts delivery attempts. Once the delivery count reaches `after`, the worker moves the message to the dead-letter topic and acknowledges the original — removing it from the PEL. The message is not dropped before `after` attempts regardless of `retry.max_attempts`.

```json
{
  "dead_letter": {
    "topic": "orders.failed",
    "after": 3
  }
}
```

### No dead-letter: drop after `max_attempts`

Without a `dead_letter` topic, a message that keeps failing is dropped (acknowledged) with a loud ERROR log once the delivery count reaches `retry.max_attempts` (default: 10). Configure `dead_letter` to retain poison messages for forensic inspection.
