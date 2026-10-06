package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/AXmishell/axmipic/internal/plugin"
)

// pluginStatePrefix 是插件启用状态在 settings 表中的键前缀。
const pluginStatePrefix = "plugin.state."

// pluginDescriptorPrefix 是插件自描述缓存的键前缀。未加载（暂停/空闲卸载）
// 的插件也可据此在后台展示配置表单与渠道列表。
const pluginDescriptorPrefix = "plugin.desc."

// 启用状态的落库值。
const (
	pluginStateEnabled  = "enabled"
	pluginStateDisabled = "disabled"
)

// PluginStatus 是插件安装与启用状态的对外表示。
type PluginStatus struct {
	plugin.Descriptor
	// Runtime 为运行时（wasm / process）。
	Runtime string `json:"runtime"`
	// State 为三态：disabled（暂停）/ standby（已启用待激活）/ active（已加载）。
	State string `json:"state"`
	// Enabled 为是否已启用（state != disabled）。
	Enabled bool `json:"enabled"`
	// Loaded 为当前是否已实例化（占用内存）。standby 或 disabled 时为 false。
	Loaded bool `json:"loaded"`
	// MemoryBytes 为已加载插件报告的内存占用（WASM 线性内存）；未知时为 0。
	MemoryBytes uint64 `json:"memory_bytes,omitempty"`
	// LastUsed 为最近一次调用时间（Unix 秒）；0 表示加载后尚未调用。
	LastUsed int64 `json:"last_used,omitempty"`
	// LastError 为最近一次激活失败的描述；成功后为空。
	LastError string `json:"last_error,omitempty"`
	// Configured 表示是否已保存配置。
	Configured bool `json:"configured"`
}

// pluginState 由启用/加载标志推导三态字符串。
func pluginState(enabled, loaded bool) string {
	switch {
	case !enabled:
		return "disabled"
	case loaded:
		return "active"
	default:
		return "standby"
	}
}

// Bootstrap 扫描插件目录并按启动模式加载：
//   - startup=lazy（默认）：所有已安装插件登记为待激活，不实例化；
//   - startup=eager：仅实例化「启用」的插件，暂停的不加载；
//   - preload 中的插件无论模式都会立即激活。
//
// 随后为未加载的插件注入缓存自描述，并缓存已加载插件的自描述。启动阶段调用一次。
func (s *PluginService) Bootstrap(ctx context.Context, startup string, preload []string) error {
	if s.manager == nil {
		return nil
	}
	eager := startup == "eager"
	enabled := func(name string) bool {
		if s.repo == nil {
			return true
		}
		state, err := s.repo.GetSetting(ctx, pluginStatePrefix+name)
		if err != nil {
			return true // 无记录或读取失败：默认启用。
		}
		return state != pluginStateDisabled
	}
	include := func(name string) bool {
		if !eager {
			return false // lazy：只登记启用意图，不实例化。
		}
		return enabled(name)
	}
	if err := s.manager.LoadFiltered(ctx, include, enabled); err != nil {
		s.logger.Warn("failed to load plugins", slog.Any("error", err))
	}

	// preload：显式预热，使其立即进入 active。
	for _, name := range preload {
		if _, ok := s.manager.Describe(name); !ok {
			s.logger.Warn("preload plugin not installed", slog.String("plugin", name))
			continue
		}
		if _, err := s.manager.Enable(ctx, name); err != nil {
			s.logger.Warn("failed to enable preload plugin", slog.String("plugin", name), slog.Any("error", err))
			continue
		}
		applyCtx, cancel := context.WithTimeout(ctx, pluginApplyTimeout)
		if err := s.activateAndApply(applyCtx, name); err != nil {
			s.logger.Warn("failed to preload plugin", slog.String("plugin", name), slog.Any("error", err))
		}
		cancel()
	}

	if s.repo == nil {
		return nil
	}
	for _, info := range s.manager.Installed() {
		if info.Loaded {
			s.cacheDescriptor(ctx, info.Manifest.Name, info.Descriptor)
			continue
		}
		raw, err := s.repo.GetSetting(ctx, pluginDescriptorPrefix+info.Manifest.Name)
		if err != nil {
			continue
		}
		var d plugin.Descriptor
		if json.Unmarshal([]byte(raw), &d) != nil {
			continue
		}
		s.manager.SetDescriptor(info.Manifest.Name, d)
	}
	return nil
}

// cacheDescriptor 持久化插件自描述，供插件未加载时后台展示。失败仅告警。
func (s *PluginService) cacheDescriptor(ctx context.Context, name string, d plugin.Descriptor) {
	if s.repo == nil {
		return
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return
	}
	if err := s.repo.SetSetting(ctx, pluginDescriptorPrefix+name, string(raw)); err != nil {
		s.logger.Warn("failed to cache plugin descriptor",
			slog.String("plugin", name), slog.Any("error", err))
	}
}

