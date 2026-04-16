# Expression Cookbook

Practical recipes for Noda expressions. For the language basics and syntax rules, see [Expressions](expressions.md). For how data moves between nodes, see [Data Flow](data-flow.md).

## Complete Function Reference

### Noda Built-in Functions

These functions are registered by Noda and available in all `{{ }}` expressions.

| Function | Signature | Description | Example |
|----------|-----------|-------------|---------|
| `$uuid()` | `() string` | Generate a UUID v4 string | `{{ $uuid() }}` |
| `$var(key)` | `(string) string` | Look up a shared variable from `vars.json` | `{{ $var('API_URL') }}` |
| `now()` | `() time.Time` | Returns the current time | `{{ now() }}` |
| `lower(s)` | `(string) string` | Convert string to lowercase | `{{ lower(input.email) }}` |
| `upper(s)` | `(string) string` | Convert string to uppercase | `{{ upper(input.code) }}` |
| `toInt(v)` | `(any) int` | Convert value to integer (coerces strings, floats) | `{{ toInt(input.page) }}` |
| `toFloat(v)` | `(any) float64` | Convert value to float64 (coerces strings, ints) | `{{ toFloat(input.price) }}` |
| `sha256(s)` | `(string) string` | Hex-encoded SHA-256 hash | `{{ sha256(input.payload) }}` |
| `sha512(s)` | `(string) string` | Hex-encoded SHA-512 hash | `{{ sha512(input.payload) }}` |
| `md5(s)` | `(string) string` | Hex-encoded MD5 hash (non-security use) | `{{ md5(input.etag_source) }}` |
| `hmac(data, key, alg)` | `(string, string, string) string` | Hex-encoded HMAC (`"sha256"` or `"sha512"`) | `{{ hmac(input.body, $var('SECRET'), 'sha256') }}` |
| `bcrypt_hash(pw)` | `(string) string` | Bcrypt hash of the password (default cost) | `{{ bcrypt_hash(input.password) }}` |
| `bcrypt_verify(pw, hash)` | `(string, string) bool` | True if password matches the bcrypt hash | `{{ bcrypt_verify(input.password, nodes.user.password_hash) }}` |

### Expr-lang Built-in Functions and Operators

