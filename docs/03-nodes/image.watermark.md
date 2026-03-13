# image.watermark

Add a watermark to an image.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `watermark` | string (expr) | yes | Watermark image path |
| `opacity` | number | no | Opacity 0-1 (default: 1.0) |
| `position` | string | no | `center` (default), `top-left`, `top-right`, `bottom-left`, `bottom-right` |

## Outputs

`success`, `error`

## Behavior

Reads the source image and watermark image from `source` storage, composites the watermark onto the source at the specified position and opacity, and writes the result to `destination` storage.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `source` | `storage` | Yes |
| `destination` | `storage` | Yes |

## Example

```json
{
  "type": "image.watermark",
  "services": {
    "source": "uploads",
    "destination": "processed"
  },
  "config": {
    "input": "{{ input.image_path }}",
    "output": "{{ 'watermarked/' + input.image_id + '.jpg' }}",
    "watermark": "assets/logo.png",
    "opacity": 0.5,
    "position": "bottom-right"
  }
}
```