// SuspendIdle 卸载空闲超过 idle 的已启用插件以回收内存；再次调用时会按需
// 重新加载。idle<=0 时不执行。返回被卸载的插件数量。
func (s *PluginService) SuspendIdle(ctx context.Context, idle time.Duration) int {
	if s.manager == nil || idle <= 0 {
		return 0
	}
	names := s.manager.SuspendIdle(ctx, idle)
	if len(names) > 0 {
		s.logger.Info("suspended idle plugins",
			slog.Int("count", len(names)), slog.Any("plugins", names))
	}
	return len(names)
}

// Installed 返回全部已安装插件的状态。
func (s *PluginService) Installed(ctx context.Context) []PluginStatus {
	if s.manager == nil {
		return nil
	}
	infos := s.manager.Installed()
	out := make([]PluginStatus, 0, len(infos))
	for _, info := range infos {
		configured := false
		if stored, err := s.loadRaw(ctx, info.Manifest.Name); err == nil {
			configured = len(stored) > 0
		}
		out = append(out, PluginStatus{
			Descriptor:  info.Descriptor,
			Runtime:     info.Manifest.Runtime,
			State:       pluginState(info.Enabled, info.Loaded),
			Enabled:     info.Enabled,
			Loaded:      info.Loaded,
			MemoryBytes: info.MemoryBytes,
			LastUsed:    info.LastUsed.Unix(),
			LastError:   info.LastError,
			Configured:  configured,
		})
	}
	return out
}

// SetEnabled 启用或暂停一个插件，并持久化状态。启用会按需实例化（若尚未加载）。
func (s *PluginService) SetEnabled(ctx context.Context, name string, enable bool) error {
	if s.manager == nil {
		return fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	if enable {
		// 仅标记为启用（standby），不立即加载：首次使用时按需激活。
		if _, err := s.manager.Enable(ctx, name); err != nil {
			return err
		}
		if desc, ok := s.manager.Describe(name); ok {
			s.cacheDescriptor(ctx, name, desc)
		}
	} else {
		if err := s.manager.Disable(ctx, name); err != nil {
			return err
		}
	}
	if s.repo != nil {
		value := pluginStateEnabled
		if !enable {
			value = pluginStateDisabled
		}
		if err := s.repo.SetSetting(ctx, pluginStatePrefix+name, value); err != nil {
			return fmt.Errorf("plugin %q: persist state: %w", name, err)
		}
	}
	return nil
}

// Registry 拉取插件市场索引。
func (s *PluginService) Registry(ctx context.Context) ([]plugin.MarketEntry, error) {
	if s.manager == nil {
		return nil, fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	return s.manager.FetchRegistry(ctx)
}

// RegistryConfigured 报告是否配置了插件市场索引地址。
func (s *PluginService) RegistryConfigured() bool {
	return s.manager != nil && s.manager.RegistryConfigured()
}

// Install 安装插件（按 URL 或索引名称），并标记为启用。
func (s *PluginService) Install(ctx context.Context, spec plugin.InstallSpec) (plugin.Manifest, error) {
	if s.manager == nil {
		return plugin.Manifest{}, fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	man, err := s.manager.InstallFromURL(ctx, spec)
	if err != nil {
		return plugin.Manifest{}, err
	}
	s.markInstalled(ctx, man.Name)
	return man, nil
}

// InstallArchive 从上传的归档安装插件，并标记为启用。
func (s *PluginService) InstallArchive(ctx context.Context, data []byte, sha256Hex, signature string) (plugin.Manifest, error) {
	if s.manager == nil {
		return plugin.Manifest{}, fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	man, err := s.manager.InstallFromArchive(ctx, data, sha256Hex, signature)
	if err != nil {
		return plugin.Manifest{}, err
	}
	s.markInstalled(ctx, man.Name)
	return man, nil
}

// markInstalled 将插件状态重置为启用（重新安装时避免沿用旧的暂停态）。
func (s *PluginService) markInstalled(ctx context.Context, name string) {
	if s.repo == nil {
		return
	}
	if err := s.repo.SetSetting(ctx, pluginStatePrefix+name, pluginStateEnabled); err != nil {
		s.logger.Warn("failed to reset plugin state", slog.String("plugin", name), slog.Any("error", err))
	}
}

// Remove 卸载并删除插件，同时清理其启用状态。
func (s *PluginService) Remove(ctx context.Context, name string) error {
	if s.manager == nil {
		return fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	if err := s.manager.Remove(ctx, name); err != nil {
		return err
	}
	if s.repo != nil {
		if err := s.repo.SetSetting(ctx, pluginStatePrefix+name, pluginStateEnabled); err != nil {
			s.logger.Warn("failed to clear plugin state", slog.String("plugin", name), slog.Any("error", err))
		}
	}
	return nil
}

// Reload 重新加载指定插件（保持其启用状态）。
func (s *PluginService) Reload(ctx context.Context, name string) error {
	if s.manager == nil {
		return fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	if err := s.manager.Reload(ctx, name); err != nil {
		return err
	}
	if s.manager.Enabled(name) {
		applyCtx, cancel := context.WithTimeout(ctx, pluginApplyTimeout)
		defer cancel()
		if err := s.activateAndApply(applyCtx, name); err != nil {
			s.logger.Warn("plugin reloaded but not yet applyable",
				slog.String("plugin", name), slog.Any("error", err))
		}
	}
	return nil
}
