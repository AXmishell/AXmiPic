package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/plugin"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/store"
)

// pluginConfigPrefix 是插件配置在 settings 表中的键前缀。
const pluginConfigPrefix = "plugin."

// pluginApplyTimeout 是配置插件（可能涉及其内部初始化）的超时。
const pluginApplyTimeout = 15 * time.Second

// PluginService 管理 WASM 插件的配置（含密文字段）并将其接入通知渠道。
type PluginService struct {
	manager *plugin.Manager
	repo    *store.Repository
	cipher  *secret.Cipher
	logger  *slog.Logger

	// applyMu 串行化 Apply，避免并发 Configure 与 Invoke 竞争同一插件实例。
	applyMu sync.Mutex
}

// NewPluginService 构造插件服务。manager 与 cipher 可为 nil（此时相关操作报错）。
func NewPluginService(manager *plugin.Manager, repo *store.Repository, cipher *secret.Cipher, logger *slog.Logger) *PluginService {
	if logger == nil {
		logger = slog.Default()
	}
	return &PluginService{manager: manager, repo: repo, cipher: cipher, logger: logger}
}

// Manager 返回底层插件管理器（可能为 nil）。
func (s *PluginService) Manager() *plugin.Manager { return s.manager }

// Descriptors 返回指定类别的插件自描述；category 为空返回全部。
func (s *PluginService) Descriptors(category string) []plugin.Descriptor {
	if s.manager == nil {
		return nil
	}
	return s.manager.List(category)
}

// PluginReadiness 描述一个插件作为渠道是否就绪。
type PluginReadiness struct {
	// Installed 为插件是否已安装。
	Installed bool `json:"installed"`
	// Enabled 为是否已启用（state != disabled）。
	Enabled bool `json:"enabled"`
	// Configured 为是否已保存过配置。
	Configured bool `json:"configured"`
	// State 为三态：disabled / standby / active。
	State string `json:"state"`
}

// Readiness 返回指定插件作为渠道的就绪状态。未安装时 Installed 为 false。
func (s *PluginService) Readiness(ctx context.Context, name string) PluginReadiness {
	if s.manager == nil {
		return PluginReadiness{}
	}
	for _, info := range s.manager.Installed() {
		if info.Manifest.Name != name {
			continue
		}
		configured := false
		if stored, err := s.loadRaw(ctx, name); err == nil {
			configured = len(stored) > 0
		}
		return PluginReadiness{
			Installed:  true,
			Enabled:    info.Enabled,
			Configured: configured,
			State:      pluginState(info.Enabled, info.Loaded),
		}
	}
	return PluginReadiness{}
}

// PluginConfigDTO 是插件配置的对外表示：秘钥字段只返回是否已设置，绝不回传明文。
type PluginConfigDTO struct {
	plugin.Descriptor
	// Values 为非秘钥字段的当前值（含默认值）。
	Values map[string]string `json:"values"`
	// Secrets 为各秘钥字段是否已有值。
	Secrets map[string]bool `json:"secrets"`
	// Configured 表示是否已有落库配置。
	Configured bool `json:"configured"`
}

// descriptor 按名称查找插件自描述（含已暂停插件，来自缓存或清单）。
func (s *PluginService) descriptor(name string) (plugin.Descriptor, bool) {
	if s.manager == nil {
		return plugin.Descriptor{}, false
	}
	return s.manager.Describe(name)
}

// Config 返回插件配置的对外表示。
func (s *PluginService) Config(ctx context.Context, name string) (*PluginConfigDTO, error) {
	desc, ok := s.descriptor(name)
	if !ok {
		return nil, fmt.Errorf("%w: plugin %q", plugin.ErrNotFound, name)
	}
	stored, err := s.loadRaw(ctx, name)
	if err != nil {
		return nil, err
	}
	dto := &PluginConfigDTO{
		Descriptor: desc,
		Values:     make(map[string]string, len(desc.Fields)),
		Secrets:    make(map[string]bool),
		Configured: len(stored) > 0,
	}
	for _, f := range desc.Fields {
		v := stored[f.Key]
		if f.Secret {
			dto.Secrets[f.Key] = v != ""
			continue
		}
		if v == "" {
			v = f.Default
		}
		dto.Values[f.Key] = v
	}
	return dto, nil
}

