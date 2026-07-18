# Cache Cookbook — set, get, exists, del

A demonstration of Noda's cache nodes (`cache.set`, `cache.get`, `cache.exists`, `cache.del`) with Redis.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/cache | Set a cache value |
| GET | /api/cache/:key | Get a cache value by key |
| GET | /api/cache/:key/exists | Check if a key exists in cache |
| DELETE | /api/cache/:key | Delete a cache value |

## Running

```bash
# Environment
export REDIS_URL='redis://localhost:6379/0'

# Validate config
noda validate --config examples/node-cookbook/cache

# Test the workflow suite
go test -tags=integration ./internal/testing/cookbook/ -run 'TestCookbook/cache' -v
```

## Quick Test

```bash
# Set a value
curl -X POST http://localhost:3000/api/cache \
  -H "Content-Type: application/json" \
  -d '{"key":"greeting","value":"hello"}'
# → {"stored":true}

# Get the value
curl http://localhost:3000/api/cache/greeting
# → {"value":"hello"}

# Check if key exists
curl http://localhost:3000/api/cache/greeting/exists
# → {"exists":true}

# Delete the value
curl -X DELETE http://localhost:3000/api/cache/greeting
# → 204 No Content

# Verify key no longer exists
curl http://localhost:3000/api/cache/greeting/exists
# → {"exists":false}

# Get after delete returns 404
curl http://localhost:3000/api/cache/greeting
# → {"code":"NOT_FOUND","message":"Cache key not found"} (404)
```

## Project Structure

```
noda.json           — main configuration (cache service with Redis)
routes/             — HTTP route definitions (set, get, exists, del)
workflows/          — workflow definitions (cache operations)
verify.json         — integration test suite
README.md           — this file
```

## Service Dependencies

All cache nodes require the `cache` service slot. This is configured in `noda.json` with:

```json
"services": {
  "app-cache": {
    "plugin": "cache",
    "config": { "url": "{{ $env('REDIS_URL') }}" }
  }
}
```

And referenced in workflows as:

```json
"services": { "cache": "app-cache" }
```

## Node Outputs

- **cache.set**: `{ok: true}`
- **cache.get**: `{value: <cached-value>}` on success; fires `error` port on cache miss
- **cache.exists**: `{exists: true|false}`
- **cache.del**: `{ok: true}` (always succeeds, even if key didn't exist)
