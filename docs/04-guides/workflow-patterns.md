# Workflow Patterns

Common patterns for building Noda workflows. Each pattern includes a complete workflow JSON example with nodes and edges.

---

## Error Handling

### Error Branches

Route errors to a dedicated response node using the `error` output port. Every node has `success` and `error` outputs. Connect the `error` output to a `response.error` node so failures produce a structured HTTP response instead of crashing the workflow.

```json
{
  "id": "create-user-safe",
  "nodes": {
    "create": {
      "type": "db.create",
      "services": { "database": "postgres" },
      "config": {
        "table": "users",
        "data": {
          "name": "{{ input.name }}",
          "email": "{{ input.email }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": "{{ nodes.create }}"
      }
    },
    "error_response": {
      "type": "response.error",
      "config": {
        "status": 500,
        "code": "CREATE_FAILED",
        "message": "Failed to create user"
      }
    }
  },
  "edges": [
    { "from": "create", "to": "respond", "output": "success" },
    { "from": "create", "to": "error_response", "output": "error" }
  ]
}
```

**Key point:** If a node fires `error` and no error edge exists, the entire workflow fails with an unhandled error. Always add error edges for nodes that can fail (database writes, HTTP calls, cache operations).

### Retry Configuration

Retries are configured on edges, not on nodes. When an error edge has a `retry` config, the engine re-executes the source node according to the retry policy. If any retry succeeds, the workflow follows the `success` output instead.

Use retries for transient failures: network timeouts, database locks, rate-limited APIs. Do not retry validation errors or permission failures -- those will fail every time.

```json
{
  "id": "call-external-api",
  "nodes": {
    "fetch": {
      "type": "http.request",
      "config": {
        "method": "POST",
        "url": "{{ secrets.PAYMENT_API_URL }}",
        "body": {
          "amount": "{{ input.amount }}",
          "currency": "{{ input.currency }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.fetch.body }}"
      }
    },
    "error_response": {
      "type": "response.error",
      "config": {
        "status": 502,
        "code": "PAYMENT_FAILED",
        "message": "Payment API unavailable"
      }
    }
  },
  "edges": [
    { "from": "fetch", "to": "respond", "output": "success" },
    {
      "from": "fetch", "to": "error_response", "output": "error",
      "retry": { "attempts": 3, "backoff": "exponential", "delay": "1s" }
    }
  ]
}
```

**Backoff strategies:**

| Strategy | Behavior |
|----------|----------|
| `fixed` | Wait the same `delay` between each attempt |
| `exponential` | Double the delay each attempt (1s, 2s, 4s, ...) |

### Dead Letter Queues

Workers can route persistently failing messages to a dead letter queue (DLQ) topic. After `max_attempts` delivery failures, the message is published to the `dlq` topic instead of being retried further. A second worker can subscribe to the DLQ for alerting, manual review, or retry logic.

```json
{
  "id": "process-payment",
  "services": {
    "stream": "redis-stream"
  },
  "subscribe": {
    "topic": "payments.pending",
    "group": "payment-processors"
  },
  "concurrency": 3,
  "retry": {
    "max_attempts": 5,
    "dlq": "payments.failed"
  },
  "trigger": {
    "workflow": "process-payment",
    "input": {
      "payment_id": "{{ message.payload.payment_id }}",
      "amount": "{{ message.payload.amount }}"
    }
  }
}
```

To handle DLQ messages, create a second worker that subscribes to the DLQ topic:

```json
{
  "id": "handle-failed-payments",
  "services": {
    "stream": "redis-stream"
  },
  "subscribe": {
    "topic": "payments.failed",
    "group": "dlq-handlers"
  },
  "trigger": {
    "workflow": "alert-failed-payment",
    "input": {
      "payment_id": "{{ message.payload.payment_id }}",
      "error": "{{ message.payload.error }}"
    }
  }
}
```

### Graceful Degradation

Check the cache first. If the cached value is `nil`, fall back to the database, then populate the cache for future requests. This pattern keeps the API responsive even when the cache is cold.

