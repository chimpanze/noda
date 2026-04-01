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

### With data flow

After validating a player action, send the computed move data to the Wasm game engine.

```json
{
  "send_move": {
    "type": "wasm.send",
    "services": { "runtime": "game-server" },
    "config": {
      "data": {
        "action": "place_piece",
        "player_id": "{{ nodes.validate_turn.player_id }}",
        "x": "{{ nodes.validate_turn.position.x }}",
        "y": "{{ nodes.validate_turn.position.y }}"
      }
    }
  }
}
```

When `nodes.validate_turn` produced `{"player_id": "p1", "position": {"x": 3, "y": 7}, "valid": true}`, the command is sent to the module. Output stored as `nodes.send_move`:
```json
null
```

The node fires immediately without waiting for the module to process the command.