// SaveConfig 校验并持久化插件配置。秘钥字段为空表示保持原值；非秘钥字段缺省时
// 也保持原值。秘钥以 AES-256-GCM 密文存储，AAD 绑定到插件名与字段名。
func (s *PluginService) SaveConfig(ctx context.Context, name string, values map[string]string) error {
	desc, ok := s.descriptor(name)
	if !ok {
		return fmt.Errorf("%w: plugin %q", plugin.ErrNotFound, name)
	}
	prev, err := s.loadRaw(ctx, name)
	if err != nil {
		return err
	}
	out := make(map[string]string, len(desc.Fields))
	for _, f := range desc.Fields {
		v, present := values[f.Key]
		if f.Secret {
			switch {
			case v != "":
				if s.cipher == nil {
					return fmt.Errorf("%w: encryption is not configured", ErrSettingsConfig)
				}
				enc, encErr := s.cipher.EncryptWithAAD(v, s.aad(name, f.Key))
				if encErr != nil {
					return fmt.Errorf("plugin %q: encrypt %s: %w", name, f.Key, encErr)
				}
				out[f.Key] = enc
			case prev[f.Key] != "":
				out[f.Key] = prev[f.Key]
			}
			continue
		}
		if present {
			out[f.Key] = strings.TrimSpace(v)
			continue
		}
		if old := prev[f.Key]; old != "" {
			out[f.Key] = old
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("plugin %q: encode config: %w", name, err)
	}
	return s.repo.SetSetting(ctx, pluginConfigPrefix+name, string(encoded))
}

// resolve 返回解密后的完整配置（含秘钥明文），仅供内部注入插件使用。
func (s *PluginService) resolve(ctx context.Context, name string) (map[string]string, error) {
	desc, ok := s.descriptor(name)
	if !ok {
		return nil, fmt.Errorf("%w: plugin %q", plugin.ErrNotFound, name)
	}
	stored, err := s.loadRaw(ctx, name)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(desc.Fields))
	for _, f := range desc.Fields {
		v := stored[f.Key]
		if f.Secret {
			if v == "" {
				out[f.Key] = ""
				continue
			}
			if s.cipher == nil {
				return nil, fmt.Errorf("%w: encryption is not configured", ErrSettingsConfig)
			}
			dec, decErr := s.cipher.DecryptWithAAD(v, s.aad(name, f.Key))
			if decErr != nil {
				return nil, fmt.Errorf("plugin %q: decrypt %s: %w", name, f.Key, decErr)
			}
			out[f.Key] = dec
			continue
		}
		if v == "" {
			v = f.Default
		}
		out[f.Key] = v
	}
	return out, nil
}

// Apply 用已保存的配置（重新）配置插件实例。若插件处于待激活（standby）状态，
// 它**不会**触发加载：配置将在首次激活时自动应用。已暂停的插件返回错误。
func (s *PluginService) Apply(ctx context.Context, name string) error {
	if s.manager == nil {
		return fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	if !s.manager.Enabled(name) {
		return fmt.Errorf("%w: plugin %q is not enabled", plugin.ErrNotFound, name)
	}
	p, ok := s.manager.Provider(name)
	if !ok {
		return nil // standby：延迟到首次激活时应用。
	}
	return s.configure(ctx, name, p)
}

// activateAndApply 确保插件已加载，并用已保存配置 Configure 它。它用于真正
// 需要使用插件的路径（构造通知渠道、测试发送、预热）。
func (s *PluginService) activateAndApply(ctx context.Context, name string) error {
	if s.manager == nil {
		return fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	p, err := s.manager.Acquire(ctx, name)
	if err != nil {
		return err
	}
	return s.configure(ctx, name, p)
}

// configure 解析已保存配置并注入给定实例。
func (s *PluginService) configure(ctx context.Context, name string, p plugin.Provider) error {
	cfg, err := s.resolve(ctx, name)
	if err != nil {
		return err
	}
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	if err := p.Configure(ctx, cfg); err != nil {
		return fmt.Errorf("plugin %q: configure: %w", name, err)
	}
	return nil
}

// Invoke 调用插件执行一次操作。已启用但空闲卸载的插件会按需重新加载。
func (s *PluginService) Invoke(ctx context.Context, category, name, op string, payload []byte) ([]byte, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	return s.manager.Invoke(ctx, category, name, op, payload)
}

// smsPayload 是交给短信插件的中性载荷，字段与 SDK 的 SMSMessage 对齐。
type smsPayload struct {
	To       string            `json:"to"`
	Subject  string            `json:"subject,omitempty"`
	Body     string            `json:"body,omitempty"`
	Template string            `json:"template,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
	SignName string            `json:"sign_name,omitempty"`
}

// Sender 用插件构造一个短信渠道。调用前会用已保存配置配置该插件。
func (s *PluginService) Sender(ctx context.Context, name string) (notify.Sender, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	if !s.manager.Has(plugin.CategoryNotifySMS, name) {
		return nil, fmt.Errorf("%w: sms plugin %q", plugin.ErrNotFound, name)
	}
	applyCtx, cancel := context.WithTimeout(ctx, pluginApplyTimeout)
	defer cancel()
	if err := s.activateAndApply(applyCtx, name); err != nil {
		return nil, err
	}
	return NewPluginSender(s, plugin.CategoryNotifySMS, name), nil
}

// Test 用当前已保存的配置向插件发送一条测试通知，返回插件的结果载荷。
func (s *PluginService) Test(ctx context.Context, name, to, subject, body string) ([]byte, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	p, err := s.manager.Acquire(ctx, name)
	if err != nil {
		return nil, err
	}
	applyCtx, cancel := context.WithTimeout(ctx, pluginApplyTimeout)
	defer cancel()
	if err := s.activateAndApply(applyCtx, name); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(smsPayload{To: to, Subject: subject, Body: body})
	if err != nil {
		return nil, err
	}
	return p.Invoke(ctx, "send", payload)
}

// loadRaw 读取落库的原始配置（秘钥仍为密文）。
func (s *PluginService) loadRaw(ctx context.Context, name string) (map[string]string, error) {
	raw, err := s.repo.GetSetting(ctx, pluginConfigPrefix+name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("plugin %q: load config: %w", name, err)
	}
	var stored map[string]string
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		s.logger.Warn("ignoring corrupt plugin config",
			slog.String("plugin", name), slog.Any("error", err))
		return map[string]string{}, nil
	}
	return stored, nil
}

// aad 返回某插件某字段密文的 AAD 标识。
func (s *PluginService) aad(name, key string) string {
	return pluginConfigPrefix + name + "." + key
}