These come from the [expr-lang](https://expr-lang.org/) engine and are always available.

| Function / Operator | Description | Example |
|---------------------|-------------|---------|
| `len(v)` | Length of string, array, or map | `{{ len(nodes.fetch) }}` |
| `contains(haystack, needle)` | True if string/array contains value | `{{ input.roles contains 'admin' }}` |
| `startsWith(s, prefix)` | True if string starts with prefix | `{{ startsWith(input.path, '/api') }}` |
| `endsWith(s, suffix)` | True if string ends with suffix | `{{ endsWith(input.email, '@company.com') }}` |
| `trim(s)` | Remove leading/trailing whitespace | `{{ trim(input.name) }}` |
| `trimPrefix(s, prefix)` | Remove prefix from string | `{{ trimPrefix(input.path, '/api') }}` |
| `trimSuffix(s, suffix)` | Remove suffix from string | `{{ trimSuffix(input.file, '.json') }}` |
| `split(s, sep)` | Split string into array | `{{ split(input.tags, ',') }}` |
| `join(arr, sep)` | Join array into string | `{{ join(input.ids, ',') }}` |
| `replace(s, old, new)` | Replace all occurrences | `{{ replace(input.text, ' ', '-') }}` |
| `matches(s, regex)` | True if string matches regex | `{{ matches(input.email, '^[^@]+@[^@]+$') }}` |
| `indexOf(s, substr)` | Index of first occurrence (-1 if not found) | `{{ indexOf(input.path, '/') }}` |
| `int(v)` | Cast to int | `{{ int(input.count) }}` |
| `float(v)` | Cast to float | `{{ float(input.amount) }}` |
| `string(v)` | Cast to string | `{{ string(input.id) }}` |
| `map(arr, {pred})` | Map array using predicate | `{{ map(nodes.fetch, {.name}) }}` |
| `filter(arr, {pred})` | Filter array by predicate | `{{ filter(nodes.fetch, {.active}) }}` |
| `count(arr, {pred})` | Count matching elements | `{{ count(nodes.fetch, {.status == 'done'}) }}` |
| `all(arr, {pred})` | True if all match | `{{ all(nodes.items, {.qty > 0}) }}` |
| `any(arr, {pred})` | True if any match | `{{ any(nodes.items, {.flagged}) }}` |
| `none(arr, {pred})` | True if none match | `{{ none(nodes.items, {.deleted}) }}` |
| `find(arr, {pred})` | First matching element | `{{ find(nodes.users, {.id == input.target_id}) }}` |
| `findIndex(arr, {pred})` | Index of first match (-1 if none) | `{{ findIndex(nodes.items, {.id == input.id}) }}` |
| `groupBy(arr, {key})` | Group array elements by key | `{{ groupBy(nodes.orders, {.status}) }}` |
| `sortBy(arr, field)` | Sort array by field | `{{ sortBy(nodes.items, 'name') }}` |
| `reduce(arr, {acc + #}, init)` | Reduce array to single value | `{{ reduce(nodes.items, {#acc + #item.qty}, 0) }}` |
| `keys(m)` | Keys of a map | `{{ keys(input.metadata) }}` |
| `values(m)` | Values of a map | `{{ values(input.metadata) }}` |
| `not` / `!` | Logical NOT | `{{ not input.disabled }}` |
| `and` / `&&` | Logical AND | `{{ input.active and input.verified }}` |
| `or` / `\|\|` | Logical OR | `{{ input.role == 'admin' or input.role == 'super' }}` |
| `? :` (ternary) | Conditional value | `{{ input.count > 0 ? 'yes' : 'no' }}` |
| `??` | Nil coalescing | `{{ input.nickname ?? input.name }}` |
| `in` | Membership test | `{{ input.status in ['active', 'pending'] }}` |

## Context Variables Quick Reference

| Variable | Available | Description |
|----------|-----------|-------------|
| `input` | All nodes | Data passed to the workflow from the trigger's `input` mapping |
| `input.*` | All nodes | Individual input fields: `input.name`, `input.user_id`, etc. |
| `auth` | All nodes | Auth data from JWT middleware |
| `auth.user_id` | All nodes | The authenticated user's ID (`sub` claim) |
| `auth.roles` | All nodes | Array of user roles |
| `auth.claims` | All nodes | All JWT claims as a map |
| `trigger` | All nodes | Trigger metadata |
| `trigger.type` | All nodes | Trigger type (e.g., `"http"`, `"schedule"`, `"stream"`) |
| `trigger.timestamp` | All nodes | When the workflow was triggered |
| `trigger.trace_id` | All nodes | Unique trace ID for the execution |
| `nodes.<id>` | Downstream nodes | Output of a previously executed node |
| `nodes.<id>.*` | Downstream nodes | Individual fields from a node's output |
| `secrets.<NAME>` | All nodes | Secret value from providers (`.env` by default) |
| `$item` | `transform.map`, `transform.filter`, `control.loop` | Current element during iteration |
| `$index` | `transform.map`, `transform.filter`, `control.loop` | Zero-based index during iteration |

## Type Coercion and Null Handling

### Simple vs. Interpolated Expressions

When `{{ }}` is the **entire** field value, the result type is preserved (object, array, number, bool). When mixed with literal text, all expression results are converted to strings and concatenated.

```json
{
  "body": "{{ nodes.fetch }}",
  "message": "Found {{ len(nodes.fetch) }} items"
}
```

- `body` receives the actual array/object from `nodes.fetch` (type preserved).
- `message` receives a string like `"Found 5 items"` (string concatenation).

### Null and Nil Access

Accessing a field on `nil` returns `nil` rather than crashing. This means chained access like `nodes.lookup.email` is safe even if `lookup` returned nil -- the result will be `nil`.

Use the nil-coalescing operator `??` to provide defaults:

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "display_name": "{{ input.nickname ?? input.name ?? 'Anonymous' }}"
    }
  }
}
```

#### `??` vs `||` — use `??`, never `||`

Coming from JavaScript or Python, developers often reach for `||` as a fallback. **In Noda expressions, `||` is strictly a boolean operator** — `a || "default"` where `a` is a string produces a compile error (`mismatched types string and string`). Use `??` for every fallback that isn't genuinely boolean.

| Expression | Result |
|---|---|
| `nil ?? "x"` | `"x"` |
| `"" ?? "x"` | `""` — empty string is **not** nil |
| `false ?? "x"` | `false` — false is **not** nil |
| `0 ?? "x"` | `0` — zero is **not** nil |
| `nil \|\| true` | compile error when either side isn't bool |

If you need a truthy-style fallback (treat empty string / zero as missing), write it explicitly:

```
{{ input.page != "" ? input.page : 1 }}
```

Finally: accessing a top-level identifier that isn't in the expression env (e.g., `{{ env.FOO }}`) is a compile error or silently returns nil, depending on strict mode. `??` doesn't rescue missing names — it only rescues `nil` values of existing names.

### Coercion Functions

Use `toInt()` and `toFloat()` to convert query parameters and other string values to numbers:

```json
{
  "type": "db.find",
  "services": { "database": "postgres" },
  "config": {
    "table": "products",
    "where": { "category": "{{ input.category }}" },
    "limit": "{{ toInt(input.limit ?? '20') }}",
    "offset": "{{ toInt(input.offset ?? '0') }}"
  }
}
```

`toInt()` handles strings (`"42"`), floats (`42.9` becomes `42`), and integers (pass-through). `toFloat()` handles strings, ints, and floats.

### Truthiness

Falsy values: `nil`, `false`, `0`, `""` (empty string), empty arrays, empty maps.

Everything else is truthy. This matters for `control.if` conditions and `transform.filter` predicates.

## Recipes

### String Building

Concatenate strings with `+` or use interpolated expressions.

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "full_name": "{{ input.first_name + ' ' + input.last_name }}",
      "greeting": "Hello, {{ input.first_name }}!",
      "slug": "{{ lower(replace(input.title, ' ', '-')) }}",
      "initials": "{{ upper(input.first_name[0:1] + input.last_name[0:1]) }}",
      "api_url": "{{ $var('API_BASE') }}/users/{{ input.user_id }}"
    }
  }
}
```

