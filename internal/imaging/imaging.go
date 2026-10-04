// Package imaging 在可构建选择的处理器背后提供图像变换功能。默认构建使用纯 Go
// 实现；使用 `-tags libvips` 编译（需要 libvips 系统库和 CGO）则会改用更快的
// bimg/libvips 处理器。
package imaging

import (
	"errors"
	"fmt"
	"strings"
)

// 处理器返回的错误。
var (
	// ErrDecode 表示输入的字节无法被解码为图像。
	ErrDecode = errors.New("imaging: cannot decode image")
	// ErrUnsupportedFormat 表示处理器无法编码该格式。
	ErrUnsupportedFormat = errors.New("imaging: unsupported output format")
	// ErrInvalidOptions 表示请求的变换无效。
	ErrInvalidOptions = errors.New("imaging: invalid options")
)

// Format 是可编码的图像格式。
type Format string

// 支持的格式。
const (
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
	FormatGIF  Format = "gif"
	FormatWebP Format = "webp"
	FormatAVIF Format = "avif"
)

// Fit 是当请求的尺寸框与源图像宽高比不同时使用的策略。
type Fit string

// 支持的适配策略。
const (
	// FitContain 缩放图像使其适配到尺寸框内，并保持宽高比。
	FitContain Fit = "contain"
	// FitCover 缩放并裁剪图像，使其恰好填满尺寸框。
	FitCover Fit = "cover"
	// FitFill 拉伸图像以恰好填满尺寸框（不保持宽高比）。
	FitFill Fit = "fill"
)

// Watermark 描述一个文字水印。
type Watermark struct {
	Text string
	// Position 为水印位置：top-left、top-right、bottom-left、bottom-right、center。
	Position string
	// Opacity 为不透明度（0-100）。
	Opacity int
	// Size 为字号（像素）；为 0 时按图像尺寸自适应。
	Size int
	// Color 为十六进制颜色，例如 #ffffff。
	Color string
}

// Options 描述一次变换请求。
type Options struct {
	Width         int
	Height        int
	Fit           Fit
	Quality       int
	Format        Format
	StripMetadata bool
	Enlarge       bool
	Rotate        int
	// Flip 为翻转方式：h（水平）、v（垂直）、hv（同时）。
	Flip string
	// Grayscale 为 true 时转为灰度。
	Grayscale bool
	// Blur 为高斯模糊半径；0 表示不模糊。
	Blur float64
	// Sharpen 为锐化强度；0 表示不锐化。
	Sharpen float64
	// Watermark 非空时叠加文字水印。
	Watermark *Watermark
}

// Result 是处理后的图像。
type Result struct {
	Data        []byte
	ContentType string
	Width       int
	Height      int
	Format      Format
}

// Info 是无需完整解码即可获取的图像头部信息。
type Info struct {
	Width  int
	Height int
	Format Format
}

// Capabilities 描述处理器支持的能力。
type Capabilities struct {
	Name          string
	OutputFormats []Format
	StripMetadata bool
}

// Processor 变换并检查图像。
type Processor interface {
	// Process 根据 opts 重新编码 src。
	Process(src []byte, opts Options) (Result, error)
	// Info 仅解码图像头部。
	Info(src []byte) (Info, error)
	// Capabilities 报告支持的输出格式。
	Capabilities() Capabilities
}

// ParseFormat 将格式名称（包括 "jpg" 别名）映射为 Format。
func ParseFormat(name string) (Format, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "jpeg", "jpg":
		return FormatJPEG, true
	case "png":
		return FormatPNG, true
	case "gif":
		return FormatGIF, true
	case "webp":
		return FormatWebP, true
	case "avif":
		return FormatAVIF, true
	default:
		return "", false
	}
}

// ParseFit 将适配名称映射为 Fit。空名称默认为 FitContain。
func ParseFit(name string) (Fit, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "contain", "inside", "fit":
		return FitContain, true
	case "cover", "crop":
		return FitCover, true
	case "fill", "stretch":
		return FitFill, true
	default:
		return "", false
	}
}

// ParseFlip 将翻转名称映射为规范化的 Flip 值；空值返回空字符串。
func ParseFlip(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		return "", true
	case "h", "horizontal":
		return "h", true
	case "v", "vertical":
		return "v", true
	case "hv", "vh", "both":
		return "hv", true
	default:
		return "", false
	}
}

// ContentType 返回格式对应的 HTTP 媒体类型。
func ContentType(f Format) string {
	switch f {
	case FormatJPEG:
		return "image/jpeg"
	case FormatPNG:
		return "image/png"
	case FormatGIF:
		return "image/gif"
	case FormatWebP:
		return "image/webp"
	case FormatAVIF:
		return "image/avif"
	default:
		return "application/octet-stream"
	}
}

// validateOptions 检查所有处理器共享的选项不变式。
func validateOptions(opts Options) error {
	if opts.Width < 0 || opts.Height < 0 {
		return fmt.Errorf("%w: negative dimension", ErrInvalidOptions)
	}
	switch opts.Fit {
	case "", FitContain, FitCover, FitFill:
	default:
		return fmt.Errorf("%w: unknown fit %q", ErrInvalidOptions, opts.Fit)
	}
	if (opts.Fit == FitCover || opts.Fit == FitFill) && (opts.Width <= 0 || opts.Height <= 0) {
		return fmt.Errorf("%w: %s requires both width and height", ErrInvalidOptions, opts.Fit)
	}
	if opts.Quality < 1 || opts.Quality > 100 {
		return fmt.Errorf("%w: quality %d out of range", ErrInvalidOptions, opts.Quality)
	}
	switch opts.Rotate {
	case 0, 90, 180, 270:
	default:
		return fmt.Errorf("%w: rotate %d is not supported", ErrInvalidOptions, opts.Rotate)
	}
	switch opts.Flip {
	case "", "h", "v", "hv":
	default:
		return fmt.Errorf("%w: unknown flip %q", ErrInvalidOptions, opts.Flip)
	}
	if opts.Blur < 0 || opts.Blur > 100 {
		return fmt.Errorf("%w: blur radius out of range", ErrInvalidOptions)
	}
	if opts.Sharpen < 0 || opts.Sharpen > 100 {
		return fmt.Errorf("%w: sharpen amount out of range", ErrInvalidOptions)
	}
	if opts.Watermark != nil {
		if strings.TrimSpace(opts.Watermark.Text) == "" {
			return fmt.Errorf("%w: watermark text is empty", ErrInvalidOptions)
		}
		if len([]rune(opts.Watermark.Text)) > 200 {
			return fmt.Errorf("%w: watermark text is too long", ErrInvalidOptions)
		}
	}
	return nil
}
