package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/plugin"
)

// 内置通知插件的名称。builtin- 前缀由宿主保留，外置插件不能使用。
const (
	BuiltinSMSLog    = "builtin-sms-log"
	BuiltinSMSHTTP   = "builtin-sms-http"
	BuiltinEmailLog  = "builtin-email-log"
	BuiltinEmailSMTP = "builtin-email-smtp"
)

// builtinNotifyProvider 是一个内置通知 Provider：直接编译进宿主、进程内运行，
// 默认受信且不需要 WASM 沙箱。配置仍通过插件配置（AES-GCM 加密密文字段）注入。
type builtinNotifyProvider struct {
	desc    plugin.Descriptor
	factory func(map[string]string) (notify.Sender, error)

	mu     sync.RWMutex
	sender notify.Sender
}

func (p *builtinNotifyProvider) Descriptor() plugin.Descriptor { return p.desc }

// Configure 依据插件配置构造实际的发送渠道。
func (p *builtinNotifyProvider) Configure(_ context.Context, cfg map[string]string) error {
	sender, err := p.factory(cfg)
	if err != nil {
		p.mu.Lock()
		p.sender = nil
		p.mu.Unlock()
		return err
	}
	p.mu.Lock()
	p.sender = sender
	p.mu.Unlock()
	return nil
}

// Invoke 处理通知发送。内置渠道与 WASM 渠道共用 "send" 约定。
func (p *builtinNotifyProvider) Invoke(ctx context.Context, op string, input []byte) ([]byte, error) {
	if op != "send" {
		return nil, fmt.Errorf("plugin %q: unsupported op %q", p.desc.Name, op)
	}
	var msg smsPayload
	if err := json.Unmarshal(input, &msg); err != nil {
		return nil, fmt.Errorf("plugin %q: invalid message: %w", p.desc.Name, err)
	}
	p.mu.RLock()
	sender := p.sender
	p.mu.RUnlock()
	if sender == nil {
		return nil, fmt.Errorf("plugin %q: channel is not configured", p.desc.Name)
	}
	if err := sender.Send(ctx, notify.Message{
		To:       msg.To,
		Subject:  msg.Subject,
		Body:     msg.Body,
		Template: msg.Template,
		Params:   msg.Params,
		SignName: msg.SignName,
	}); err != nil {
		return nil, err
	}
	return []byte(`{"ok":true}`), nil
}

func (p *builtinNotifyProvider) Close(context.Context) error { return nil }

// RegisterBuiltinNotify 把内置的短信/邮件渠道注册到插件管理器。内置插件在
// 启动阶段注册，始终启用且常驻，不占用 WASM 运行时内存。
func (s *PluginService) RegisterBuiltinNotify(logger *slog.Logger) {
	if s.manager == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	for _, spec := range builtinNotifySpecs(logger) {
		s.manager.RegisterBuiltin(plugin.Manifest{
			Name:     spec.desc.Name,
			Version:  spec.desc.Version,
			Category: spec.desc.Category,
			Runtime:  plugin.RuntimeBuiltin,
			ABI:      plugin.ABI,
		}, &builtinNotifyProvider{desc: spec.desc, factory: spec.factory})
	}
}

type builtinNotifySpec struct {
	desc    plugin.Descriptor
	factory func(map[string]string) (notify.Sender, error)
}

// builtinNotifySpecs 返回内置通知插件的描述与构造器。
func builtinNotifySpecs(logger *slog.Logger) []builtinNotifySpec {
	return []builtinNotifySpec{
		{
			desc: plugin.Descriptor{
				Name:      BuiltinSMSLog,
				Category:  plugin.CategoryNotifySMS,
				Title:     "日志短信（内置）",
				Version:   "1.0.0",
				Origin:    plugin.OriginBuiltin,
				Sandboxed: false,
			},
			factory: func(map[string]string) (notify.Sender, error) {
				return notify.NewLogSender("sms", logger), nil
			},
		},
		{
			desc: plugin.Descriptor{
				Name:      BuiltinSMSHTTP,
				Category:  plugin.CategoryNotifySMS,
				Title:     "通用 HTTP 短信（内置）",
				Version:   "1.0.0",
				Origin:    plugin.OriginBuiltin,
				Sandboxed: false,
				Fields: []plugin.Field{
					{Key: "provider", Label: "服务商名称", Help: "仅用于日志与展示，例如 aliyun"},
					{Key: "endpoint", Label: "HTTP 网关地址", Required: true, Help: "短信服务商或自建网关的完整 URL"},
					{Key: "method", Label: "请求方法", Type: plugin.FieldSelect, Default: "POST", Options: []string{"POST", "GET"}},
				},
			},
			factory: func(cfg map[string]string) (notify.Sender, error) {
				return notify.NewHTTPSSender(notify.HTTPOptions{
					Name:     strings.TrimSpace(cfg["provider"]),
					Endpoint: strings.TrimSpace(cfg["endpoint"]),
					Method:   strings.TrimSpace(cfg["method"]),
				})
			},
		},
		{
			desc: plugin.Descriptor{
				Name:      BuiltinEmailLog,
				Category:  plugin.CategoryNotifyEmail,
				Title:     "日志邮件（内置）",
				Version:   "1.0.0",
				Origin:    plugin.OriginBuiltin,
				Sandboxed: false,
			},
			factory: func(map[string]string) (notify.Sender, error) {
				return notify.NewLogSender("email", logger), nil
			},
		},
		{
			desc: plugin.Descriptor{
				Name:      BuiltinEmailSMTP,
				Category:  plugin.CategoryNotifyEmail,
				Title:     "SMTP 邮件（内置）",
				Version:   "1.0.0",
				Origin:    plugin.OriginBuiltin,
				Sandboxed: false,
				Fields: []plugin.Field{
					{Key: "host", Label: "SMTP 服务器", Required: true, Help: "例如 smtp.example.com"},
					{Key: "port", Label: "端口", Type: plugin.FieldInt, Default: "587"},
					{Key: "username", Label: "用户名"},
					{Key: "password", Label: "密码 / 授权码", Type: plugin.FieldPassword, Secret: true},
					{Key: "from", Label: "发件人", Help: "留空时使用用户名"},
					{Key: "use_tls", Label: "隐式 TLS", Type: plugin.FieldBool, Help: "465 端口开启；587 端口使用 STARTTLS"},
				},
			},
			factory: func(cfg map[string]string) (notify.Sender, error) {
				port := 587
				if v := strings.TrimSpace(cfg["port"]); v != "" {
					n, err := strconv.Atoi(v)
					if err != nil || n < 1 || n > 65535 {
						return nil, fmt.Errorf("invalid smtp port %q", v)
					}
					port = n
				}
				return notify.NewSMTPSender(notify.SMTPOptions{
					Host:     strings.TrimSpace(cfg["host"]),
					Port:     port,
					Username: strings.TrimSpace(cfg["username"]),
					Password: cfg["password"],
					From:     strings.TrimSpace(cfg["from"]),
					UseTLS:   strings.EqualFold(strings.TrimSpace(cfg["use_tls"]), "true"),
				})
			},
		},
	}
}

// builtinLogPlugin 返回某类别使用的日志兜底插件名。
func builtinLogPlugin(category string) string {
	if category == plugin.CategoryNotifyEmail {
		return BuiltinEmailLog
	}
	return BuiltinSMSLog
}
