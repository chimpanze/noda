# storage.delete

Deletes a file from storage.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string (expr) | yes | File path |

## Outputs

`success`, `error`

## Behavior

Deletes the file at `path` from the configured storage service. Fires `success` on completion.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `storage` | `storage` | Yes |