```json
{
  "id": "get-user-profile",
  "nodes": {
    "cache_lookup": {
      "type": "cache.get",
      "as": "cached",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'user:' + input.user_id }}"
      }
    },
    "check_cache": {
      "type": "control.if",
      "config": {
        "condition": "{{ nodes.cached.value != nil }}"
      }
    },
    "respond_cached": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.cached.value }}"
      }
    },
    "db_lookup": {
      "type": "db.findOne",
      "as": "user",
      "services": { "database": "postgres" },
      "config": {
        "table": "users",
        "where": { "id": "{{ input.user_id }}" }
      }
    },
    "populate_cache": {
      "type": "cache.set",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'user:' + input.user_id }}",
        "value": "{{ nodes.user }}",
        "ttl": 300
      }
    },
    "respond_fresh": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.user }}"
      }
    }
  },
  "edges": [
    { "from": "cache_lookup", "to": "check_cache", "output": "success" },
    { "from": "cache_lookup", "to": "db_lookup", "output": "error" },
    { "from": "check_cache", "to": "respond_cached", "output": "then" },
    { "from": "check_cache", "to": "db_lookup", "output": "else" },
    { "from": "db_lookup", "to": "populate_cache", "output": "success" },
    { "from": "populate_cache", "to": "respond_fresh", "output": "success" }
  ]
}
```

**Data flow:**
1. `cache_lookup` tries to get the value from Redis. If Redis itself fails, fall through to `db_lookup` via the error edge.
2. `check_cache` tests whether the cached value is non-nil. If found (`then`), respond immediately with the cached data.
3. If not found (`else`), query the database, write the result to cache with a 5-minute TTL, then respond.

---

## Parallelism

### Independent Parallel Nodes

Nodes with no edge connecting them execute in parallel automatically. The engine builds a dependency graph at compile time -- any node whose dependencies are all satisfied is dispatched immediately. No special configuration is needed.

```json
{
  "id": "list-tasks",
  "nodes": {
    "count": {
      "type": "db.query",
      "as": "total",
      "services": { "database": "postgres" },
      "config": {
        "sql": "SELECT COUNT(*) as count FROM tasks WHERE user_id = $1",
        "params": ["{{ auth.sub }}"]
      }
    },
    "fetch": {
      "type": "db.query",
      "as": "rows",
      "services": { "database": "postgres" },
      "config": {
        "sql": "SELECT * FROM tasks WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
        "params": ["{{ auth.sub }}", "{{ input.limit }}", "{{ (input.page - 1) * input.limit }}"]
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "data": "{{ nodes.rows }}",
          "total": "{{ nodes.total[0].count }}"
        }
      }
    }
  },
  "edges": [
    { "from": "count", "to": "respond", "output": "success" },
    { "from": "fetch", "to": "respond", "output": "success" }
  ]
}
```

**How it works:** Both `count` and `fetch` are entry nodes (no inbound edges), so they start simultaneously. The `respond` node has two inbound edges, making it an AND-join -- it waits for both queries to complete before executing.

### Fan-Out / Fan-In

Use `control.loop` to process each item in a collection through a sub-workflow. The loop runs iterations sequentially and collects all results into an array.

```json
{
  "id": "enrich-orders",
  "nodes": {
    "fetch_orders": {
      "type": "db.find",
      "as": "orders",
      "services": { "database": "postgres" },
      "config": {
        "table": "orders",
        "where": { "status": "pending" },
        "limit": 50
      }
    },
    "process_each": {
      "type": "control.loop",
      "as": "enriched",
      "config": {
        "collection": "{{ nodes.orders }}",
        "workflow": "enrich-single-order",
        "input": {
          "order_id": "{{ $item.id }}",
          "customer_id": "{{ $item.customer_id }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.enriched }}"
      }
    }
  },
  "edges": [
    { "from": "fetch_orders", "to": "process_each", "output": "success" },
    { "from": "process_each", "to": "respond", "output": "done" }
  ]
}
```

The sub-workflow `enrich-single-order` receives each order's data as `input.order_id` and `input.customer_id`, performs lookups or transformations, and returns enriched data via a `workflow.output` node. The `control.loop` node collects all iteration outputs into an array on the `done` port.

