package service

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/axmipic/axmipic/internal/imaging"
	"github.com/axmipic/axmipic/internal/storage"
)

// ErrProcessingFailed is returned when a transformation cannot be applied.
var ErrProcessingFailed = errors.New("service: image processing failed")

// ErrProcessingUnsupported is returned when processing is disabled.
var ErrProcessingUnsupported = errors.New("service: image processing is disabled")

// maxRenderSourceBytes bounds how much of a stored object is read to apply a
// transformation.
const maxRenderSourceBytes = 64 << 20

// TransformRequest is a raw, unvalidated transformation request.
type TransformRequest struct {
	Width   int
	Height  int
	Fit     string
	Quality int
	Format  string
	Enlarge bool
	Rotate  int
}

// Empty reports whether the request asks for no transformation.
func (r TransformRequest) Empty() bool {
	return r.Width == 0 && r.Height == 0 && r.Format == "" &&
		r.Quality == 0 && !r.Enlarge && r.Rotate == 0
}

// ProcessingPolicy constrains transformations.
type ProcessingPolicy struct {
	Enabled        bool
	MaxWidth       int
	MaxHeight      int
	DefaultQuality int
	AllowedFormats []imaging.Format
}

// ImagingService applies on-the-fly transformations to stored images.
type ImagingService struct {
	storage   storage.Storage
	processor imaging.Processor
	policy    ProcessingPolicy
	allowed   map[imaging.Format]struct{}
}

// NewImagingService constructs an ImagingService.
func NewImagingService(backend storage.Storage, processor imaging.Processor, policy ProcessingPolicy) *ImagingService {
	allowed := make(map[imaging.Format]struct{}, len(policy.AllowedFormats))
	for _, f := range policy.AllowedFormats {
		allowed[f] = struct{}{}
	}
	return &ImagingService{storage: backend, processor: processor, policy: policy, allowed: allowed}
}

// Enabled reports whether transformations are available.
func (s *ImagingService) Enabled() bool {
	return s.policy.Enabled && s.processor != nil
}

// Capabilities returns the active processor's capabilities.
func (s *ImagingService) Capabilities() imaging.Capabilities {
	if s.processor == nil {
		return imaging.Capabilities{}
	}
	return s.processor.Capabilities()
}

// Options validates req against the policy and processor and returns the
// processor options to use.
func (s *ImagingService) Options(req TransformRequest) (imaging.Options, error) {
	if req.Width < 0 || req.Height < 0 {
		return imaging.Options{}, fmt.Errorf("%w: dimensions must not be negative", ErrInvalidInput)
	}
	if req.Width > s.policy.MaxWidth || req.Height > s.policy.MaxHeight {
		return imaging.Options{}, fmt.Errorf("%w: requested size exceeds the maximum %dx%d", ErrInvalidInput, s.policy.MaxWidth, s.policy.MaxHeight)
	}
	fit, ok := imaging.ParseFit(req.Fit)
	if !ok {
		return imaging.Options{}, fmt.Errorf("%w: unknown fit %q", ErrInvalidInput, req.Fit)
	}
	quality := req.Quality
	if quality == 0 {
		quality = s.policy.DefaultQuality
	}
	if quality < 1 || quality > 100 {
		return imaging.Options{}, fmt.Errorf("%w: quality must be between 1 and 100", ErrInvalidInput)
	}
	switch req.Rotate {
	case 0, 90, 180, 270:
	default:
		return imaging.Options{}, fmt.Errorf("%w: rotate must be 90, 180, or 270", ErrInvalidInput)
	}

	var format imaging.Format
	if req.Format != "" {
		f, ok := imaging.ParseFormat(req.Format)
		if !ok {
			return imaging.Options{}, fmt.Errorf("%w: unknown format %q", ErrInvalidInput, req.Format)
		}
		if _, ok := s.allowed[f]; !ok {
			return imaging.Options{}, fmt.Errorf("%w: format %q is not allowed", ErrInvalidInput, f)
		}
		if !s.canEncode(f) {
			return imaging.Options{}, fmt.Errorf("%w: format %q is not supported by the %s processor", ErrInvalidInput, f, s.processor.Capabilities().Name)
		}
		format = f
	}

	return imaging.Options{
		Width:         req.Width,
		Height:        req.Height,
		Fit:           fit,
		Quality:       quality,
		Format:        format,
		StripMetadata: true,
		Enlarge:       req.Enlarge,
		Rotate:        req.Rotate,
	}, nil
}

func (s *ImagingService) canEncode(f imaging.Format) bool {
	for _, supported := range s.processor.Capabilities().OutputFormats {
		if supported == f {
			return true
		}
	}
	return false
}

// Render fetches the original object and applies opts.
func (s *ImagingService) Render(ctx context.Context, key string, opts imaging.Options) (*imaging.Result, error) {
	if !s.Enabled() {
		return nil, ErrProcessingUnsupported
	}
	object, err := s.storage.Get(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("render: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("render: fetch object: %w", err)
	}
	defer func() {
		_ = object.Close()
	}()
	src, err := io.ReadAll(io.LimitReader(object, maxRenderSourceBytes))
	if err != nil {
		return nil, fmt.Errorf("render: read object: %w", err)
	}

	result, err := s.processor.Process(src, opts)
	if err != nil {
		switch {
		case errors.Is(err, imaging.ErrUnsupportedFormat):
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		case errors.Is(err, imaging.ErrInvalidOptions):
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		default:
			return nil, fmt.Errorf("%w: %v", ErrProcessingFailed, err)
		}
	}
	return &result, nil
}
