package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AXmishell/axmipic/internal/plugin"
)

// pluginStatePrefix 是插件启用状态在 settings 表中的键前缀。
const pluginStatePrefix = "plugin.state."

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
	// Enabled 为是否已启用（未暂停）。
	Enabled bool `json:"enabled"`
	// Configured 表示是否已保存配置。
	Configured bool `json:"configured"`
}

// Bootstrap 加载插件目录并应用持久化的暂停状态。启动阶段调用一次。
func (s *PluginService) Bootstrap(ctx context.Context) error {
	if s.manager == nil {
		return nil
	}
	if err := s.manager.Load(ctx); err != nil {
		s.logger.Warn("failed to load plugins", slog.Any("error", err))
	}
	if s.repo == nil {
		return nil
	}
	for _, info := range s.manager.Installed() {
		state, err := s.repo.GetSetting(ctx, pluginStatePrefix+info.Manifest.Name)
		if err != nil {
			continue // 无记录：默认启用。
		}
		if state == pluginStateDisabled {
			if err := s.manager.Disable(ctx, info.Manifest.Name); err != nil {
				s.logger.Warn("failed to pause plugin",
					slog.String("plugin", info.Manifest.Name), slog.Any("error", err))
			}
		}
	}
	return nil
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
			Descriptor: info.Descriptor,
			Runtime:    info.Manifest.Runtime,
			Enabled:    info.Enabled,
			Configured: configured,
		})
	}
	return out
}

// SetEnabled 启用或暂停一个插件，并持久化状态。
func (s *PluginService) SetEnabled(ctx context.Context, name string, enable bool) error {
	if s.manager == nil {
		return fmt.Errorf("%w: plugins are unavailable", plugin.ErrNotFound)
	}
	if enable {
		if _, err := s.manager.Enable(ctx, name); err != nil {
			return err
		}
		// 启用后尽力应用已保存配置；配置不完整不阻止启用。
		applyCtx, cancel := context.WithTimeout(ctx, pluginApplyTimeout)
		defer cancel()
		if err := s.Apply(applyCtx, name); err != nil {
			s.logger.Warn("plugin enabled but not yet applyable",
				slog.String("plugin", name), slog.Any("error", err))
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
		if err := s.Apply(applyCtx, name); err != nil {
			s.logger.Warn("plugin reloaded but not yet applyable",
				slog.String("plugin", name), slog.Any("error", err))
		}
	}
	return nil
}
