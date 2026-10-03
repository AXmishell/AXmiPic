//go:build libvips && cgo

package imaging

import (
	"fmt"

	"github.com/h2non/bimg"
)

// NOTE: this file is only compiled with `-tags libvips` on a system with the
// libvips library and pkg-config available, and it requires the bimg module:
//
//	go get github.com/h2non/bimg
//	go build -tags libvips ./cmd/axmipic
//
// The default build uses the pure-Go processor in purego.go. bimg is
// deliberately not a default dependency so the module stays CGO-free.

// bimgProcessor implements Processor with bimg/libvips. It additionally encodes
// WebP and AVIF.
type bimgProcessor struct{}

// Default returns the libvips processor.
func Default() Processor {
	return bimgProcessor{}
}

// Capabilities reports the libvips processor's abilities.
func (bimgProcessor) Capabilities() Capabilities {
	return Capabilities{
		Name:          "libvips",
		OutputFormats: []Format{FormatJPEG, FormatPNG, FormatGIF, FormatWebP, FormatAVIF},
		StripMetadata: true,
	}
}

// Info decodes the image header.
func (bimgProcessor) Info(src []byte) (Info, error) {
	meta, err := bimg.Metadata(src)
	if err != nil {
		return Info{}, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	f, _ := ParseFormat(meta.Type)
	return Info{Width: meta.Size.Width, Height: meta.Size.Height, Format: f}, nil
}

// Process applies the transformation with libvips.
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
