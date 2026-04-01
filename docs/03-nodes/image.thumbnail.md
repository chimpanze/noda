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
        "uploaded_by": "{{ auth.user_id }}"
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
