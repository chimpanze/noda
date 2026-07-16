# Workers

Files in `workers/*.json`. Each file defines one event-driven worker.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique worker identifier |
| `services` | object | yes | Stream service binding (`services.stream`) — workers consume from Redis Streams only |
| `middleware` | array of strings | no | Worker-level middleware applied to message processing |
| `subscribe` | object | yes | Subscription configuration |
| `subscribe.topic` | string | yes | Topic or stream name |
| `subscribe.group` | string | yes | Consumer group name |
| `concurrency` | integer | no | Concurrent message processing (default: 1) |
| `timeout` | duration string | no | Per-message processing timeout (default: `5m`) |
| `retry` | object | no | Pending-reclaim and poison-cap configuration |
| `retry.min_idle` | duration string | no | How long a pending message must be idle before the reaper reclaims it (default: `timeout` + 30s) |
| `retry.max_attempts` | integer | no | Hard cap on delivery attempts when no `dead_letter` topic is configured (default: 10) |
| `retry.dlq` | string | no | **Deprecated** alias for `dead_letter.topic`; still honored for older configs |
| `dead_letter` | object | no | Dead letter queue configuration |
| `dead_letter.topic` | string | required if `dead_letter` set | Stream to publish failed messages to; an empty topic disables dead-lettering with an ERROR log |
| `dead_letter.after` | integer | no | Dead-letter the message after this many delivery attempts (default: `retry.max_attempts`) |
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

Reclaimed messages are processed with the worker's `concurrency`, so one slow message does not delay redelivery of the others.

### `retry.min_idle`

`min_idle` is the minimum time a pending entry must be idle before the reaper reclaims it. It is automatically clamped up to at least the worker `timeout` plus a 30-second safety margin (with a 60-second floor). The margin covers the gap between Redis's idle clock (which starts at delivery) and the handler's timeout clock, plus the acknowledgement after the handler returns, so the reaper does not reclaim a message whose consumer is just finishing. For example, with a 90-second handler timeout, `min_idle` cannot be set below 120 seconds.

Workers deliver messages **at least once**: a reclaimed message may re-run a workflow whose side effects partially applied, and in rare cases (for example an acknowledgement lost to a Redis failure) a successfully processed message can be redelivered. Make workflows idempotent where duplicates matter.

### Dead-lettering with `dead_letter.after`

When `dead_letter` is configured with a topic, `dead_letter.after` counts delivery attempts. Once the delivery count reaches `after`, the worker moves the message to the dead-letter topic and acknowledges the original — removing it from the PEL. The message is not dropped before `after` attempts regardless of `retry.max_attempts`. If `after` is omitted it defaults to `retry.max_attempts`, so a configured dead-letter topic always receives poison messages instead of the max-attempts drop applying silently.

Older configs that used `retry.dlq` keep working: it is treated as `dead_letter.topic` (an explicit `dead_letter` block takes precedence). Prefer the `dead_letter` block in new configs.

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
