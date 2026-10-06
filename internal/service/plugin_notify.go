package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AXmishell/axmipic/internal/notify"
)

// PluginSender 把通知渠道桥接到一个 WASM 插件：实现 notify.Sender，把消息转为
// 插件的中性载荷并调用其 "send" 操作。
type PluginSender struct {
	svc      *PluginService
	category string
	name     string
}

// NewPluginSender 构造一个插件通知渠道。
func NewPluginSender(svc *PluginService, category, name string) *PluginSender {
	return &PluginSender{svc: svc, category: category, name: name}
}

// Name 返回渠道标识（即插件名）。
func (s *PluginSender) Name() string { return s.name }

// Send 通过插件发送一条消息。
func (s *PluginSender) Send(ctx context.Context, msg notify.Message) error {
	payload, err := json.Marshal(smsPayload{
		To:       msg.To,
		Subject:  msg.Subject,
		Body:     msg.Body,
		Template: msg.Template,
		Params:   msg.Params,
		SignName: msg.SignName,
	})
	if err != nil {
		return fmt.Errorf("plugin %q: encode message: %w", s.name, err)
	}
	if _, err := s.svc.Invoke(ctx, s.category, s.name, "send", payload); err != nil {
		return fmt.Errorf("plugin %q: %w", s.name, err)
	}
	return nil
}
