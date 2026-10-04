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
