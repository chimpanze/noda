# image.thumbnail

Smart crop + resize to exact dimensions.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `width` | number (expr) | yes | Thumbnail width |
| `height` | number (expr) | yes | Thumbnail height |

## Outputs

`success`, `error`

## Behavior

Always crops to exact dimensions using smart crop. Reads from `source` storage, generates the thumbnail, and writes to `destination` storage.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `source` | `storage` | Yes |
| `destination` | `storage` | Yes |

## Example

```json
{
  "type": "image.thumbnail",
  "services": {
    "source": "uploads",
    "destination": "thumbnails"
  },
  "config": {
    "input": "{{ input.image_path }}",
    "output": "{{ 'thumbs/' + input.image_id + '.webp' }}",
    "width": 200,
    "height": 200
  }
}
```
