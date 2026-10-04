//go:build !libvips || !cgo

package imaging

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// magickProcessor 通过调用 ImageMagick 命令行（magick 或 convert）实现 Processor。
// 它无需 CGO，适用于已安装 ImageMagick 的部署环境，并提供 libvips 之外的另一种
// 外部处理驱动。二进制缺失时该驱动不可用，Resolve 会回退到默认处理器。
type magickProcessor struct {
	binary string
}

// imagemagickBinary 返回可用的 ImageMagick 二进制路径（magick 优先）。
func imagemagickBinary() (string, error) {
	for _, name := range []string{"magick", "convert"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("imaging: ImageMagick (magick/convert) not found in PATH")
}

func init() {
	register(DriverMagick, func() (Processor, error) {
		binary, err := imagemagickBinary()
		if err != nil {
			return nil, err
		}
		return &magickProcessor{binary: binary}, nil
	})
}

// Capabilities 报告 ImageMagick 处理器的能力。
func (m *magickProcessor) Capabilities() Capabilities {
	return Capabilities{
		Name:          "imagemagick",
		OutputFormats: []Format{FormatJPEG, FormatPNG, FormatGIF, FormatWebP, FormatAVIF},
		StripMetadata: true,
	}
}

// Info 通过 ImageMagick identify 获取图像头部信息。
func (m *magickProcessor) Info(src []byte) (Info, error) {
	output, err := m.runWithInput(src, "identify", "-format", "%w %h %m", "-")
	if err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 3 {
		return Info{}, fmt.Errorf("%w: unexpected identify output", ErrDecode)
	}
	width, _ := strconv.Atoi(fields[0])
	height, _ := strconv.Atoi(fields[1])
	format, _ := ParseFormat(strings.ToLower(fields[2]))
	return Info{Width: width, Height: height, Format: format}, nil
}

// Process 通过 ImageMagick 应用变换。文字水印先由内置渲染器叠加，再交由
// ImageMagick 处理其余几何与滤镜。
func (m *magickProcessor) Process(src []byte, opts Options) (Result, error) {
	if err := validateOptions(opts); err != nil {
		return Result{}, err
	}

	// 水印使用与纯 Go 处理器一致的渲染逻辑，先烧录到源图。
	if opts.Watermark != nil {
		prepared, err := applyWatermarkBytes(src, *opts.Watermark)
		if err != nil {
			return Result{}, err
		}
		src = prepared
		// 水印已在字节层完成，避免命令行重复叠加。
		opts.Watermark = nil
	}

	args := buildMagickArgs(opts)
	args = append(args, "-")
	output, err := m.runWithInput(src, "convert", args...)
	if err != nil {
		return Result{}, fmt.Errorf("imaging: imagemagick process: %w", err)
	}
	data := []byte(output)

	format, err := detectOutputFormat(data, opts.Format)
	if err != nil {
		return Result{}, err
	}
	info, err := m.Info(data)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Data:        data,
		ContentType: ContentType(format),
		Width:       info.Width,
		Height:      info.Height,
		Format:      format,
	}, nil
}

// buildMagickArgs 将 Options 转换为 ImageMagick 命令行参数。
func buildMagickArgs(opts Options) []string {
	args := []string{}
	// 尺寸与适配。
	switch opts.Fit {
	case FitCover:
		args = append(args, "-resize", fmt.Sprintf("%dx%d^", opts.Width, opts.Height),
			"-gravity", "center", "-extent", fmt.Sprintf("%dx%d", opts.Width, opts.Height))
	case FitFill:
		args = append(args, "-resize", fmt.Sprintf("%dx%d!", opts.Width, opts.Height))
	default:
		if opts.Width > 0 || opts.Height > 0 {
			size := fmt.Sprintf("%dx%d", opts.Width, opts.Height)
			spec := size + ">"
			if !opts.Enlarge {
				spec = size + ">"
			}
			args = append(args, "-resize", strings.Replace(spec, "0x0>", "", 1))
		}
	}
	switch opts.Rotate {
	case 90:
		args = append(args, "-rotate", "90")
	case 180:
		args = append(args, "-rotate", "180")
	case 270:
		args = append(args, "-rotate", "270")
	}
	switch opts.Flip {
	case "h":
		args = append(args, "-flop")
	case "v":
		args = append(args, "-flip")
	case "hv":
		args = append(args, "-flip", "-flop")
	}
	if opts.Grayscale {
		args = append(args, "-colorspace", "Gray")
	}
	if opts.Blur > 0 {
		args = append(args, "-blur", fmt.Sprintf("0x%.1f", opts.Blur))
	}
	if opts.Sharpen > 0 {
		args = append(args, "-sharpen", fmt.Sprintf("0x%.1f", opts.Sharpen))
	}
	if opts.StripMetadata {
		args = append(args, "-strip")
	}
	if opts.Format != "" {
		args = append(args, magickFormatName(opts.Format))
	}
	if opts.Quality >= 1 && opts.Quality <= 100 {
		args = append(args, "-quality", strconv.Itoa(opts.Quality))
	}
	return args
}

// magickFormatName 返回 ImageMagick 期望的输出格式前缀（形如 "png:"）。
func magickFormatName(f Format) string {
	switch f {
	case FormatJPEG:
		return "jpeg:"
	case FormatPNG:
		return "png:"
	case FormatGIF:
		return "gif:"
	case FormatWebP:
		return "webp:"
	case FormatAVIF:
		return "avif:"
	default:
		return ""
	}
}

// detectOutputFormat 依据显式请求或输出字节的魔数确定格式。
func detectOutputFormat(data []byte, requested Format) (Format, error) {
	if requested != "" {
		return requested, nil
	}
	switch {
	case bytes.HasPrefix(data, []byte("\xFF\xD8\xFF")):
		return FormatJPEG, nil
	case bytes.HasPrefix(data, []byte("\x89PNG")):
		return FormatPNG, nil
	case bytes.HasPrefix(data, []byte("GIF8")):
		return FormatGIF, nil
	case len(data) > 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return FormatWebP, nil
	default:
		return FormatPNG, nil
	}
}

// runWithInput 执行 ImageMagick 命令，把 input 从 stdin 传入并返回 stdout。
// 当二进制是统一的 magick 时使用 "magick <subcommand>"，否则使用独立的
// convert / identify 可执行文件。
func (m *magickProcessor) runWithInput(input []byte, subcommand string, args ...string) (string, error) {
	binary := m.binary
	if filepath.Base(binary) == "magick" {
		args = append([]string{subcommand}, args...)
	} else {
		// 单独的 convert/identify：换成对应子命令的可执行文件。
		binary = strings.Replace(binary, "convert", subcommand, 1)
		if path, err := exec.LookPath(binary); err == nil {
			binary = path
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// applyWatermarkBytes 解码 src，叠加水印后重新编码为 PNG，供外部处理器继续处理。
func applyWatermarkBytes(src []byte, wm Watermark) ([]byte, error) {
	img, _, err := decodeImage(src)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	rgba := toRGBA(img)
	drawTextWatermark(rgba, wm)
	return encodePNG(rgba)
}
