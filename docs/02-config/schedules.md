# Schedules

Files in `schedules/*.json`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique schedule identifier |
| `cron` | string | yes | Cron expression |
| `trigger` | object | yes | Workflow trigger |

```json
{
  "id": "daily-cleanup",
  "cron": "0 2 * * *",
  "trigger": {
    "workflow": "cleanup-expired",
    "input": {}
  }
}
```
