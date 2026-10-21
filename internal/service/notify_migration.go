package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/plugin"
	"github.com/AXmishell/axmipic/internal/store"
)

// notifyMigrationKey 标记旧版 SMTP / 短信设置已经迁移到插件配置。
const notifyMigrationKey = "plugin.migrated.notify_v1"

// MigrateLegacyNotify 把旧版 settings.smtp 与 settings.sms 迁移为内置插件的
// 配置，并写入默认的当前渠道。迁移只执行一次；之后以插件配置为准。
func (s *PluginService) MigrateLegacyNotify(ctx context.Context, settingsSvc *SettingsService, cfg config.Config, logger *slog.Logger) {
	if s.manager == nil || s.repo == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	if _, err := s.repo.GetSetting(ctx, notifyMigrationKey); err == nil {
		return
	}

	smsActive := BuiltinSMSLog
	if legacy, ok := s.legacySMS(ctx, cfg); ok && legacy.Enabled {
		switch channel := strings.TrimSpace(legacy.Channel); {
		case channel == "log":
			smsActive = BuiltinSMSLog
		case channel == "", channel == "http":
			if strings.TrimSpace(legacy.Endpoint) != "" {
				s.seedPluginConfig(ctx, BuiltinSMSHTTP, map[string]string{
					"provider": legacy.Provider,
					"endpoint": legacy.Endpoint,
					"method":   legacy.Method,
				}, logger)
				smsActive = BuiltinSMSHTTP
			}
		default:
			smsActive = channel
		}
	}

	emailActive := BuiltinEmailLog
	if legacy, ok := s.legacySMTP(ctx, cfg, settingsSvc); ok && legacy.Enabled && strings.TrimSpace(legacy.Host) != "" {
		s.seedPluginConfig(ctx, BuiltinEmailSMTP, map[string]string{
			"host":     legacy.Host,
			"port":     fmt.Sprintf("%d", legacy.Port),
			"username": legacy.Username,
			"password": legacy.Password,
			"from":     legacy.From,
			"use_tls":  fmt.Sprintf("%t", legacy.UseTLS),
		}, logger)
		emailActive = BuiltinEmailSMTP
	}

	// 不覆盖管理员已经显式选择的当前渠道。
	if _, err := s.repo.GetSetting(ctx, ActiveKey(plugin.CategoryNotifySMS)); errors.Is(err, store.ErrNotFound) {
		_ = s.repo.SetSetting(ctx, ActiveKey(plugin.CategoryNotifySMS), smsActive)
	}
	if _, err := s.repo.GetSetting(ctx, ActiveKey(plugin.CategoryNotifyEmail)); errors.Is(err, store.ErrNotFound) {
		_ = s.repo.SetSetting(ctx, ActiveKey(plugin.CategoryNotifyEmail), emailActive)
	}
	if err := s.repo.SetSetting(ctx, notifyMigrationKey, "1"); err != nil {
		logger.Warn("failed to mark notify plugin migration", slog.Any("error", err))
	}
	logger.Info("notify plugins migrated",
		slog.String("sms_channel", smsActive),
		slog.String("email_channel", emailActive),
	)
}

// seedPluginConfig 仅在插件尚无配置时写入初始值，避免覆盖管理员配置。
func (s *PluginService) seedPluginConfig(ctx context.Context, name string, values map[string]string, logger *slog.Logger) {
	if stored, err := s.loadRaw(ctx, name); err == nil && len(stored) > 0 {
		return
	}
	if err := s.SaveConfig(ctx, name, values); err != nil {
		logger.Warn("failed to seed built-in plugin config",
			slog.String("plugin", name), slog.Any("error", err))
		return
	}
	if p, ok := s.manager.Provider(name); ok {
		if err := s.configure(ctx, name, p); err != nil {
			logger.Warn("failed to apply built-in plugin config",
				slog.String("plugin", name), slog.Any("error", err))
		}
	}
}

// legacySMS 读取旧版短信设置；没有数据库记录时回退到配置文件。
func (s *PluginService) legacySMS(ctx context.Context, cfg config.Config) (SMSSettings, bool) {
	raw, err := s.repo.GetSetting(ctx, "sms")
	if err == nil {
		var v SMSSettings
		if json.Unmarshal([]byte(raw), &v) == nil {
			return v, true
		}
	}
	if cfg.SMS.Enabled || cfg.SMS.Endpoint != "" || cfg.SMS.Channel != "" {
		return SMSSettings{
			Enabled:  cfg.SMS.Enabled,
			Channel:  cfg.SMS.Channel,
			Provider: cfg.SMS.Provider,
			Endpoint: cfg.SMS.Endpoint,
			Method:   cfg.SMS.Method,
		}, true
	}
	return SMSSettings{}, false
}

// legacySMTP 读取旧版 SMTP 设置；没有数据库记录时回退到配置文件。
func (s *PluginService) legacySMTP(ctx context.Context, cfg config.Config, settingsSvc *SettingsService) (SMTPConfig, bool) {
	raw, err := s.repo.GetSetting(ctx, smtpSettingKey)
	if err == nil && settingsSvc != nil {
		if v, decErr := settingsSvc.decode(raw); decErr == nil {
			return v, true
		}
	}
	if cfg.Email.Enabled || cfg.Email.Host != "" {
		return SMTPConfig{
			Enabled:  cfg.Email.Enabled,
			Host:     cfg.Email.Host,
			Port:     cfg.Email.Port,
			Username: cfg.Email.Username,
			Password: cfg.Email.Password,
			From:     cfg.Email.From,
			UseTLS:   cfg.Email.UseTLS,
		}, true
	}
	return SMTPConfig{}, false
}
