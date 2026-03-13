# util.uuid

Generates a UUID v4. No configuration required.

## Config

No configuration fields.

## Outputs

`success`, `error`

Output is the UUID string.

## Behavior

Generates a random UUID v4 string. Fires `success` with the UUID as output data.

## Example

```json
{
  "type": "util.uuid",
  "config": {}
}
```
