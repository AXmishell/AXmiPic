//go:build libvips && cgo

package imaging

import (
	"fmt"

	"github.com/h2non/bimg"
)

// 注意：仅在具备 libvips 库和 pkg-config、且使用 `-tags libvips` 时才编译此文件，
// 并且它依赖 bimg 模块：
//
//	go get github.com/h2non/bimg
//	go build -tags libvips ./cmd/axmipic
//
// 默认构建使用 purego.go 中的纯 Go 处理器。bimg 被有意设为非默认依赖，以保持
// 模块无 CGO。

// bimgProcessor 使用 bimg/libvips 实现 Processor。它额外编码 WebP 和 AVIF。
type bimgProcessor struct{}

// Default 返回 libvips 处理器。
func Default() Processor {
	return bimgProcessor{}
}

// Capabilities 报告 libvips 处理器的能力。
func (bimgProcessor) Capabilities() Capabilities {
	return Capabilities{
		Name:          "libvips",
		OutputFormats: []Format{FormatJPEG, FormatPNG, FormatGIF, FormatWebP, FormatAVIF},
		StripMetadata: true,
	}
}

// Info 解码图像头部。
func (bimgProcessor) Info(src []byte) (Info, error) {
	meta, err := bimg.Metadata(src)
	if err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	f, _ := ParseFormat(meta.Type)
	return Info{Width: meta.Size.Width, Height: meta.Size.Height, Format: f}, nil
}

// Process 使用 libvips 应用变换。
func (bimgProcessor) Process(src []byte, opts Options) (Result, error) {
	if err := validateOptions(opts); err != nil {
		return Result{}, err
	}
	options := bimg.Options{
		Width:         opts.Width,
		Height:        opts.Height,
		Crop:          opts.Fit == FitCover,
		Enlarge:       opts.Enlarge,
		Quality:       opts.Quality,
		StripMetadata: opts.StripMetadata,
		Force:         opts.Fit == FitFill,
	}
	if opts.Format != "" {
		options.Type = bimgType(opts.Format)
	}
	switch opts.Rotate {
	case 90:
		options.Rotate = bimg.D90
	case 180:
		options.Rotate = bimg.D180
	case 270:
		options.Rotate = bimg.D270
	}
	switch opts.Flip {
	case "h":
		options.Flip = true
	case "v":
		options.Flop = true
	case "hv":
		options.Flip = true
		options.Flop = true
	}
	if opts.Grayscale {
		options.Interpretation = bimg.InterpretationBW
	}
	if opts.Blur > 0 {
		options.GaussianBlur = bimg.GaussianBlur{Sigma: opts.Blur}
	}
	if opts.Sharpen > 0 {
		options.Sharpen = bimg.Sharpen{Sigma: opts.Sharpen}
	}
	if opts.Watermark != nil {
		options.Watermark = bimg.Watermark{
			Text:       opts.Watermark.Text,
			Opacity:    float64(effectiveOpacity(opts.Watermark.Opacity)) / 100,
			Font:       "sans " + bimgWatermarkFontSize(opts.Watermark.Size),
			Background: bimg.WatermarkBackground{Colour: bimgWatermarkColor(opts.Watermark.Color)},
		}
	}

	data, err := bimg.NewImage(src).Process(options)
	if err != nil {
		return Result{}, fmt.Errorf("imaging: libvips process: %w", err)
	}
	meta, err := bimg.Metadata(data)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	outFormat, _ := ParseFormat(meta.Type)
	if outFormat == "" {
		outFormat = opts.Format
	}
	return Result{
		Data:        data,
		ContentType: ContentType(outFormat),
		Width:       meta.Size.Width,
		Height:      meta.Size.Height,
		Format:      outFormat,
	}, nil
}

// effectiveOpacity 把水印不透明度收敛到默认的 80。
func effectiveOpacity(opacity int) int {
	if opacity <= 0 {
		return 80
	}
	if opacity > 100 {
		return 100
	}
	return opacity
}

// bimgWatermarkFontSize 返回 libvips 期望的字号字符串。
func bimgWatermarkFontSize(size int) string {
	if size <= 0 {
		return "48"
	}
	return itoa(size)
}

// bimgWatermarkColor 返回 libvips 期望的十六进制颜色。
func bimgWatermarkColor(value string) string {
	if value == "" {
		return "#ffffff"
	}
	if value[0] == '#' {
		return value
	}
	return "#" + value
}

// itoa 是 strconv.Itoa 的轻量替代，避免再引入一个 import。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func bimgType(f Format) bimg.ImageType {
	switch f {
	case FormatJPEG:
		return bimg.JPEG
	case FormatPNG:
		return bimg.PNG
	case FormatGIF:
		return bimg.GIF
	case FormatWebP:
		return bimg.WEBP
	case FormatAVIF:
		return bimg.AVIF
	default:
		return bimg.ImageType(0)
	}
}
