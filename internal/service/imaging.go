package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/storage"
)

// renderCacheTTL 是派生图缓存条目的存活时间。键是随机且不可变的，因此缓存
// 本身不需要失效逻辑；TTL 只是为了避免长时间驻留过期内容。
const renderCacheTTL = 24 * time.Hour

// ErrProcessingFailed 在无法应用某种变换时返回。
var ErrProcessingFailed = errors.New("service: image processing failed")

// ErrProcessingUnsupported 在处理功能被禁用时返回。
var ErrProcessingUnsupported = errors.New("service: image processing is disabled")

// defaultMaxRenderSourceBytes 限定了为应用变换而读取的已存储对象的大小上限。
const defaultMaxRenderSourceBytes = 64 << 20

// defaultMaxDecodePixels 限定变换时可接受的解码像素数量上限，以防范解压缩
// 炸弹并约束瞬时内存。它可由配置覆盖。
const defaultMaxDecodePixels = 16_000_000

// renderMemoryUnit 是渲染内存预算的计量单位（1 MiB）。
const renderMemoryUnit = 1 << 20

// TransformRequest 是一个原始的、未经校验的变换请求。
type TransformRequest struct {
	Width   int
	Height  int
	Fit     string
	Quality int
	Format  string
	Enlarge bool
	Rotate  int
	// Flip 为翻转方式：h、v、hv。
	Flip string
	// Grayscale 为 true 时转为灰度。
	Grayscale bool
	// Blur 为高斯模糊半径。
	Blur float64
	// Sharpen 为锐化强度。
	Sharpen float64
	// WatermarkText 非空时叠加文字水印。
	WatermarkText string
	// WatermarkPosition 为水印位置。
	WatermarkPosition string
	// WatermarkOpacity 为水印不透明度（0-100）。
	WatermarkOpacity int
	// WatermarkSize 为水印字号。
	WatermarkSize int
	// WatermarkColor 为水印颜色（十六进制）。
	WatermarkColor string
}

// Empty 报告该请求是否不要求任何变换。
func (r TransformRequest) Empty() bool {
	return r.Width == 0 && r.Height == 0 && r.Format == "" &&
		r.Quality == 0 && !r.Enlarge && r.Rotate == 0 &&
		r.Flip == "" && !r.Grayscale && r.Blur == 0 && r.Sharpen == 0 &&
		r.WatermarkText == ""
}

// ProcessingPolicy 约束变换行为。
type ProcessingPolicy struct {
	Enabled        bool
	MaxWidth       int
	MaxHeight      int
	DefaultQuality int
	AllowedFormats []imaging.Format
	// AllowEnlarge 为 false 时禁止放大。
	AllowEnlarge bool
	// AllowEffects 为 false 时禁止滤镜（灰度/模糊/锐化）。
	AllowEffects bool
	// AllowWatermark 为 false 时禁止水印。
	AllowWatermark bool
	// WatermarkText 为强制水印文字；非空时始终叠加到输出。
	WatermarkText string
}

// ImagingService 对已存储的图片应用即时变换。
type ImagingService struct {
	manager   *storage.Manager
	processor imaging.Processor
	policy    ProcessingPolicy
	allowed   map[imaging.Format]struct{}

	// cache 缓存派生图，避免对同一 (key, opts) 重复下载与重编码。
	cache *renderCache
	// sem 限制同时进行的渲染数量，防止并发解码造成内存尖峰。
	sem chan struct{}
	// mem 按字节对并发渲染施加总内存预算，避免多个大图同时解码耗尽内存。
	mem *byteLimiter
	// maxSourceBytes 与 maxDecodePixels 约束单次渲染读取与解码的规模。
	maxSourceBytes  int64
	maxDecodePixels int64
	// policyMu 保护 policy/allowed，使其可在运行时热替换。
	policyMu sync.RWMutex
}