### Parallel with Error Handling

When parallel branches run concurrently, any node that fires `error` without an error edge cancels the entire workflow. To handle errors gracefully in parallel branches, add error edges to each branch independently.

```json
{
  "id": "parallel-with-errors",
  "nodes": {
    "fetch_user": {
      "type": "db.findOne",
      "as": "user",
      "services": { "database": "postgres" },
      "config": {
        "table": "users",
        "where": { "id": "{{ input.user_id }}" }
      }
    },
    "fetch_prefs": {
      "type": "cache.get",
      "as": "prefs",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'prefs:' + input.user_id }}"
      }
    },
    "default_prefs": {
      "type": "transform.set",
      "as": "prefs",
      "config": {
        "fields": {
          "value": { "theme": "light", "lang": "en" }
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "user": "{{ nodes.user }}",
          "preferences": "{{ nodes.prefs.value }}"
        }
      }
    },
    "error_response": {
      "type": "response.error",
      "config": {
        "status": 404,
        "code": "USER_NOT_FOUND",
        "message": "User not found"
      }
    }
  },
  "edges": [
    { "from": "fetch_user", "to": "respond", "output": "success" },
    { "from": "fetch_user", "to": "error_response", "output": "error" },
    { "from": "fetch_prefs", "to": "respond", "output": "success" },
    { "from": "fetch_prefs", "to": "default_prefs", "output": "error" },
    { "from": "default_prefs", "to": "respond", "output": "success" }
  ]
}
```

**Behavior:** `fetch_user` and `fetch_prefs` run in parallel. If the cache miss fires `error` on `fetch_prefs`, the workflow falls back to `default_prefs` instead of failing entirely. The `respond` node is an AND-join -- it waits for both the user data and preferences (from cache or defaults) before sending the response. If `fetch_user` fails, the workflow goes to `error_response` immediately.

---

## Data Transformation

### Fetch, Transform, Respond

Query the database, reshape the results with `transform.map`, then return the transformed data.

```json
{
  "id": "list-users-formatted",
  "nodes": {
    "fetch": {
      "type": "db.find",
      "as": "raw_users",
      "services": { "database": "postgres" },
      "config": {
        "table": "users",
        "select": ["id", "first_name", "last_name", "email", "created_at"]
      }
    },
    "format": {
      "type": "transform.map",
      "as": "users",
      "config": {
        "collection": "{{ nodes.raw_users }}",
        "expression": "{{ { 'id': $item.id, 'display_name': $item.first_name + ' ' + $item.last_name, 'email': $item.email, 'member_since': $item.created_at } }}"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "users": "{{ nodes.users }}" }
      }
    }
  },
  "edges": [
    { "from": "fetch", "to": "format", "output": "success" },
    { "from": "format", "to": "respond", "output": "success" }
  ]
}
```

### Filtering Results

Use `transform.filter` to remove items from a collection based on a predicate expression.

```json
{
  "id": "active-subscriptions",
  "nodes": {
    "fetch": {
      "type": "db.find",
      "as": "all_subs",
      "services": { "database": "postgres" },
      "config": {
        "table": "subscriptions",
        "where": { "user_id": "{{ input.user_id }}" }
      }
    },
    "filter_active": {
      "type": "transform.filter",
      "as": "active",
      "config": {
        "collection": "{{ nodes.all_subs }}",
        "expression": "{{ $item.status == 'active' && $item.expires_at > now() }}"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "subscriptions": "{{ nodes.active }}" }
      }
    }
  },
  "edges": [
    { "from": "fetch", "to": "filter_active", "output": "success" },
    { "from": "filter_active", "to": "respond", "output": "success" }
  ]
}
```

### Merging Data

Run parallel queries, then combine the results with `transform.merge`. The merge node supports `append` (concatenation), `match` (join by field), and `position` (zip by index).

