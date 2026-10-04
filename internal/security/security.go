// Package security 提供图片安全与云处理的可插拔抽象：上传前的内容审核/病毒
// 扫描，以及把图片变换委托给外部云服务。
package security

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// 扫描判定结果。
const (
	// VerdictSafe 表示内容安全。
	VerdictSafe = "safe"
	// VerdictBlocked 表示内容被判定为违规，应拒绝上传。
	VerdictBlocked = "blocked"
	// VerdictReview 表示需要人工复核（当前按放行处理但记录日志）。
	VerdictReview = "review"
)

// ScanResult 是一次内容扫描的结果。
type ScanResult struct {
	Verdict string
	// Reason 为判定原因或命中的标签。
	Reason string
	// Score 为违规置信度（0-1），部分服务商提供。
	Score float64
}

// Scanner 对上传内容进行安全扫描。
type Scanner interface {
	Name() string
	// Scan 检查图片字节。返回 blocked 时调用方应拒绝上传。
	Scan(ctx context.Context, data []byte, mimeType string) (ScanResult, error)
}

// AllowAllScanner 不做任何检查，始终放行；用于未配置扫描服务时。
type AllowAllScanner struct{}

// Name 返回扫描器标识。
func (AllowAllScanner) Name() string { return "none" }

// Scan 始终放行。
func (AllowAllScanner) Scan(context.Context, []byte, string) (ScanResult, error) {
	return ScanResult{Verdict: VerdictSafe}, nil
}

// BlockingScanner 按扩展名/魔数拒绝明显可执行或非图片的内容。
type BlockingScanner struct {
	allowedMIME map[string]struct{}
}

// NewBlockingScanner 构造一个基于白名单的本地扫描器。
func NewBlockingScanner(allowed []string) *BlockingScanner {
	set := make(map[string]struct{}, len(allowed))
	for _, m := range allowed {
		set[strings.TrimSpace(m)] = struct{}{}
	}
	return &BlockingScanner{allowedMIME: set}
}

// Name 返回扫描器标识。
func (BlockingScanner) Name() string { return "builtin" }

// Scan 校验声明的 MIME 是否在白名单内，并检测常见危险魔数。
func (s *BlockingScanner) Scan(_ context.Context, data []byte, mimeType string) (ScanResult, error) {
	if len(data) == 0 {
		return ScanResult{Verdict: VerdictBlocked, Reason: "empty content"}, nil
	}
	if mimeType != "" {
		if _, ok := s.allowedMIME[mimeType]; !ok {
			return ScanResult{Verdict: VerdictBlocked, Reason: "unsupported content type"}, nil
		}
	}
	if isDangerous(data) {
		return ScanResult{Verdict: VerdictBlocked, Reason: "content looks executable"}, nil
	}
	return ScanResult{Verdict: VerdictSafe}, nil
}

// isDangerous 检查常见可执行文件/脚本的魔数。
func isDangerous(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	prefixes := [][]byte{
		{0x4D, 0x5A},             // MZ (Windows PE)
		{0x7F, 0x45, 0x4C, 0x46}, // ELF
		{0xCE, 0xFA, 0xED, 0xFE}, // Mach-O
		{0xCF, 0xFA, 0xED, 0xFE},
		{0x23, 0x21}, // shebang
	}
	for _, p := range prefixes {
		if len(data) >= len(p) && bytesEqual(data[:len(p)], p) {
			return true
		}
	}
	return false
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// CloudResult 是云端处理返回的图片。
type CloudResult struct {
	Data        []byte
	ContentType string
}

// CloudProcessor 把图片变换委托给外部云服务（例如 CDN 图片处理或云函数）。
type CloudProcessor interface {
	Name() string
	// Available 报告云处理是否可用。
	Available() bool
	// Process 处理图片。params 为原始处理参数。
	Process(ctx context.Context, data []byte, params map[string]string) (*CloudResult, error)
}

// LoggingCloudProcessor 是一个占位云处理器：报告不可用，使调用方回退到本地
// 处理器。用于未配置云服务时。
type LoggingCloudProcessor struct {
	logger *slog.Logger
}

// NewLoggingCloudProcessor 构造占位云处理器。
func NewLoggingCloudProcessor(logger *slog.Logger) *LoggingCloudProcessor {
	return &LoggingCloudProcessor{logger: logger}
}

// Name 返回标识。
func (LoggingCloudProcessor) Name() string { return "local" }

// Available 返回 false，表示应回退到本地处理。
func (LoggingCloudProcessor) Available() bool { return false }

// Process 返回不可用错误。
func (LoggingCloudProcessor) Process(context.Context, []byte, map[string]string) (*CloudResult, error) {
	return nil, errors.New("security: cloud processing is not configured")
}

// IsBlocked 报告扫描结果是否应拒绝上传。
func (r ScanResult) IsBlocked() bool { return r.Verdict == VerdictBlocked }

// String 返回便于日志的可读描述。
func (r ScanResult) String() string {
	return fmt.Sprintf("%s(%s, score=%.2f)", r.Verdict, r.Reason, r.Score)
}