### Conditional Values (Ternary)

Use the ternary operator for inline conditionals. Use `control.if` for branching to different nodes.

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "label": "{{ input.count == 1 ? 'item' : 'items' }}",
      "status": "{{ input.is_active ? 'active' : 'inactive' }}",
      "role": "{{ auth.roles contains 'admin' ? 'admin' : 'user' }}",
      "discount": "{{ input.total > 100 ? input.total * 0.1 : 0 }}"
    }
  }
}
```

Use nil-coalescing for default values:

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "page_size": "{{ input.limit ?? 25 }}",
      "sort_order": "{{ input.order ?? 'created_at DESC' }}"
    }
  }
}
```

### Working with Arrays

#### Filter, Map, and Reduce inside Expressions

The expr-lang `filter()`, `map()`, and `reduce()` builtins work inline. Use the `{predicate}` closure syntax where `.` or `#item` refers to the current element.

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "active_users": "{{ filter(nodes.fetch, {.status == 'active'}) }}",
      "names": "{{ map(nodes.fetch, {.name}) }}",
      "emails_upper": "{{ map(nodes.fetch, {upper(.email)}) }}",
      "total_qty": "{{ reduce(nodes.items, {#acc + #item.quantity}, 0) }}",
      "has_errors": "{{ any(nodes.results, {.error != nil}) }}",
      "admin_count": "{{ count(nodes.users, {.role == 'admin'}) }}"
    }
  }
}
```

#### Using transform.map and transform.filter Nodes

For complex transformations, use the dedicated nodes. Inside these nodes, `$item` and `$index` are the iteration variables.

```json
{
  "type": "transform.map",
  "config": {
    "collection": "{{ nodes.fetch }}",
    "expression": "{{ { 'id': $item.id, 'label': $item.first_name + ' ' + $item.last_name, 'position': $index + 1 } }}"
  }
}
```

```json
{
  "type": "transform.filter",
  "config": {
    "collection": "{{ nodes.fetch }}",
    "expression": "{{ $item.age >= 18 and $item.status == 'active' }}"
  }
}
```

#### Array Indexing

Access array elements by index. Arrays are zero-based.

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "first_result": "{{ nodes.fetch[0] }}",
      "first_name": "{{ nodes.fetch[0].name }}",
      "last_result": "{{ nodes.fetch[len(nodes.fetch) - 1] }}",
      "top_three": "{{ nodes.fetch[0:3] }}"
    }
  }
}
```

### Accessing Nested Data

