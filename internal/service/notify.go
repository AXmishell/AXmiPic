package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/AXmishell/axmipic/internal/notify"
)

// ErrNotifyFailed 在通知发送失败时返回。
var ErrNotifyFailed = errors.New("service: notification delivery failed")

// NotifyService 通过已配置的短信与邮件渠道发送通知。渠道可在运行时热替换
// （例如管理员在后台修改 SMTP 设置后立即生效）。
type NotifyService struct {
	mu    sync.RWMutex
	sms   notify.Sender
	email notify.Sender
}

// NewNotifyService 构造一个 NotifyService。任一渠道为空时回退到日志渠道。
func NewNotifyService(sms, email notify.Sender) *NotifyService {
	return &NotifyService{sms: sms, email: email}
}

// SetSMS 替换短信渠道。
func (s *NotifyService) SetSMS(sender notify.Sender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sms = sender
}

// SetEmail 替换邮件渠道。
func (s *NotifyService) SetEmail(sender notify.Sender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.email = sender
}

// SendSMS 发送一条短信。
func (s *NotifyService) SendSMS(ctx context.Context, to, body string) error {
	s.mu.RLock()
	sender := s.sms
	s.mu.RUnlock()
	if sender == nil {
		return fmt.Errorf("%w: no sms channel configured", ErrNotifyFailed)
	}
	if strings.TrimSpace(to) == "" || strings.TrimSpace(body) == "" {
		return fmt.Errorf("%w: recipient and body are required", ErrInvalidInput)
	}
	if err := sender.Send(ctx, notify.Message{To: to, Body: body}); err != nil {
		return fmt.Errorf("%w: %v", ErrNotifyFailed, err)
	}
	return nil
}

// SendEmail 发送一封邮件。
func (s *NotifyService) SendEmail(ctx context.Context, to, subject, body string) error {
	s.mu.RLock()
	sender := s.email
	s.mu.RUnlock()
	if sender == nil {
		return fmt.Errorf("%w: no email channel configured", ErrNotifyFailed)
	}
	if strings.TrimSpace(to) == "" || strings.TrimSpace(body) == "" {
		return fmt.Errorf("%w: recipient and body are required", ErrInvalidInput)
	}
	if err := sender.Send(ctx, notify.Message{To: to, Subject: subject, Body: body}); err != nil {
		return fmt.Errorf("%w: %v", ErrNotifyFailed, err)
	}
	return nil
}

// Channels 返回已配置的渠道名称。
func (s *NotifyService) Channels() (sms, email string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.sms != nil {
		sms = s.sms.Name()
	}
	if s.email != nil {
		email = s.email.Name()
	}
	return sms, email
}
