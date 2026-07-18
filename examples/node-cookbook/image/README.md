# Cookbook: image nodes

Runnable examples for `image.resize`, `image.crop`, `image.convert`, `image.thumbnail`, and
`image.watermark` against a local-filesystem storage service.
Every request/response below is verified in CI by [`verify.json`](verify.json).

## Run

This project needs a writable directory — CI's cookbook walker always exports
`COOKBOOK_DATA_DIR` (an isolated temp dir) before running the suite. To run it
yourself:

```bash
export COOKBOOK_DATA_DIR=/tmp/noda-image-cookbook
go run ./cmd/noda start --config examples/node-cookbook/image
```

## image.resize — `GET /api/resize`

Resizes the image to target dimensions, maintaining aspect ratio by default.

```bash
curl localhost:3000/api/resize
# → 200 {"path":"out/resized.png","width":200,"height":150,"size":12345}
```

## image.crop — `GET /api/crop`

Crops the image to specified dimensions from a gravity position (center, north, south, east, west, smart).

```bash
curl localhost:3000/api/crop
# → 200 {"path":"out/cropped.png","width":100,"height":100,"size":5678}
```

## image.convert — `GET /api/convert`

Converts the image to a target format (jpeg, png, webp, avif, tiff, gif). Supported formats: `"jpeg"`/`"jpg"`, `"png"`, `"webp"`, `"avif"`, `"tiff"`, `"gif"`.

```bash
curl localhost:3000/api/convert
# → 200 {"path":"out/converted.jpg","width":400,"height":300,"size":45000}
```

## image.thumbnail — `GET /api/thumbnail`

Generates a thumbnail using smart crop to exact dimensions. Outputs keep the source encoding unless `format` says otherwise (see [`docs/03-nodes/image.thumbnail.md`](../../docs/03-nodes/image.thumbnail.md)).

```bash
curl localhost:3000/api/thumbnail
# → 200 {"path":"out/thumb","width":120,"height":120,"size":3200}
```

## image.watermark — `GET /api/watermark`

Adds a watermark image to the source at a specified position (center, top-left, top-right, bottom-left, bottom-right) with optional opacity (0-1).

```bash
curl localhost:3000/api/watermark
# → 200 {"path":"out/marked.png","width":400,"height":300,"size":56000}
```