// NewImagingService 构造一个 ImagingService。
func NewImagingService(manager *storage.Manager, processor imaging.Processor, policy ProcessingPolicy) *ImagingService {
	allowed := make(map[imaging.Format]struct{}, len(policy.AllowedFormats))
	for _, f := range policy.AllowedFormats {
		allowed[f] = struct{}{}
	}
	return &ImagingService{
		manager:         manager,
		processor:       processor,
		policy:          policy,
		allowed:         allowed,
		maxSourceBytes:  defaultMaxRenderSourceBytes,
		maxDecodePixels: defaultMaxDecodePixels,
	}
}

// SetRenderCache 启用派生图缓存，容量以字节计（<=0 表示禁用）。
func (s *ImagingService) SetRenderCache(maxBytes int64) {
	s.cache = newRenderCache(maxBytes, renderCacheTTL)
}

// SetMaxConcurrency 限制同时进行的渲染数量；n<=0 表示不限制。
func (s *ImagingService) SetMaxConcurrency(n int) {
	if n > 0 {
		s.sem = make(chan struct{}, n)
	}
}

// SetRenderMemoryBudget 设置并发渲染的总内存预算（字节）。渲染按其预估内存
// 占用加权占用该预算；budgetBytes<=0 表示不限制。
func (s *ImagingService) SetRenderMemoryBudget(budgetBytes int64) {
	s.mem = newByteLimiter(budgetBytes, renderMemoryUnit)
}

// SetMaxDecodePixels 覆盖单张变换可解码的像素上限；n<=0 时保持内置默认值。
func (s *ImagingService) SetMaxDecodePixels(n int) {
	if n > 0 {
		s.maxDecodePixels = int64(n)
	}
}

// SetMaxRenderSourceBytes 覆盖为变换而读取的源对象大小上限；n<=0 时保持默认。
func (s *ImagingService) SetMaxRenderSourceBytes(n int64) {
	if n > 0 {
		s.maxSourceBytes = n
	}
}

// SetPolicy 在运行时替换处理策略（后台设置热更新），并重建允许格式集合。
func (s *ImagingService) SetPolicy(policy ProcessingPolicy) {
	allowed := make(map[imaging.Format]struct{}, len(policy.AllowedFormats))
	for _, f := range policy.AllowedFormats {
		allowed[f] = struct{}{}
	}
	s.policyMu.Lock()
	s.policy = policy
	s.allowed = allowed
	s.policyMu.Unlock()
}