```json
{
  "id": "user-with-orders",
  "nodes": {
    "fetch_users": {
      "type": "db.find",
      "as": "users",
      "services": { "database": "postgres" },
      "config": {
        "table": "users",
        "where": { "team_id": "{{ input.team_id }}" }
      }
    },
    "fetch_order_counts": {
      "type": "db.query",
      "as": "order_counts",
      "services": { "database": "postgres" },
      "config": {
        "sql": "SELECT user_id, COUNT(*) as total_orders FROM orders GROUP BY user_id"
      }
    },
    "merge": {
      "type": "transform.merge",
      "as": "enriched",
      "config": {
        "mode": "match",
        "inputs": ["{{ nodes.users }}", "{{ nodes.order_counts }}"],
        "match": {
          "type": "enrich",
          "fields": { "left": "id", "right": "user_id" }
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "members": "{{ nodes.enriched }}" }
      }
    }
  },
  "edges": [
    { "from": "fetch_users", "to": "merge", "output": "success" },
    { "from": "fetch_order_counts", "to": "merge", "output": "success" },
    { "from": "merge", "to": "respond", "output": "success" }
  ]
}
```

**Data flow:** Both queries run in parallel (no edges between them). The `merge` node is an AND-join that waits for both, then enriches each user with their order count using the `enrich` match type -- all users are kept, and matching order data is added.

---

## Conditional Logic

### Binary Decision

Use `control.if` to branch between two paths. The node evaluates a condition expression and fires `then` (truthy) or `else` (falsy).

```json
{
  "id": "get-task",
  "nodes": {
    "fetch": {
      "type": "db.findOne",
      "as": "task",
      "services": { "database": "postgres" },
      "config": {
        "table": "tasks",
        "where": {
          "id": "{{ input.task_id }}",
          "user_id": "{{ auth.sub }}"
        }
      }
    },
    "check_found": {
      "type": "control.if",
      "config": {
        "condition": "{{ nodes.task != nil }}"
      }
    },
    "respond_found": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.task }}"
      }
    },
    "respond_not_found": {
      "type": "response.error",
      "config": {
        "status": 404,
        "code": "NOT_FOUND",
        "message": "Task not found"
      }
    }
  },
  "edges": [
    { "from": "fetch", "to": "check_found", "output": "success" },
    { "from": "check_found", "to": "respond_found", "output": "then" },
    { "from": "check_found", "to": "respond_not_found", "output": "else" }
  ]
}
```

### Multi-Way Routing

Use `control.switch` when there are more than two branches. Define case values as static strings; the engine matches the evaluated expression against each case. Unmatched values fire `default`.

```json
{
  "id": "handle-webhook",
  "nodes": {
    "route": {
      "type": "control.switch",
      "config": {
        "expression": "{{ input.event_type }}",
        "cases": ["issue_opened", "issue_closed", "comment_added"]
      }
    },
    "handle_opened": {
      "type": "db.create",
      "services": { "database": "postgres" },
      "config": {
        "table": "issues",
        "data": {
          "external_id": "{{ input.payload.id }}",
          "title": "{{ input.payload.title }}",
          "status": "open"
        }
      }
    },
    "handle_closed": {
      "type": "db.update",
      "services": { "database": "postgres" },
      "config": {
        "table": "issues",
        "where": { "external_id": "{{ input.payload.id }}" },
        "data": { "status": "closed" }
      }
    },
    "handle_comment": {
      "type": "db.create",
      "services": { "database": "postgres" },
      "config": {
        "table": "comments",
        "data": {
          "issue_id": "{{ input.payload.issue_id }}",
          "body": "{{ input.payload.body }}"
        }
      }
    },
    "log_unknown": {
      "type": "util.log",
      "config": {
        "level": "warn",
        "message": "{{ 'Unknown webhook event: ' + input.event_type }}"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "status": "processed" }
      }
    }
  },
  "edges": [
    { "from": "route", "to": "handle_opened", "output": "issue_opened" },
    { "from": "route", "to": "handle_closed", "output": "issue_closed" },
    { "from": "route", "to": "handle_comment", "output": "comment_added" },
    { "from": "route", "to": "log_unknown", "output": "default" },
    { "from": "handle_opened", "to": "respond", "output": "success" },
    { "from": "handle_closed", "to": "respond", "output": "success" },
    { "from": "handle_comment", "to": "respond", "output": "success" },
    { "from": "log_unknown", "to": "respond", "output": "success" }
  ]
}
```

