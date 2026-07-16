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
| `max_width` | integer | no | Override the default 10,000 px per-side output limit |
| `max_height` | integer | no | Override the default 10,000 px per-side output limit |
| `max_pixels` | integer | no | Override the default 40,000,000 px (width x height) output limit |

## Outputs

`success`, `error`

Output: `{path, width, height, size}`

## Behavior

Reads from `source` storage at `input` path, writes to the `target` storage at `output` path. Maintains aspect ratio by default. Source images larger than 20 MiB or 50,000,000 px (width x height) are rejected with a validation error. Requested output dimensions above 10,000 px per side or 40,000,000 px total are rejected (override with `max_width`/`max_height`/`max_pixels`).

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `source` | `storage` | Yes |
| `target` | `storage` | Yes |

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