Use dot notation for objects and bracket notation for arrays.

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "city": "{{ input.address.city }}",
      "first_tag": "{{ input.tags[0] }}",
      "primary_email": "{{ input.contacts[0].email }}",
      "setting": "{{ nodes.config.settings.notifications.email_enabled }}"
    }
  }
}
```

Use nil-coalescing for deeply nested optional fields:

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "timezone": "{{ input.preferences.timezone ?? 'UTC' }}",
      "avatar": "{{ input.profile.images.avatar ?? $var('DEFAULT_AVATAR_URL') }}"
    }
  }
}
```

### Pagination Math

Compute limit/offset from page-based input parameters.

```json
{
  "type": "db.find",
  "services": { "database": "postgres" },
  "config": {
    "table": "products",
    "where": { "category": "{{ input.category }}" },
    "order": "created_at DESC",
    "limit": "{{ toInt(input.page_size ?? '20') }}",
    "offset": "{{ (toInt(input.page ?? '1') - 1) * toInt(input.page_size ?? '20') }}"
  }
}
```

Return pagination metadata alongside results:

```json
{
  "type": "response.json",
  "config": {
    "status": 200,
    "body": {
      "data": "{{ nodes.fetch }}",
      "page": "{{ toInt(input.page ?? '1') }}",
      "page_size": "{{ toInt(input.page_size ?? '20') }}",
      "count": "{{ len(nodes.fetch) }}"
    }
  }
}
```

### Referencing Previous Nodes

Every executed node stores its output in `nodes.<id>`. Use the `"as"` alias for cleaner references.

```json
{
  "nodes": {
    "find_user": {
      "type": "db.findOne",
      "as": "user",
      "services": { "database": "postgres" },
      "config": {
        "table": "users",
        "where": { "id": "{{ input.user_id }}" }
      }
    },
    "find_orders": {
      "type": "db.find",
      "as": "orders",
      "services": { "database": "postgres" },
      "config": {
        "table": "orders",
        "where": { "user_id": "{{ nodes.user.id }}" },
        "order": "created_at DESC",
        "limit": 10
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "user": "{{ nodes.user }}",
          "recent_orders": "{{ nodes.orders }}",
          "order_count": "{{ len(nodes.orders) }}"
        }
      }
    }
  },
  "edges": [
    { "from": "find_user", "to": "find_orders", "output": "success" },
    { "from": "find_orders", "to": "respond", "output": "success" }
  ]
}
```

### Dates and Timestamps

`now()` returns the current time. Use it for created/updated timestamps.

```json
{
  "type": "transform.set",
  "config": {
    "fields": {
      "created_at": "{{ now() }}",
      "id": "{{ $uuid() }}"
    }
  }
}
```

### Hashing and Passwords (Bcrypt Register/Login Flow)

#### Registration: Hash the Password

```json
{
  "nodes": {
    "prepare": {
      "type": "transform.set",
      "config": {
        "fields": {
          "id": "{{ $uuid() }}",
          "email": "{{ lower(input.email) }}",
          "password_hash": "{{ bcrypt_hash(input.password) }}",
          "created_at": "{{ now() }}"
        }
      }
    },
    "create_user": {
      "type": "db.create",
      "services": { "database": "postgres" },
      "config": {
        "table": "users",
        "data": {
          "id": "{{ nodes.prepare.id }}",
          "email": "{{ nodes.prepare.email }}",
          "password_hash": "{{ nodes.prepare.password_hash }}",
          "created_at": "{{ nodes.prepare.created_at }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": {
          "id": "{{ nodes.create_user.id }}",
          "email": "{{ nodes.create_user.email }}"
        }
      }
    }
  },
  "edges": [
    { "from": "prepare", "to": "create_user", "output": "success" },
    { "from": "create_user", "to": "respond", "output": "success" }
  ]
}
```

#### Login: Verify the Password and Issue JWT