**Key point:** The `respond` node has four inbound edges from four mutually exclusive branches. The engine detects this as an OR-join -- only one branch fires, and `respond` executes as soon as that branch completes. No waiting for the other (unreachable) branches.

---

## Sub-Workflows

### Extracting Reusable Logic

Use `workflow.run` to call a shared workflow from multiple parents. Extract a sub-workflow when the same sequence of nodes appears in more than one workflow, or when a workflow grows beyond 8-10 nodes and a logical boundary exists.

```json
{
  "id": "create-order",
  "nodes": {
    "validate_inventory": {
      "type": "workflow.run",
      "as": "inventory_check",
      "config": {
        "workflow": "check-inventory",
        "input": {
          "items": "{{ input.items }}"
        }
      }
    },
    "create": {
      "type": "db.create",
      "services": { "database": "postgres" },
      "config": {
        "table": "orders",
        "data": {
          "user_id": "{{ input.user_id }}",
          "items": "{{ input.items }}",
          "total": "{{ nodes.inventory_check.total }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": "{{ nodes.create }}"
      }
    },
    "out_of_stock": {
      "type": "response.error",
      "config": {
        "status": 409,
        "code": "OUT_OF_STOCK",
        "message": "{{ nodes.inventory_check.message }}"
      }
    }
  },
  "edges": [
    { "from": "validate_inventory", "to": "create", "output": "success" },
    { "from": "validate_inventory", "to": "out_of_stock", "output": "error" },
    { "from": "create", "to": "respond", "output": "success" }
  ]
}
```

The sub-workflow `check-inventory` is a standalone workflow with its own nodes and edges. It uses `workflow.output` nodes to signal its result back to the parent. The output port name on `workflow.run` matches whichever `workflow.output` node fires inside the sub-workflow.

### Transaction Boundaries

Wrap a sub-workflow in a database transaction by setting `transaction: true`. All `db.*` nodes inside the sub-workflow share the same transaction. If any node fails, the entire transaction rolls back.

```json
{
  "id": "transfer-funds",
  "nodes": {
    "execute_transfer": {
      "type": "workflow.run",
      "as": "result",
      "services": { "database": "postgres" },
      "config": {
        "workflow": "transfer-funds-inner",
        "input": {
          "from_account": "{{ input.from_account }}",
          "to_account": "{{ input.to_account }}",
          "amount": "{{ input.amount }}"
        },
        "transaction": true
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.result }}"
      }
    },
    "error_response": {
      "type": "response.error",
      "config": {
        "status": 400,
        "code": "TRANSFER_FAILED",
        "message": "Transfer could not be completed"
      }
    }
  },
  "edges": [
    { "from": "execute_transfer", "to": "respond", "output": "success" },
    { "from": "execute_transfer", "to": "error_response", "output": "error" }
  ]
}
```

The inner workflow `transfer-funds-inner` performs the debit and credit operations. Because `transaction: true` is set, the engine wraps both operations in a single database transaction. If either the debit or credit fails, both are rolled back automatically.

---

## Performance

### Cache-Aside Pattern

Check the cache before hitting the database. On a miss, query the database and populate the cache for subsequent requests. This is the same as the Graceful Degradation pattern above, applied specifically for performance.

```json
{
  "id": "get-product",
  "nodes": {
    "cache_get": {
      "type": "cache.get",
      "as": "cached",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'product:' + input.product_id }}"
      }
    },
    "check_hit": {
      "type": "control.if",
      "config": {
        "condition": "{{ nodes.cached.value != nil }}"
      }
    },
    "respond_cached": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.cached.value }}"
      }
    },
    "db_fetch": {
      "type": "db.findOne",
      "as": "product",
      "services": { "database": "postgres" },
      "config": {
        "table": "products",
        "where": { "id": "{{ input.product_id }}" }
      }
    },
    "cache_set": {
      "type": "cache.set",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'product:' + input.product_id }}",
        "value": "{{ nodes.product }}",
        "ttl": 600
      }
    },
    "respond_fresh": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.product }}"
      }
    }
  },
  "edges": [
    { "from": "cache_get", "to": "check_hit", "output": "success" },
    { "from": "cache_get", "to": "db_fetch", "output": "error" },
    { "from": "check_hit", "to": "respond_cached", "output": "then" },
    { "from": "check_hit", "to": "db_fetch", "output": "else" },
    { "from": "db_fetch", "to": "cache_set", "output": "success" },
    { "from": "cache_set", "to": "respond_fresh", "output": "success" }
  ]
}
```