// acquire 获取一个渲染槽位，尊重 ctx 取消。
func (s *ImagingService) acquire(ctx context.Context) error {
	if s.sem == nil {
		return nil
	}
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// release 释放一个渲染槽位。
func (s *ImagingService) release() {
	if s.sem != nil {
		<-s.sem
	}
}

// Enabled 报告变换功能是否可用。
func (s *ImagingService) Enabled() bool {
	s.policyMu.RLock()
	defer s.policyMu.RUnlock()
	return s.policy.Enabled && s.processor != nil
}

// Capabilities 返回当前处理器的能力。
func (s *ImagingService) Capabilities() imaging.Capabilities {
	if s.processor == nil {
		return imaging.Capabilities{}
	}
	return s.processor.Capabilities()
}

// Options 根据策略和处理器校验 req，并返回要使用的处理器选项。
func (s *ImagingService) Options(req TransformRequest) (imaging.Options, error) {
	s.policyMu.RLock()
	defer s.policyMu.RUnlock()
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
	if fit == imaging.FitCover && (req.Width <= 0 || req.Height <= 0) {
		return imaging.Options{}, fmt.Errorf("%w: cover requires both width and height", ErrInvalidInput)
	}
	if fit == imaging.FitFill && (req.Width <= 0 || req.Height <= 0) {
		return imaging.Options{}, fmt.Errorf("%w: fill requires both width and height", ErrInvalidInput)
	}
	enlarge := req.Enlarge
	if enlarge && !s.policy.AllowEnlarge {
		return imaging.Options{}, fmt.Errorf("%w: enlarging is disabled", ErrInvalidInput)
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
	flip, ok := imaging.ParseFlip(req.Flip)
	if !ok {
		return imaging.Options{}, fmt.Errorf("%w: unknown flip %q", ErrInvalidInput, req.Flip)
	}
	if (req.Grayscale || req.Blur > 0 || req.Sharpen > 0) && !s.policy.AllowEffects {
		return imaging.Options{}, fmt.Errorf("%w: image effects are disabled", ErrInvalidInput)
	}
	if req.Blur < 0 || req.Blur > 100 {
		return imaging.Options{}, fmt.Errorf("%w: blur must be between 0 and 100", ErrInvalidInput)
	}
	if req.Sharpen < 0 || req.Sharpen > 100 {
		return imaging.Options{}, fmt.Errorf("%w: sharpen must be between 0 and 100", ErrInvalidInput)
	}

	var watermark *imaging.Watermark
	if text := strings.TrimSpace(req.WatermarkText); text != "" {
		if !s.policy.AllowWatermark {
			return imaging.Options{}, fmt.Errorf("%w: watermark is disabled", ErrInvalidInput)
		}
		watermark = &imaging.Watermark{
			Text:     text,
			Position: req.WatermarkPosition,
			Opacity:  req.WatermarkOpacity,
			Size:     req.WatermarkSize,
			Color:    req.WatermarkColor,
		}
	} else if s.policy.WatermarkText != "" {
		// 策略强制水印时始终叠加。
		watermark = &imaging.Watermark{Text: s.policy.WatermarkText, Position: "bottom-right"}
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
		Enlarge:       enlarge,
		Rotate:        req.Rotate,
		Flip:          flip,
		Grayscale:     req.Grayscale,
		Blur:          req.Blur,
		Sharpen:       req.Sharpen,
		Watermark:     watermark,
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

// Render 获取原始对象并应用 opts。backend 指定对象所在的存储后端，由调用方
// 按图片记录解析后传入。命中派生图缓存时直接返回，不触碰存储与处理器。
func (s *ImagingService) Render(ctx context.Context, backend storage.Storage, key string, opts imaging.Options) (*imaging.Result, error) {
	if !s.Enabled() {
		return nil, ErrProcessingUnsupported
	}
	cacheKey := key + "|" + imaging.OptionsDigest(opts)
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached, nil
	}
	if err := s.acquire(ctx); err != nil {
		return nil, err
	}
	defer s.release()
	// 等待闸门期间可能已有并发请求完成同一变换，做一次双重检查。
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached, nil
	}

	object, err := backend.Get(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("render: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("render: fetch object: %w", err)
	}
	defer func() {
		_ = object.Close()
	}()
	src, err := io.ReadAll(io.LimitReader(object, s.maxSourceBytes))
	if err != nil {
		return nil, fmt.Errorf("render: read object: %w", err)
	}
	pixels := 0
	if info, infoErr := s.processor.Info(src); infoErr == nil && info.Width > 0 && info.Height > 0 {
		pixels = info.Width * info.Height
		if int64(pixels) > s.maxDecodePixels {
			return nil, fmt.Errorf("%w: source image is too large to process", ErrInvalidInput)
		}
	}
	// 按预估内存占用申请预算，避免多个大图同时解码耗尽进程内存。
	estimate := renderMemoryEstimate(int64(len(src)), pixels)
	if err := s.mem.Acquire(ctx, estimate); err != nil {
		return nil, err
	}
	defer s.mem.Release(estimate)

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
	s.cache.Put(cacheKey, &result)
	return &result, nil
}

// renderMemoryEstimate 估算一次渲染的瞬时内存占用：源字节 + 解码后的位图
// （按每像素 4 字节的 RGBA 计）。无法取得像素数时按源字节的 3 倍保守估计。
func renderMemoryEstimate(sourceBytes int64, pixels int) int64 {
	if pixels > 0 {
		return sourceBytes + int64(pixels)*4
	}
	return sourceBytes * 3
}
