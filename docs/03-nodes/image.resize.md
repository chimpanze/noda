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
    "target": "processed"
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

### With data flow

An upload handler stores the original path, then a resize node reads it and a response returns the result.

```json
{
  "handle_upload": {
    "type": "upload.handle",
    "services": { "storage": "uploads" },
    "config": {
      "field": "image",
      "path": "{{ 'originals/' + $uuid() + '.' + input.ext }}"
    }
  },
  "resize": {
    "type": "image.resize",
    "services": {
      "source": "uploads",
      "target": "processed"
    },
    "config": {
      "input": "{{ nodes.handle_upload.path }}",
      "output": "{{ 'resized/' + nodes.handle_upload.name + '.webp' }}",
      "width": 800,
      "height": 600,
      "format": "webp"
    }
  }
}
```

Output stored as `nodes.resize`:
```json
{ "path": "resized/photo.webp", "width": 800, "height": 600, "size": 45200 }
```

Downstream nodes access fields via `nodes.resize.path` or `nodes.resize.size`.