### Avoiding N+1 Queries

**Bad -- N+1 pattern:** Fetching a list, then querying for each item individually in a loop. This creates N+1 database round-trips.

```json
{
  "id": "list-orders-n-plus-1",
  "nodes": {
    "fetch_orders": {
      "type": "db.find",
      "as": "orders",
      "services": { "database": "postgres" },
      "config": {
        "table": "orders",
        "where": { "user_id": "{{ input.user_id }}" }
      }
    },
    "fetch_each_customer": {
      "type": "control.loop",
      "as": "enriched",
      "config": {
        "collection": "{{ nodes.orders }}",
        "workflow": "fetch-customer-for-order",
        "input": {
          "customer_id": "{{ $item.customer_id }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.enriched }}"
      }
    }
  },
  "edges": [
    { "from": "fetch_orders", "to": "fetch_each_customer", "output": "success" },
    { "from": "fetch_each_customer", "to": "respond", "output": "done" }
  ]
}
```

**Good -- single query with join:** Fetch all the data you need in one query.

```json
{
  "id": "list-orders-efficient",
  "nodes": {
    "fetch_orders": {
      "type": "db.query",
      "as": "orders",
      "services": { "database": "postgres" },
      "config": {
        "sql": "SELECT o.*, c.name as customer_name, c.email as customer_email FROM orders o JOIN customers c ON o.customer_id = c.id WHERE o.user_id = $1 ORDER BY o.created_at DESC",
        "params": ["{{ input.user_id }}"]
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "orders": "{{ nodes.orders }}" }
      }
    }
  },
  "edges": [
    { "from": "fetch_orders", "to": "respond", "output": "success" }
  ]
}
```

One query, one round-trip, regardless of how many orders exist.

### When to Parallelize

**Parallelize** when operations are independent -- they do not read each other's output. Examples:
- Counting total rows while fetching a page of results
- Loading a user profile and their notification preferences from different tables
- Making outbound HTTP calls to unrelated services

**Serialize** when one node needs another's output. Examples:
- Creating a record, then returning the created row's ID
- Validating input, then inserting into the database
- Fetching a user, then checking their permissions

The engine handles this automatically based on edges. Nodes with no inbound edges are entry nodes and start immediately. Nodes whose inbound edges all come from different branches (AND-join) wait for all branches. Nodes whose inbound edges come from mutually exclusive branches (OR-join, e.g., after `control.if`) fire on the first arrival.

---

## Anti-Patterns

| Anti-Pattern | Why It Is Bad | What to Do Instead |
|---|---|---|
| Missing error edges | Node fires `error` with no edge, workflow crashes with unhandled error | Add `error` edges to every node that can fail (db, cache, http) |
| N+1 queries in a loop | `control.loop` with `db.findOne` per item causes N+1 round-trips | Use a single `db.query` with a JOIN or IN clause |
| Oversized workflows | 20+ nodes in one workflow are hard to debug and maintain | Extract sub-workflows with `workflow.run` at logical boundaries |
| Retrying non-transient errors | Retrying validation or permission errors wastes time and always fails | Only use retry config for network/timeout/lock errors |
| Cache without TTL | `cache.set` with no `ttl` fills memory and serves stale data forever | Always set a `ttl` appropriate to the data's freshness needs |
| Sequential independent queries | Two queries that do not depend on each other chained with edges | Remove the edge between them so they run in parallel |
| Ignoring `default` in switch | `control.switch` without a `default` edge silently drops unmatched values | Always connect `default` to a log or error response node |
| Deep recursion | Nested `workflow.run` or `control.loop` calls exceeding depth 64 | Flatten logic or process in batches instead of recursive calls |
