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

// pureGoProcessor 使用纯 Go 的 disintegration/imaging 库实现 Processor。它可编码
// JPEG、PNG 和 GIF；可解码 WebP 但无法编码 WebP（编码 WebP 需要 libvips 构建）。
type pureGoProcessor struct{}

// Default 返回纯 Go 处理器。
func Default() Processor {
	return pureGoProcessor{}
}

// Capabilities 报告纯 Go 处理器的能力。
func (pureGoProcessor) Capabilities() Capabilities {
	return Capabilities{
		Name:          "purego",
		OutputFormats: []Format{FormatJPEG, FormatPNG, FormatGIF},
		StripMetadata: true,
	}
}

// Info 解码图像头部。
func (pureGoProcessor) Info(src []byte) (Info, error) {
	cfg, name, err := image.DecodeConfig(bytes.NewReader(src))
	if err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	f, _ := ParseFormat(name)
	return Info{Width: cfg.Width, Height: cfg.Height, Format: f}, nil
}

// Process 应用变换并重新编码图像。使用 Go 标准编码器重新编码会丢弃 EXIF 及其他
// 元数据。
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

// applyTransform 先旋转图像，再调整其尺寸。
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

// applyFit 缩放图像（对于 cover 还会裁剪）以适配请求的尺寸框。
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

// resolveOutputFormat 选择输出格式：显式请求优先，否则在源格式可编码时使用源格式，
// 再否则根据透明度在 JPEG 和 PNG 之间选择。
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

// encodeImage 将图像编码为目标格式。
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

// flatten 将图像合成到白色背景上，使 JPEG 编码不会将透明像素变为黑色。
func flatten(img image.Image) image.Image {
	bounds := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Over)
	return dst
}
