# Schedules

Files in `schedules/*.json`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique schedule identifier |
| `cron` | string | yes | Cron expression (6-field with seconds) |
| `timezone` | string | no | IANA timezone (e.g. `"America/New_York"`) |
| `timeout` | string | no | Per-job execution timeout as a Go duration (default `"5m"`) |
| `description` | string | no | Human-readable description |
| `trigger` | object | yes | Workflow trigger |
| `trigger.workflow` | string | yes | Workflow ID to execute |
| `trigger.input` | object | no | Input mapping passed to the workflow |
| `services` | object | no | Service wiring |
| `services.lock` | string | no | Cache service name for distributed locking |
| `lock` | object | no | Distributed lock configuration |
| `lock.enabled` | boolean | no | Enable distributed locking |
| `lock.ttl` | string | no | Lock TTL as a Go duration (auto-adjusted to exceed timeout) |

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

## Cron Expression Quick Reference

Noda uses 6-field cron expressions (with seconds) via `robfig/cron/v3`:

```
┌──────────── second (0-59)
│ ┌────────── minute (0-59)
│ │ ┌──────── hour (0-23)
│ │ │ ┌────── day of month (1-31)
│ │ │ │ ┌──── month (1-12)
│ │ │ │ │ ┌── day of week (0-6, 0=Sun)
│ │ │ │ │ │
* * * * * *
```

| Pattern | Description |
|---------|-------------|
| `0 0 2 * * *` | Every day at 2:00 AM |
| `0 0 * * * *` | Every hour on the hour |
| `0 */15 * * * *` | Every 15 minutes |
| `0 0 0 * * 1` | Every Monday at midnight |
| `0 0 9 * * 1-5` | Weekdays at 9:00 AM |
| `0 0 0 1 * *` | First day of every month at midnight |
| `0 30 6 * * *` | Every day at 6:30 AM |
| `0 0 0 * * 0` | Every Sunday at midnight |

## Timezone Support

Set the `timezone` field to an IANA timezone name. If omitted, the server's local timezone is used. Invalid timezone names are logged as warnings and fall back to the server default.

```json
{
  "id": "eu-morning-report",
  "cron": "0 0 8 * * 1-5",
  "timezone": "Europe/Berlin",
  "trigger": {
    "workflow": "generate-morning-report"
  }
}
```

## Distributed Locking

When running multiple Noda instances, every instance registers the same cron jobs. Without locking, a scheduled job runs on all instances simultaneously. Distributed locking ensures only one instance executes each job.

Noda uses Redis `SET NX` (set-if-not-exists) to acquire a lock before running the job. The lock key includes the schedule ID and the current minute timestamp, so each scheduled firing gets its own lock:

```
noda:schedule:<schedule_id>:<unix_minute>
```

If another instance already holds the lock, the job is skipped on this instance (logged as "lock not acquired, skipping"). After the job completes, the lock is released via a Lua compare-and-delete script that only deletes the key if the token matches -- preventing one instance from releasing another's lock.

The lock TTL is automatically adjusted to be at least 30 seconds longer than the job timeout, ensuring the lock outlives the job even in slow-execution cases. If the lock TTL is not set, it defaults to 5 minutes.

### Enabling Distributed Locking

Point `services.lock` at a cache (Redis) service defined in `noda.json`, then enable locking:

```json
{
  "id": "hourly-sync",
  "cron": "0 0 * * * *",
  "services": {
    "lock": "app-cache"
  },
  "lock": {
    "enabled": true,
    "ttl": "10m"
  },
  "trigger": {
    "workflow": "sync-external-data",
    "input": {
      "source": "partner-api"
    }
  }
}
```

## Error Handling

When a scheduled workflow fails:

1. The error is logged with the schedule ID, trace ID, and duration.
2. The failure is recorded in the in-memory job history (capped at 1,000 entries).
3. The scheduler continues running -- a failed job does not stop subsequent firings.
4. If the job panics, the panic is recovered, logged with a stack trace, and recorded as a failed run.