```json
{
  "nodes": {
    "find_user": {
      "type": "db.findOne",
      "as": "user",
      "services": { "database": "postgres" },
      "config": {
        "table": "users",
        "where": { "email": "{{ lower(input.email) }}" }
      }
    },
    "check_password": {
      "type": "control.if",
      "config": {
        "condition": "{{ nodes.user != nil and bcrypt_verify(input.password, nodes.user.password_hash) }}"
      }
    },
    "sign_token": {
      "type": "util.jwt_sign",
      "as": "token",
      "config": {
        "claims": {
          "sub": "{{ nodes.user.id }}",
          "email": "{{ nodes.user.email }}"
        },
        "secret": "{{ secrets.JWT_SECRET }}",
        "expiry": "24h"
      }
    },
    "success_response": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "token": "{{ nodes.token }}"
        }
      }
    },
    "error_response": {
      "type": "response.error",
      "config": {
        "status": 401,
        "message": "Invalid email or password"
      }
    }
  },
  "edges": [
    { "from": "find_user", "to": "check_password", "output": "success" },
    { "from": "check_password", "to": "sign_token", "output": "then" },
    { "from": "check_password", "to": "error_response", "output": "else" },
    { "from": "sign_token", "to": "success_response", "output": "success" }
  ]
}
```

### JWT Signing

Use `util.jwt_sign` with claims built from expressions. The `secret` field should reference a secret, not a hardcoded value.

```json
{
  "type": "util.jwt_sign",
  "config": {
    "claims": {
      "sub": "{{ nodes.user.id }}",
      "email": "{{ nodes.user.email }}",
      "roles": "{{ nodes.user.roles }}",
      "org_id": "{{ nodes.user.organization_id }}"
    },
    "secret": "{{ secrets.JWT_SECRET }}",
    "algorithm": "HS256",
    "expiry": "1h"
  }
}
```

The output is the signed token string, accessible as `nodes.<id>` (or via `"as"` alias).

### Environment-Specific Config ($var and secrets)

Use `$var()` for non-sensitive configuration from `vars.json`. Use `secrets.*` for sensitive values from `.env` or secret providers.

```json
{
  "type": "http.request",
  "config": {
    "method": "POST",
    "url": "{{ $var('PAYMENT_API_URL') }}/charges",
    "headers": {
      "Authorization": "Bearer {{ secrets.PAYMENT_API_KEY }}",
      "Content-Type": "application/json"
    },
    "body": {
      "amount": "{{ input.amount }}",
      "currency": "{{ $var('DEFAULT_CURRENCY') }}"
    }
  }
}
```

When `$var('NAME')` is the **entire** field value, it is resolved at config load time (static substitution). When used inside a larger expression, it is evaluated at runtime. See [Expressions](expressions.md) for details.

### Iterating with control.loop

`control.loop` runs a sub-workflow for each item. Use `$item` and `$index` in the `input` template.

```json
{
  "type": "control.loop",
  "config": {
    "collection": "{{ nodes.fetch }}",
    "workflow": "send-notification",
    "input": {
      "user_id": "{{ $item.id }}",
      "email": "{{ $item.email }}",
      "position": "{{ $index + 1 }}"
    }
  }
}
```

The loop fires `done` with an array of all sub-workflow results. Iterations run sequentially. Maximum recursion depth is 64.

## Common Mistakes

| Mistake | Problem | Fix |
|---------|---------|-----|
| `{{ input.name }}` in static fields | Fields like `mode`, `cases`, `workflow`, `method`, `type`, `backoff` are static and never evaluate expressions | Use literal values for static fields |
| `"limit": "{{ input.limit }}"` | Query params are strings; `limit` expects an integer | `"limit": "{{ toInt(input.limit) }}"` |
| `nodes.fetch.name` on an array | `db.find` returns an array, not an object | `nodes.fetch[0].name` or use `transform.map` |
| `{{ nodes.lookup.email }}` when lookup may be nil | Works but returns nil silently -- response shows `null` | `{{ nodes.lookup.email ?? 'unknown' }}` for a default |
| `"body": "User: {{ nodes.user }}"` | Interpolated expression stringifies the object with `%v` formatting | `"body": "{{ nodes.user }}"` to preserve the object type |
| `{{ $item.name }}` outside iteration | `$item` is only available inside `transform.map`, `transform.filter`, and `control.loop` | Use `nodes.<id>` to reference previous node output |
| `$var('KEY')` with missing key | Returns an error at runtime | Ensure all referenced keys exist in `vars.json` |
| `{{ auht.is_admin }}` (typo) | Returns nil silently (undefined variable) | Enable strict mode in `noda.json` to catch typos at compile time |
| Very large `map()` or `filter()` | Exceeds the expression memory budget | Increase `expression_memory_budget` in `noda.json` or pre-filter in the database query |
| `{{ bcrypt_hash(input.pw) }}` in a response body | Leaking the hash to the client | Only use `bcrypt_hash` in fields written to the database |
