# image.thumbnail

Smart crop + resize to exact dimensions.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `width` | number (expr) | yes | Thumbnail width |
| `height` | number (expr) | yes | Thumbnail height |
| `max_width` | integer | no | Override the default 10,000 px per-side output limit |
| `max_height` | integer | no | Override the default 10,000 px per-side output limit |
| `max_pixels` | integer | no | Override the default 40,000,000 px (width x height) output limit |

## Outputs

`success`, `error`

## Behavior

Always crops to exact dimensions using smart crop. Reads from `source` storage, generates the thumbnail, and writes to the `target` storage. Source images larger than 20 MiB or 50,000,000 px (width x height) are rejected with a validation error. Requested output dimensions above 10,000 px per side or 40,000,000 px total are rejected (override with `max_width`/`max_height`/`max_pixels`). Sources smaller than the requested dimensions are **not** enlarged — the output keeps the source size. The output is encoded in the **source image format** regardless of the `output` path's file extension; use `image.convert` to change formats.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `source` | `storage` | Yes |
| `target` | `storage` | Yes |

## Example

```json
{
  "type": "image.thumbnail",
  "services": {
    "source": "uploads",
    "target": "thumbnails"
  },
  "config": {
    "input": "{{ input.image_path }}",
    "output": "{{ 'thumbs/' + input.image_id + '.webp' }}",
    "width": 200,
    "height": 200
  }
}
```

### With data flow

After creating a database record for an uploaded image, the thumbnail node generates a preview from the stored path.

```json
{
  "save_record": {
    "type": "db.create",
    "services": { "database": "postgres" },
    "config": {
      "table": "images",
      "data": {
        "path": "{{ input.image_path }}",
        "uploaded_by": "{{ auth.sub }}"
      }
    }
  },
  "make_thumb": {
    "type": "image.thumbnail",
    "services": {
      "source": "uploads",
      "target": "thumbnails"
    },
    "config": {
      "input": "{{ nodes.save_record.path }}",
      "output": "{{ 'thumbs/' + nodes.save_record.id + '.webp' }}",
      "width": 200,
      "height": 200
    }
  }
}
```

Output stored as `nodes.make_thumb`:
```json
{ "path": "thumbs/42.webp", "width": 200, "height": 200, "size": 8400 }
```

Downstream nodes access the thumbnail via `nodes.make_thumb.path`.

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/image`](../../examples/node-cookbook/image/README.md) — its README documents the exact request/response pair the integration suite executes.
