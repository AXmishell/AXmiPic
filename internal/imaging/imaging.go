// Package imaging provides image transformation behind a build-selectable
// processor. The default build uses a pure-Go implementation; compiling with
// `-tags libvips` (which requires the libvips system library and CGO) uses the
// faster bimg/libvips processor instead.
package imaging

import (
	"errors"
	"fmt"
	"strings"
)

// Errors returned by processors.
var (
	// ErrDecode indicates the input bytes could not be decoded as an image.
	ErrDecode = errors.New("imaging: cannot decode image")
	// ErrUnsupportedFormat indicates the processor cannot encode the format.
	ErrUnsupportedFormat = errors.New("imaging: unsupported output format")
	// ErrInvalidOptions indicates the requested transformation is not valid.
	ErrInvalidOptions = errors.New("imaging: invalid options")
)

// Format is an encodable image format.
type Format string

// Supported formats.
const (
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
	FormatGIF  Format = "gif"
	FormatWebP Format = "webp"
	FormatAVIF Format = "avif"
)

// Fit is the strategy used when the requested box differs from the source
// aspect ratio.
type Fit string

// Supported fit strategies.
const (
	// FitContain scales the image to fit inside the box, preserving aspect.
	FitContain Fit = "contain"
	// FitCover scales and crops the image to exactly fill the box.
	FitCover Fit = "cover"
)

// Options describes a single transformation request.
type Options struct {
	Width         int
	Height        int
	Fit           Fit
	Quality       int
	Format        Format
	StripMetadata bool
	Enlarge       bool
	Rotate        int
}

// Result is a processed image.
type Result struct {
	Data        []byte
	ContentType string
	Width       int
	Height      int
	Format      Format
}

// Info is image header information obtained without a full decode.
type Info struct {
	Width  int
	Height int
	Format Format
}

// Capabilities describes what a processor supports.
type Capabilities struct {
	Name          string
	OutputFormats []Format
	StripMetadata bool
}

// Processor transforms and inspects images.
type Processor interface {
	// Process re-encodes src according to opts.
	Process(src []byte, opts Options) (Result, error)
	// Info decodes only the image header.
	Info(src []byte) (Info, error)
	// Capabilities reports supported output formats.
	Capabilities() Capabilities
}

// ParseFormat maps a format name (including the "jpg" alias) to a Format.
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

// ParseFit maps a fit name to a Fit. An empty name defaults to FitContain.
func ParseFit(name string) (Fit, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "contain", "inside", "fit":
		return FitContain, true
	case "cover", "crop":
		return FitCover, true
	default:
		return "", false
	}
}

// ContentType returns the HTTP media type for a format.
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

// validateOptions checks option invariants shared by all processors.
func validateOptions(opts Options) error {
	if opts.Width < 0 || opts.Height < 0 {
		return fmt.Errorf("%w: negative dimension", ErrInvalidOptions)
	}
	switch opts.Fit {
	case "", FitContain, FitCover:
	default:
		return fmt.Errorf("%w: unknown fit %q", ErrInvalidOptions, opts.Fit)
	}
	if opts.Fit == FitCover && (opts.Width <= 0 || opts.Height <= 0) {
		return fmt.Errorf("%w: cover requires both width and height", ErrInvalidOptions)
	}
	if opts.Quality < 1 || opts.Quality > 100 {
		return fmt.Errorf("%w: quality %d out of range", ErrInvalidOptions, opts.Quality)
	}
	switch opts.Rotate {
	case 0, 90, 180, 270:
	default:
		return fmt.Errorf("%w: rotate %d is not supported", ErrInvalidOptions, opts.Rotate)
	}
	return nil
}
