# image.crop

Crop an image to specified dimensions.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `width` | number (expr) | yes | Crop width |
| `height` | number (expr) | yes | Crop height |
| `gravity` | string | no | Position: `center` (default), `north`, `south`, `east`, `west`, `smart` |
| `max_width` | integer | no | Override the default 10,000 px per-side output limit |
| `max_height` | integer | no | Override the default 10,000 px per-side output limit |
| `max_pixels` | integer | no | Override the default 40,000,000 px (width x height) output limit |

## Outputs

`success`, `error`

## Behavior

Reads the image from `source` storage, crops it to the specified dimensions from the given gravity position, and writes to the `target` storage. Gravity defaults to `"center"`; other recognized values are `"north"`, `"south"`, `"east"`, `"west"`, `"smart"`. Unrecognized gravity values silently fall back to center. Source images larger than 20 MiB or 50,000,000 px (width x height) are rejected with a validation error. Requested output dimensions above 10,000 px per side or 40,000,000 px total are rejected (override with `max_width`/`max_height`/`max_pixels`).

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `source` | `storage` | Yes |
| `target` | `storage` | Yes |

## Example

```json
{
  "type": "image.crop",
  "services": {
    "source": "uploads",
    "target": "processed"
  },
  "config": {
    "input": "{{ input.image_path }}",
    "output": "{{ 'cropped/' + input.image_id + '.jpg' }}",
    "width": 400,
    "height": 400,
    "gravity": "center"
  }
}
```

### With data flow

A profile avatar workflow reads the uploaded image path from a previous node, crops it to a square, then returns the cropped path.

```json
{
  "handle_upload": {
    "type": "upload.handle",
    "services": { "storage": "uploads" },
    "config": {
      "field": "avatar",
      "path": "{{ 'avatars/raw/' + auth.sub + '.' + input.ext }}"
    }
  },
  "crop_avatar": {
    "type": "image.crop",
    "services": {
      "source": "uploads",
      "target": "processed"
    },
    "config": {
      "input": "{{ nodes.handle_upload.path }}",
      "output": "{{ 'avatars/cropped/' + auth.sub + '.jpg' }}",
      "width": 400,
      "height": 400,
      "gravity": "center"
    }
  }
}
```

Output stored as `nodes.crop_avatar`:
```json
{ "path": "avatars/cropped/usr_42.jpg", "width": 400, "height": 400, "size": 28400 }
```

Downstream nodes access fields via `nodes.crop_avatar.path`.
