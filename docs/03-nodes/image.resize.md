# image.resize

Resize an image to target dimensions.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `width` | number (expr) | yes | Target width |
| `height` | number (expr) | yes | Target height |
| `quality` | number | no | JPEG quality (1-100) |
| `format` | string | no | Output format: jpeg, png, webp |

## Outputs

`success`, `error`

Output: `{path, width, height, size}`

## Behavior

Reads from `source` storage at `input` path, writes to `destination` storage at `output` path. Maintains aspect ratio by default.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `source` | `storage` | Yes |
| `destination` | `storage` | Yes |

## Example

```json
{
  "type": "image.resize",
  "services": {
    "source": "uploads",
    "destination": "processed"
  },
  "config": {
    "input": "{{ input.image_path }}",
    "output": "{{ 'resized/' + input.image_id + '.webp' }}",
    "width": 800,
    "height": 600,
    "format": "webp"
  }
}
```
