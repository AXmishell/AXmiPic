package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/store"
)

// ErrNotifyFailed 在通知发送失败时返回。
var ErrNotifyFailed = errors.New("service: notification delivery failed")

// NotifyService 通过已配置的短信与邮件渠道发送通知。渠道可在运行时热替换
// （例如管理员在后台修改 SMTP 设置后立即生效）。每次发送都会尽力记录一条
// 日志，供管理员在后台查看。
type NotifyService struct {
	mu    sync.RWMutex
	sms   notify.Sender
	email notify.Sender
	// repo 非空时记录发送日志。
	repo *store.Repository
}

// NewNotifyService 构造一个 NotifyService。任一渠道为空时回退到日志渠道。
func NewNotifyService(sms, email notify.Sender) *NotifyService {
	return &NotifyService{sms: sms, email: email}
}

// SetRepository 安装发送日志的持久化仓库；未安装时不记录日志。
func (s *NotifyService) SetRepository(repo *store.Repository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repo = repo
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
	err := sender.Send(ctx, notify.Message{To: to, Body: body})
	s.record(ctx, "sms", sender.Name(), to, "", body, err)
	if err != nil {
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
	err := sender.Send(ctx, notify.Message{To: to, Subject: subject, Body: body})
	s.record(ctx, "email", sender.Name(), to, subject, body, err)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotifyFailed, err)
	}
	return nil
}

// record 尽力记录一条发送日志；记录失败不影响发送结果。
func (s *NotifyService) record(ctx context.Context, channel, provider, to, subject, body string, sendErr error) {
	s.mu.RLock()
	repo := s.repo
	s.mu.RUnlock()
	if repo == nil {
		return
	}
	status := "sent"
	errText := ""
	if sendErr != nil {
		status = "failed"
		errText = sendErr.Error()
		if len(errText) > 500 {
			errText = errText[:500]
		}
	}
	entry := &store.NotifyLog{
		ID:        uuid.NewString(),
		Channel:   channel,
		Recipient: to,
		Subject:   subject,
		Body:      body,
		Status:    status,
		Error:     errText,
		Provider:  provider,
	}
	// 发送可能已取消请求上下文；日志写入仍应完成。
	logCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := repo.CreateNotifyLog(logCtx, entry); err != nil {
		slog.Default().Warn("failed to record notify log", slog.Any("error", err))
	}
}

// NotifyLogDTO 是一条通知日志的对外表示。
type NotifyLogDTO struct {
	ID        string    `json:"id"`
	Channel   string    `json:"channel"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	Provider  string    `json:"provider"`
	CreatedAt time.Time `json:"created_at"`
}

// NotifyLogList 是通知日志的分页集合。
type NotifyLogList struct {
	Items    []NotifyLogDTO `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// ListNotifyLogs 返回一页通知日志。channel 为空表示全部，否则为 email/sms。
func (s *NotifyService) ListNotifyLogs(ctx context.Context, channel string, page, pageSize int) (*NotifyLogList, error) {
	s.mu.RLock()
	repo := s.repo
	s.mu.RUnlock()
	if repo == nil {
		return &NotifyLogList{Items: []NotifyLogDTO{}, Page: 1, PageSize: pageSize}, nil
	}
	channel = strings.TrimSpace(channel)
	if channel != "" && channel != "email" && channel != "sms" {
		return nil, fmt.Errorf("%w: channel must be email or sms", ErrInvalidInput)
	}
	page, pageSize = normalizePagination(page, pageSize)
	logs, total, err := repo.ListNotifyLogs(ctx, channel, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]NotifyLogDTO, 0, len(logs))
	for i := range logs {
		items = append(items, NotifyLogDTO{
			ID:        logs[i].ID,
			Channel:   logs[i].Channel,
			To:        logs[i].Recipient,
			Subject:   logs[i].Subject,
			Body:      logs[i].Body,
			Status:    logs[i].Status,
			Error:     logs[i].Error,
			Provider:  logs[i].Provider,
			CreatedAt: logs[i].CreatedAt,
		})
	}
	return &NotifyLogList{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
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
