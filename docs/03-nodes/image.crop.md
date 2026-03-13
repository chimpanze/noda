# image.crop

Crop an image to specified dimensions.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `input` | string (expr) | yes | Source image path |
| `output` | string (expr) | yes | Output image path |
| `width` | number (expr) | yes | Crop width |
| `height` | number (expr) | yes | Crop height |
| `gravity` | string | no | Position: `center` (default), `top-left`, `top-right`, `bottom-left`, `bottom-right` |

## Outputs

`success`, `error`

## Behavior

Reads the image from `source` storage, crops it to the specified dimensions from the given gravity position, and writes to `destination` storage. Gravity defaults to `"center"`. Other options: `"north"`, `"south"`, `"east"`, `"west"`, `"smart"`.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `source` | `storage` | Yes |
| `destination` | `storage` | Yes |

## Example

```json
{
  "type": "image.crop",
  "services": {
    "source": "uploads",
    "destination": "processed"
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
