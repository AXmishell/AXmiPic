package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AXmishell/axmipic/internal/notify"
)

// ErrNotifyFailed 在通知发送失败时返回。
var ErrNotifyFailed = errors.New("service: notification delivery failed")

// NotifyService 通过已配置的短信与邮件渠道发送通知。
type NotifyService struct {
	sms   notify.Sender
	email notify.Sender
}

// NewNotifyService 构造一个 NotifyService。任一渠道为空时回退到日志渠道。
func NewNotifyService(sms, email notify.Sender) *NotifyService {
	return &NotifyService{sms: sms, email: email}
}

// SendSMS 发送一条短信。
func (s *NotifyService) SendSMS(ctx context.Context, to, body string) error {
	if s.sms == nil {
		return fmt.Errorf("%w: no sms channel configured", ErrNotifyFailed)
	}
	if strings.TrimSpace(to) == "" || strings.TrimSpace(body) == "" {
		return fmt.Errorf("%w: recipient and body are required", ErrInvalidInput)
	}
	if err := s.sms.Send(ctx, notify.Message{To: to, Body: body}); err != nil {
		return fmt.Errorf("%w: %v", ErrNotifyFailed, err)
	}
	return nil
}

// SendEmail 发送一封邮件。
func (s *NotifyService) SendEmail(ctx context.Context, to, subject, body string) error {
	if s.email == nil {
		return fmt.Errorf("%w: no email channel configured", ErrNotifyFailed)
	}
	if strings.TrimSpace(to) == "" || strings.TrimSpace(body) == "" {
		return fmt.Errorf("%w: recipient and body are required", ErrInvalidInput)
	}
	if err := s.email.Send(ctx, notify.Message{To: to, Subject: subject, Body: body}); err != nil {
		return fmt.Errorf("%w: %v", ErrNotifyFailed, err)
	}
	return nil
}

// Channels 返回已配置的渠道名称。
func (s *NotifyService) Channels() (sms, email string) {
	if s.sms != nil {
		sms = s.sms.Name()
	}
	if s.email != nil {
		email = s.email.Name()
	}
	return sms, email
}
