# wasm.query

Sends a synchronous query to a Wasm module and awaits the response.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `data` | any (expr) | yes | Query data |
| `timeout` | string | no | Query timeout (default: `"5s"`) |

## Outputs

`success`, `error`

## Behavior

Resolves `data`. Calls the module's `query` export synchronously (serialized with respect to ticks). Waits for the response up to `timeout`. Fires `success` with the module's response data. Fires `error` with `TimeoutError` if the module doesn't respond in time.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `runtime` | `wasm` | Yes |

## Example

```json
{
  "type": "wasm.query",
  "services": { "runtime": "game-server" },
  "config": {
    "data": {
      "query": "get_state",
      "player_id": "{{ auth.user_id }}"
    },
    "timeout": "2s"
  }
}
```

### With data flow

Query the Wasm module for a leaderboard filtered by the room a player joined.

```json
{
  "get_leaderboard": {
    "type": "wasm.query",
    "services": { "runtime": "game-server" },
    "config": {
      "data": {
        "query": "leaderboard",
        "room_id": "{{ nodes.join_room.room_id }}",
        "limit": 10
      },
      "timeout": "3s"
    }
  }
}
```

When `nodes.join_room` produced `{"room_id": "room-42", "player_count": 8}`, the query is sent with that room ID. Output stored as `nodes.get_leaderboard`:
```json
{
  "entries": [
    { "player": "Alice", "score": 1500 },
    { "player": "Bob", "score": 1200 }
  ]
}
```
