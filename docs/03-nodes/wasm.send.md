# wasm.send

Sends a fire-and-forget command to a Wasm module.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `data` | any (expr) | yes | Command data |

## Outputs

`success`, `error`

## Behavior

Resolves `data`. If the module exports `command`, Noda calls it immediately between ticks. Otherwise, the data is buffered for the next tick's `commands` array. Fires `success` immediately -- the workflow does not wait for the module to process the data.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `runtime` | `wasm` | Yes |

## Example

```json
{
  "type": "wasm.send",
  "services": { "runtime": "game-server" },
  "config": {
    "data": {
      "action": "player_move",
      "player_id": "{{ auth.user_id }}",
      "position": "{{ input.position }}"
    }
  }
}
```
