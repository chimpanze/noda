# Expressions

Noda uses `{{ }}` expression syntax powered by [expr-lang/expr](https://expr-lang.org/).

## Context Variables

All nodes have access to these variables in expressions:

| Variable | Description |
|----------|-------------|
| `input` | Data passed to the workflow from the trigger |
| `auth` | Auth data: `user_id`, `roles`, `claims` |
| `trigger` | Trigger metadata: `type`, `timestamp`, `trace_id` |
| `nodes.<id>` | Output data from a previously executed node |
| `secrets.<NAME>` | Secret value from configured providers (`.env` files by default) |
| `$item`, `$index` | Loop iteration variables (inside `control.loop`) |

## Built-in Functions

`len()`, `lower()`, `upper()`, `now()`, `$uuid()`, `$var()`, `toInt()`, `toFloat()`.

### Type Conversion Functions

| Function | Description |
|---|---|
| `toInt(value)` | Converts a string or float to an integer |
| `toFloat(value)` | Converts a string or integer to a float64 |

### Hashing Functions

| Function | Description |
|---|---|
| `sha256(string)` | Returns hex-encoded SHA-256 hash |
| `sha512(string)` | Returns hex-encoded SHA-512 hash |
| `md5(string)` | Returns hex-encoded MD5 hash |
| `hmac(data, key, algorithm)` | Returns hex-encoded HMAC. Algorithm: `"sha256"` or `"sha512"` |
| `bcrypt_hash(password)` | Returns a bcrypt hash string (default cost) |
| `bcrypt_verify(password, hash)` | Returns `true` if the password matches the hash, `false` otherwise |

```json
{
  "body": {
    "checksum": "{{ sha256(input.payload) }}",
    "signature": "{{ hmac(input.body, $var('WEBHOOK_SECRET'), 'sha256') }}",
    "password_hash": "{{ bcrypt_hash(input.password) }}",
    "valid": "{{ bcrypt_verify(input.password, nodes.lookup.password_hash) }}"
  }
}
```

## Expressions in Config Fields

Most config fields accept expressions:

```json
{
  "body": "{{ nodes.fetch[0] }}",
  "message": "Hello, {{ input.name }}!",
  "condition": "{{ auth.roles contains 'admin' }}"
}
```

Some fields are **static** (never expressions): `mode`, `cases`, `workflow`, `method`, `type`, `backoff`.

## `$var()` — Shared Variables

`$var()` works in two ways:

- **Config-time substitution** when it is the entire field value (`{{ $var('NAME') }}`) — resolved at config load time before the workflow runs
- **Runtime expression function** when used inside a larger expression (e.g., `{{ "prefix." + $var('TOPIC') }}`) — evaluated by the expression engine

Both resolve values from `vars.json`. See [Variables](../02-config/variables.md) for details.

## Limits

Expressions are always terminating — the expression language does not allow infinite loops. Additionally, each expression evaluation enforces a memory budget (default: 1M allocation units) that limits array, map, and range allocations. Expressions that exceed the budget return an error.

The memory budget can be configured in `noda.json`:

```json
{ "server": { "expression_memory_budget": 2000000 } }
```

See [noda.json](../02-config/noda-json.md) for all server settings.
