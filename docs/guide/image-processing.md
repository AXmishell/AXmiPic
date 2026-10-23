# Image Processing

Images are accessed via `/i/<storage-key>`, and processing parameters can be appended to the query string:

```
http://localhost:8080/i/2026/10/05/6f1c…a2.png?w=400&h=300&fit=cover&f=webp&q=80
```

| Parameter | Meaning | Values |
|-----------|---------|--------|
| `w` | Target width | pixels |
| `h` | Target height | pixels |
| `fit` | Resize strategy | `contain` (default, scale proportionally) / `cover` (crop to fill) / `fill` (stretch to fill; both `cover`/`fill` require `w` and `h`) |
| `q` | Output quality | 1–100, defaults to `processing.default_quality` |
| `f` | Output format | `jpeg`/`png`/`gif`/`webp`/`avif` (limited by `processing.allowed_formats` and processor capabilities) |
| `r` | Rotation angle | `90`/`180`/`270` |
| `flip` | Flip | `h` (horizontal) / `v` (vertical) / `hv` (both) |
| `gray` | Convert to grayscale | `1`/`true` |
| `blur` | Gaussian blur radius | 0–100 |
| `sharpen` | Sharpening strength | 0–100 |
| `wm` | Text watermark content | any text (up to 200 characters) |
| `wm_pos` | Watermark position | `top-left`/`top-right`/`bottom-left`/`bottom-right`/`center` |
| `wm_opacity` | Watermark opacity | 0–100 (default 80) |
| `wm_size` | Watermark font size | pixels, 0 means automatic |
| `wm_color` | Watermark color | hexadecimal, e.g. `#ffffff` |
| `enlarge` | Whether to allow upscaling | `1`/`true`/`yes`/`on` |

Responses with processing parameters include an `ETag` and support `If-None-Match` to return `304`.

The `processing` configuration provides the switches and defaults:

```yaml
processing:
  enabled: true
  max_width: 4096
  max_height: 4096
  default_quality: 82
  allowed_formats: ["jpeg", "png", "gif", "webp", "avif"]
  allow_enlarge: false      # whether upscaling is allowed
  allow_effects: true       # whether grayscale/blur/sharpen are allowed
  allow_watermark: true     # whether watermarks can be added via URL
  watermark_text: ""        # when non-empty, this watermark is forcibly applied to all transformations
```

The frontend "Image Processing" page (`/processing`) offers live previews of the parameters above and generates copyable processing links.

## Using the libvips processor (optional)

The pure Go processor is used by default (supports JPEG/PNG/GIF output). For WebP/AVIF output and higher performance, install libvips and build with:

```bash
go build -tags libvips -o bin/axmipic ./cmd/axmipic
```

## Image processing drivers

`processing.driver` selects the processor used at runtime; unknown or unavailable drivers fall back to `purego` at startup:

| Driver | Description | Requirements |
|--------|-------------|--------------|
| `purego` | Pure Go (default), supports JPEG/PNG/GIF output | none |
| `libvips` | libvips/bimg, additionally supports WebP/AVIF with higher performance | `-tags libvips` + CGO + libvips |
| `magick` | ImageMagick command line, additionally supports WebP/AVIF | `magick` or `convert` installed on the system |

`GET /api/v1/admin/imaging/drivers` returns the drivers available in the current binary and the processor in use.
