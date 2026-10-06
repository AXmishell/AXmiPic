// Package notify 提供可插拔的通知渠道：短信与邮件。真实服务商（阿里云短信、
// SMTP 等）可在此接口下实现；未配置时回退到日志渠道，便于开发调试。
package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// ErrUnsupported 在渠道不支持某项操作时返回。
var ErrUnsupported = errors.New("notify: operation is not supported by this sender")

// Message 是一条待发送的通知。
type Message struct {
	// To 为接收方：短信为手机号，邮件为邮箱地址。
	To      string
	Subject string
	Body    string
	// Template 与 Params 供模板化渠道（如云厂商短信）使用；直接发送正文的
	// 渠道可忽略。
	Template string
	Params   map[string]string
	// SignName 为短信签名，供需要签名的渠道使用。
	SignName string
}

// Sender 是通知渠道的通用接口。
type Sender interface {
	// Name 返回渠道标识。
	Name() string
	// Send 发送一条消息。
	Send(ctx context.Context, msg Message) error
}

// LogSender 仅记录日志，不实际发送，用于未配置服务商时的兜底。
type LogSender struct {
	kind   string
	logger *slog.Logger
}

// NewLogSender 构造一个日志渠道。kind 为 "sms" 或 "email"。
func NewLogSender(kind string, logger *slog.Logger) *LogSender {
	return &LogSender{kind: kind, logger: logger}
}

// Name 返回渠道标识。
func (s *LogSender) Name() string { return "log" }

// Send 记录一条日志。
func (s *LogSender) Send(_ context.Context, msg Message) error {
	if s.logger == nil {
		return nil
	}
	s.logger.Info("notification captured (log sender)",
		slog.String("channel", s.kind),
		slog.String("to", msg.To),
		slog.String("subject", msg.Subject),
		slog.String("body", msg.Body),
	)
	return nil
}

// MockSender 记录发送内容，供测试断言使用。
type MockSender struct {
	kind string
	Sent []Message
}

// NewMockSender 构造一个模拟渠道。
func NewMockSender(kind string) *MockSender {
	return &MockSender{kind: kind}
}

// Name 返回渠道标识。
func (s *MockSender) Name() string { return "mock" }

// Send 记录消息。
func (s *MockSender) Send(_ context.Context, msg Message) error {
	s.Sent = append(s.Sent, msg)
	return nil
}

// ValidateMessage 校验一条消息的基本字段。
func ValidateMessage(msg Message) error {
	if strings.TrimSpace(msg.To) == "" {
		return fmt.Errorf("%w: recipient is required", ErrUnsupported)
	}
	return nil
}
