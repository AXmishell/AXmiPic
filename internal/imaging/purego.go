//go:build !libvips || !cgo

package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/disintegration/imaging"
)

// pureGoProcessor implements Processor with the pure-Go disintegration/imaging
// library. It encodes JPEG, PNG, and GIF; it can decode WebP but cannot encode
// it (that requires the libvips build).
type pureGoProcessor struct{}

// Default returns the pure-Go processor.
func Default() Processor {
	return pureGoProcessor{}
}

// Capabilities reports the pure-Go processor's abilities.
func (pureGoProcessor) Capabilities() Capabilities {
	return Capabilities{
		Name:          "purego",
		OutputFormats: []Format{FormatJPEG, FormatPNG, FormatGIF},
		StripMetadata: true,
	}
}

// Info decodes the image header.
func (pureGoProcessor) Info(src []byte) (Info, error) {
	cfg, name, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	f, _ := ParseFormat(name)
	return Info{Width: cfg.Width, Height: cfg.Height, Format: f}, nil
}

// Process applies the transformation and re-encodes the image. Re-encoding
// with Go's standard encoders drops EXIF and other metadata.
func (pureGoProcessor) Process(src []byte, opts Options) (Result, error) {
	if err := validateOptions(opts); err != nil {
		return Result{}, err
	}
	img, srcFormatName, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	outFormat, err := resolveOutputFormat(srcFormatName, img, opts.Format)
	if err != nil {
		return Result{}, err
	}
	img = applyTransform(img, opts)
	data, err := encodeImage(img, outFormat, opts.Quality)
	if err != nil {
		return Result{}, err
	}
	bounds := img.Bounds()
	return Result{
		Data:        data,
		ContentType: ContentType(outFormat),
		Width:       bounds.Dx(),
		Height:      bounds.Dy(),
		Format:      outFormat,
	}, nil
}

// applyTransform rotates then resizes the image.
func applyTransform(img image.Image, opts Options) image.Image {
	switch opts.Rotate {
	case 90:
		img = imaging.Rotate90(img)
	case 180:
		img = imaging.Rotate180(img)
	case 270:
		img = imaging.Rotate270(img)
	}
	return applyFit(img, opts)
}

// applyFit scales (and for cover, crops) the image to the requested box.
func applyFit(img image.Image, opts Options) image.Image {
	w, h := opts.Width, opts.Height
	if w <= 0 && h <= 0 {
		return img
	}
	if opts.Fit == FitCover {
		return imaging.Fill(img, w, h, imaging.Center, imaging.Lanczos)
	}
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return img
	}
	if w > 0 && h > 0 {
		scale := math.Min(float64(w)/float64(srcW), float64(h)/float64(srcH))
		if !opts.Enlarge && scale > 1 {
			scale = 1
		}
		newW := int(math.Round(float64(srcW) * scale))
		newH := int(math.Round(float64(srcH) * scale))
		if newW < 1 {
			newW = 1
		}
		if newH < 1 {
			newH = 1
		}
		if newW == srcW && newH == srcH {
			return img
		}
		return imaging.Resize(img, newW, newH, imaging.Lanczos)
	}
	if w > 0 {
		if !opts.Enlarge && w >= srcW {
			return img
		}
		return imaging.Resize(img, w, 0, imaging.Lanczos)
	}
	if !opts.Enlarge && h >= srcH {
		return img
	}
	return imaging.Resize(img, 0, h, imaging.Lanczos)
}

// resolveOutputFormat picks the output format: an explicit request wins,
// otherwise the source format when encodable, otherwise JPEG or PNG based on
// transparency.
func resolveOutputFormat(srcFormatName string, img image.Image, requested Format) (Format, error) {
	if requested != "" {
		if !pureGoEncodable(requested) {
			return "", fmt.Errorf("%w: %s (rebuild with -tags libvips for WebP/AVIF)", ErrUnsupportedFormat, requested)
		}
		return requested, nil
	}
	if f, ok := ParseFormat(srcFormatName); ok && pureGoEncodable(f) {
		return f, nil
	}
	if isOpaque(img) {
		return FormatJPEG, nil
	}
	return FormatPNG, nil
}

func pureGoEncodable(f Format) bool {
	switch f {
	case FormatJPEG, FormatPNG, FormatGIF:
		return true
	default:
		return false
	}
}

func isOpaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return false
}

// encodeImage encodes the image to the target format.
func encodeImage(img image.Image, f Format, quality int) ([]byte, error) {
	var buf bytes.Buffer
	switch f {
	case FormatJPEG:
		if err := imaging.Encode(&buf, flatten(img), imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
			return nil, fmt.Errorf("imaging: encode jpeg: %w", err)
		}
	case FormatPNG:
		if err := imaging.Encode(&buf, img, imaging.PNG); err != nil {
			return nil, fmt.Errorf("imaging: encode png: %w", err)
		}
	case FormatGIF:
		if err := imaging.Encode(&buf, img, imaging.GIF); err != nil {
			return nil, fmt.Errorf("imaging: encode gif: %w", err)
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, f)
	}
	return buf.Bytes(), nil
}

// flatten composites the image onto a white background so JPEG encoding does
// not turn transparent pixels black.
func flatten(img image.Image) image.Image {
	bounds := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Over)
	return dst
}
