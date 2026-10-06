package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/moderation"
	"github.com/AXmishell/axmipic/internal/store"
)

// moderationSettingKey 是图片广场 AI 审查在数据库中的键。
const moderationSettingKey = "moderation"

// aadModerationAPIKey 是审查 API Key 密文的 AAD 标识。
const aadModerationAPIKey = "settings.moderation.api_key"

// ModerationSettingsDTO 是图片广场 AI 审查设置的对外表示（密钥仅返回是否已设置）。
type ModerationSettingsDTO struct {
	Enabled    bool   `json:"enabled"`
	BaseURL    string `json:"base_url"`
	Model      string `json:"model"`
	TimeoutSec int    `json:"timeout_sec"`
	Prompt     string `json:"prompt"`
	MaxImageMB int    `json:"max_image_mb"`
	APIKeySet  bool   `json:"api_key_set"`
}

// ModerationSettingsInput 是保存审查设置的输入。APIKey 为空表示保持原密钥。
type ModerationSettingsInput struct {
	Enabled    bool   `json:"enabled"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	Model      string `json:"model"`
	TimeoutSec int    `json:"timeout_sec"`
	Prompt     string `json:"prompt"`
	MaxImageMB int    `json:"max_image_mb"`
}

// storedModeration 是审查设置的落库形态，APIKey 以密文保存。
type storedModeration struct {
	Enabled    bool   `json:"enabled"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	Model      string `json:"model"`
	TimeoutSec int    `json:"timeout_sec"`
	Prompt     string `json:"prompt"`
	MaxImageMB int    `json:"max_image_mb"`
}

// SetModerationDefaults 记录 AI 审查的配置兜底值，并作为 BootstrapModeration
// 之前的当前值。
func (s *SettingsService) SetModerationDefaults(cfg config.ModerationConfig) {
	normalizeModerationConfig(&cfg)
	s.mu.Lock()
	s.moderationDefaults = cfg
	s.currentModeration = cfg
	s.mu.Unlock()
}

// SetModerationApplier 安装一个回调，用于在审查设置变化时即时重建运行中的审查器。
func (s *SettingsService) SetModerationApplier(fn func(config.ModerationConfig) error) {
	s.mu.Lock()
	s.moderationApplier = fn
	s.mu.Unlock()
}

// ApplyStoredModeration 用当前已加载的审查配置构建运行中的审查器。启动时在安装
// applier 之后调用，使数据库中的设置立即生效。
func (s *SettingsService) ApplyStoredModeration() error {
	s.mu.RLock()
	cfg := s.currentModeration
	applier := s.moderationApplier
	s.mu.RUnlock()
	if applier == nil {
		return nil
	}
	return applier(cfg)
}

// BootstrapModeration 加载数据库中保存的审查设置；首次启动时用配置兜底写库，
// 使其可在后台在线维护。记录损坏时保留配置兜底。
func (s *SettingsService) BootstrapModeration(ctx context.Context) error {
	raw, err := s.repo.GetSetting(ctx, moderationSettingKey)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("settings: load moderation: %w", err)
		}
		s.mu.RLock()
		fallback := s.moderationDefaults
		s.mu.RUnlock()
		if err := s.saveModeration(ctx, fallback); err != nil {
			return err
		}
		s.applyModeration(fallback)
		return nil
	}
	cfg, err := s.decodeModeration(raw)
	if err != nil {
		s.logger.Warn("ignoring corrupt moderation settings", slog.Any("error", err))
		return nil
	}
	normalizeModerationConfig(&cfg)
	s.applyModeration(cfg)
	return nil
}

// Moderation 返回当前生效的审查设置（密钥脱敏）。
func (s *SettingsService) Moderation() ModerationSettingsDTO {
	s.mu.RLock()
	cfg := s.currentModeration
	s.mu.RUnlock()
	return moderationToDTO(cfg)
}

// UpdateModeration 校验并保存审查设置，成功后即时重建运行中的审查器。
func (s *SettingsService) UpdateModeration(ctx context.Context, in ModerationSettingsInput) (ModerationSettingsDTO, error) {
	s.mu.RLock()
	existing := s.currentModeration
	applier := s.moderationApplier
	s.mu.RUnlock()

	cfg := moderationConfigFromInput(in)
	// 未提供（空）的密钥沿用现有值。
	if strings.TrimSpace(in.APIKey) == "" {
		cfg.APIKey = existing.APIKey
	}
	normalizeModerationConfig(&cfg)

	// 先应用，确保配置可用；失败则不落库。
	if applier != nil {
		if err := applier(cfg); err != nil {
			return ModerationSettingsDTO{}, fmt.Errorf("%w: %v", ErrSettingsConfig, err)
		}
	}
	if err := s.saveModeration(ctx, cfg); err != nil {
		return ModerationSettingsDTO{}, err
	}
	s.setCurrentModeration(cfg)
	return s.Moderation(), nil
}