The job timeout (default 5 minutes, configurable via `timeout`) cancels the workflow context if the job runs too long. This prevents stuck jobs from blocking the scheduler.

Scheduled workflows do not automatically retry on failure. Each cron firing is independent -- if a job fails, it will run again at the next scheduled time. If you need retry logic, use retry configuration on individual nodes within the workflow (see [Workflow Patterns](../04-guides/workflow-patterns.md#retry-configuration)).

## Service Wiring

Scheduled workflows can reference services defined in `noda.json` just like route-triggered workflows. Use `trigger.input` to pass static values or schedule metadata:

```json
{
  "id": "weekly-report",
  "cron": "0 0 9 * * 1",
  "timezone": "America/New_York",
  "timeout": "15m",
  "description": "Generate weekly summary report every Monday at 9 AM ET",
  "trigger": {
    "workflow": "generate-weekly-report",
    "input": {
      "report_type": "weekly",
      "schedule_id": "{{ schedule.id }}"
    }
  }
}
```

Inside the trigger input, the expression context provides `schedule.id` and `schedule.cron` for the current schedule.

## Realistic Examples

### Daily Cleanup Job

Delete expired sessions and soft-deleted records older than 30 days:

**schedules/daily-cleanup.json**

```json
{
  "id": "daily-cleanup",
  "cron": "0 0 3 * * *",
  "timezone": "UTC",
  "timeout": "10m",
  "description": "Clean up expired data every day at 3 AM UTC",
  "services": {
    "lock": "app-cache"
  },
  "lock": {
    "enabled": true
  },
  "trigger": {
    "workflow": "cleanup-expired-data",
    "input": {
      "retention_days": 30
    }
  }
}
```

**workflows/cleanup-expired-data.json**

```json
{
  "id": "cleanup-expired-data",
  "nodes": {
    "delete_sessions": {
      "type": "db.exec",
      "config": {
        "sql": "DELETE FROM sessions WHERE expires_at < NOW()",
        "service": "main-db"
      }
    },
    "delete_soft_deleted": {
      "type": "db.exec",
      "config": {
        "sql": "DELETE FROM users WHERE deleted_at IS NOT NULL AND deleted_at < NOW() - INTERVAL '30 days'",
        "service": "main-db"
      }
    },
    "log_result": {
      "type": "util.log",
      "config": {
        "message": "Cleanup complete: sessions={{ nodes.delete_sessions.rows_affected }}, users={{ nodes.delete_soft_deleted.rows_affected }}"
      }
    }
  },
  "edges": [
    { "from": "delete_sessions", "to": "delete_soft_deleted", "output": "success" },
    { "from": "delete_soft_deleted", "to": "log_result", "output": "success" }
  ]
}
```

### Hourly Data Sync

Pull data from an external API every hour:

```json
{
  "id": "hourly-partner-sync",
  "cron": "0 0 * * * *",
  "timeout": "10m",
  "services": {
    "lock": "app-cache"
  },
  "lock": {
    "enabled": true,
    "ttl": "15m"
  },
  "trigger": {
    "workflow": "sync-partner-data",
    "input": {
      "api_url": "{{ secrets.PARTNER_API_URL }}",
      "batch_size": 100
    }
  }
}
```

### Weekly Report

Generate and email a report every Monday morning:

```json
{
  "id": "weekly-sales-report",
  "cron": "0 0 9 * * 1",
  "timezone": "America/Chicago",
  "timeout": "15m",
  "description": "Weekly sales summary emailed to team leads",
  "services": {
    "lock": "app-cache"
  },
  "lock": {
    "enabled": true
  },
  "trigger": {
    "workflow": "generate-sales-report",
    "input": {
      "recipients": ["sales-leads@example.com"],
      "period": "weekly"
    }
  }
}
```
