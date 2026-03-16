# lk.egressStop

Stops an active egress (recording).

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `egress_id` | string (expr) | yes | Egress ID to stop |

## Outputs

`success`, `error`

Output: final egress info with `egress_id`, `room_id`, `room_name`, `status`, `started_at`, `ended_at`.

## Behavior

Stops an active egress recording. The final output file is uploaded to the configured storage backend. Fires `success` with the final egress info once the recording has been finalized.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.egressStop",
  "services": { "livekit": "lk" },
  "config": {
    "egress_id": "{{ input.egress_id }}"
  }
}
```