// saveModeration 把审查设置序列化并写库（密钥加密）。
func (s *SettingsService) saveModeration(ctx context.Context, cfg config.ModerationConfig) error {
	encoded, err := s.encodeModeration(cfg)
	if err != nil {
		return err
	}
	if err := s.repo.SetSetting(ctx, moderationSettingKey, encoded); err != nil {
		return fmt.Errorf("settings: save moderation: %w", err)
	}
	return nil
}

// setCurrentModeration 更新内存中的审查配置。
func (s *SettingsService) setCurrentModeration(cfg config.ModerationConfig) {
	s.mu.Lock()
	s.currentModeration = cfg
	s.mu.Unlock()
}

// applyModeration 把审查设置切换到运行中的服务并更新缓存。
func (s *SettingsService) applyModeration(cfg config.ModerationConfig) {
	s.mu.Lock()
	applier := s.moderationApplier
	s.currentModeration = cfg
	s.mu.Unlock()
	if applier != nil {
		if err := applier(cfg); err != nil {
			s.logger.Warn("apply moderation settings", slog.Any("error", err))
		}
	}
}

// encodeModeration 序列化审查设置，APIKey 以密文写入。
func (s *SettingsService) encodeModeration(cfg config.ModerationConfig) (string, error) {
	encrypted, err := s.cipher.EncryptWithAAD(cfg.APIKey, aadModerationAPIKey)
	if err != nil {
		return "", fmt.Errorf("settings: encrypt moderation api key: %w", err)
	}
	payload, err := json.Marshal(storedModeration{
		Enabled:    cfg.Enabled,
		BaseURL:    cfg.BaseURL,
		APIKey:     encrypted,
		Model:      cfg.Model,
		TimeoutSec: cfg.TimeoutSec,
		Prompt:     cfg.Prompt,
		MaxImageMB: cfg.MaxImageMB,
	})
	if err != nil {
		return "", fmt.Errorf("settings: encode moderation: %w", err)
	}
	return string(payload), nil
}

// decodeModeration 解析落库的审查设置并解密 APIKey。
func (s *SettingsService) decodeModeration(raw string) (config.ModerationConfig, error) {
	var stored storedModeration
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return config.ModerationConfig{}, fmt.Errorf("settings: decode moderation: %w", err)
	}
	key, err := s.cipher.DecryptWithAAD(stored.APIKey, aadModerationAPIKey)
	if err != nil {
		return config.ModerationConfig{}, fmt.Errorf("settings: decrypt moderation api key: %w", err)
	}
	return config.ModerationConfig{
		Enabled:    stored.Enabled,
		BaseURL:    stored.BaseURL,
		APIKey:     key,
		Model:      stored.Model,
		TimeoutSec: stored.TimeoutSec,
		Prompt:     stored.Prompt,
		MaxImageMB: stored.MaxImageMB,
	}, nil
}

// moderationConfigFromInput 把提交输入转换为运行时配置。
func moderationConfigFromInput(in ModerationSettingsInput) config.ModerationConfig {
	return config.ModerationConfig{
		Enabled:    in.Enabled,
		BaseURL:    strings.TrimSpace(in.BaseURL),
		APIKey:     strings.TrimSpace(in.APIKey),
		Model:      strings.TrimSpace(in.Model),
		TimeoutSec: in.TimeoutSec,
		Prompt:     strings.TrimSpace(in.Prompt),
		MaxImageMB: in.MaxImageMB,
	}
}

// normalizeModerationConfig 为空字段补上默认值。
func normalizeModerationConfig(cfg *config.ModerationConfig) {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.Prompt = strings.TrimSpace(cfg.Prompt)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	// 提示词留空时回填内置默认值，使后台直接展示可编辑的默认提示词。
	if cfg.Prompt == "" {
		cfg.Prompt = moderation.DefaultPrompt
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 30
	}
	if cfg.MaxImageMB <= 0 {
		cfg.MaxImageMB = 10
	}
}

// moderationToDTO 组装对外表示（密钥脱敏）。
func moderationToDTO(cfg config.ModerationConfig) ModerationSettingsDTO {
	return ModerationSettingsDTO{
		Enabled:    cfg.Enabled,
		BaseURL:    cfg.BaseURL,
		Model:      cfg.Model,
		TimeoutSec: cfg.TimeoutSec,
		Prompt:     cfg.Prompt,
		MaxImageMB: cfg.MaxImageMB,
		APIKeySet:  strings.TrimSpace(cfg.APIKey) != "",
	}
}
