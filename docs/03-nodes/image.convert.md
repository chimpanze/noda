# image.convert

Convert an image between formats.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `format` | string | yes | Target format: jpeg, png, webp, gif |
| `quality` | number | no | JPEG quality (1-100) |

## Outputs

`success`, `error`

## Behavior

Reads the image from `source` storage, converts it to the specified format, and writes the result to `destination` storage. Supported formats: `"jpeg"`, `"png"`, `"webp"`, `"avif"`.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `source` | `storage` | Yes |
| `destination` | `storage` | Yes |

## Example

```json
{
  "type": "image.convert",
  "services": {
    "source": "uploads",
    "destination": "processed"
  },
  "config": {
    "input": "{{ input.image_path }}",
    "output": "{{ 'converted/' + input.image_id + '.webp' }}",
    "format": "webp",
    "quality": 85
  }
}
```

### With data flow

An upload handler stores a PNG, then the convert node creates a WebP version for serving on the web.

```json
{
  "handle_upload": {
    "type": "upload.handle",
    "services": { "storage": "uploads" },
    "config": {
      "field": "photo",
      "path": "{{ 'originals/' + $uuid() + '.png' }}"
    }
  },
  "to_webp": {
    "type": "image.convert",
    "services": {
      "source": "uploads",
      "destination": "processed"
    },
    "config": {
      "input": "{{ nodes.handle_upload.path }}",
      "output": "{{ 'web/' + nodes.handle_upload.name + '.webp' }}",
      "format": "webp",
      "quality": 80
    }
  }
}
```

Output stored as `nodes.to_webp`:
```json
{ "path": "web/photo.webp", "width": 1920, "height": 1080, "size": 67800 }
```

Downstream nodes access the converted file via `nodes.to_webp.path`.
