package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/store"
)

// ErrSettingsConfig 表示提交的设置无效。
var ErrSettingsConfig = errors.New("service: invalid settings")

// smtpSettingKey 是 SMTP 设置在数据库中的键。
const smtpSettingKey = "smtp"

// authSettingKey 是权限开关在数据库中的键。
const authSettingKey = "auth"

// aadSMTPPassword 是 SMTP 密码密文的 AAD 标识（防密文跨字段搬运）。
const aadSMTPPassword = "settings.smtp.password"

// AuthConfig 是可在线切换的权限开关。
type AuthConfig struct {
	// AllowRegistration 控制是否开放自助注册。
	AllowRegistration bool `json:"allow_registration"`
	// RequireAuth 控制上传接口是否强制登录。
	RequireAuth bool `json:"require_auth"`
	// AllowGuestUpload 允许未登录访客以 Guest 角色上传；为真时覆盖 RequireAuth。
	AllowGuestUpload bool `json:"allow_guest_upload"`
}

// SMTPConfig 是 SMTP 邮件渠道的完整配置（含明文密码，仅驻留内存）。
type SMTPConfig struct {
	Enabled  bool
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
}

// SMTPConfigDTO 是 SMTP 配置的对外表示；密码仅返回「是否已设置」，绝不回传明文。
type SMTPConfigDTO struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	From        string `json:"from"`
	UseTLS      bool   `json:"use_tls"`
	PasswordSet bool   `json:"password_set"`
}

// SMTPInput 是更新 SMTP 配置的输入。Password 为空表示保持原密码不变。
type SMTPInput struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	UseTLS   bool   `json:"use_tls"`
}

// storedSMTP 是 SMTP 设置的落库形态，密码以密文保存。
type storedSMTP struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	UseTLS   bool   `json:"use_tls"`
}

// SettingsService 管理运行时可修改的系统设置，并把更改即时应用到相关服务。
type SettingsService struct {
	repo   *store.Repository
	cipher *secret.Cipher
	notify *NotifyService
	logger *slog.Logger

	mu      sync.RWMutex
	current SMTPConfig
	// 支付设置：默认值来自配置文件，运行值来自数据库，可热替换。
	paymentDefaults config.PaymentConfig
	currentPayment  config.PaymentConfig
	paymentApplier  func(config.PaymentConfig) error
	// 权限设置：默认值来自配置文件，运行值来自数据库，可热替换。
	authDefaults AuthConfig
	currentAuth  AuthConfig
	authApplier  func(AuthConfig)
	// 图片广场 AI 审查：默认值来自配置文件，运行值来自数据库，可热替换。
	moderationDefaults config.ModerationConfig
	currentModeration  config.ModerationConfig
	moderationApplier  func(config.ModerationConfig) error

	// domains 是按域组织的通用设置（upload/processing/security/sms/limits/site/maintenance）。
	domains map[string]SettingDomain
}

// NewSettingsService 构建设置服务。fallback 为配置文件中的 SMTP 配置，在数据库
// 尚无记录时作为当前生效值。
func NewSettingsService(
	repo *store.Repository,
	cipher *secret.Cipher,
	notifySvc *NotifyService,
	logger *slog.Logger,
	fallback SMTPConfig,
) *SettingsService {
	return &SettingsService{
		repo:    repo,
		cipher:  cipher,
		notify:  notifySvc,
		logger:  logger,
		current: fallback,
		domains: make(map[string]SettingDomain),
	}
}

// RegisterDomain 注册一个设置域。
func (s *SettingsService) RegisterDomain(d SettingDomain) {
	if s.domains == nil {
		s.domains = make(map[string]SettingDomain)
	}
	s.domains[d.Name()] = d
}

// Domain 返回指定名称的设置域。
func (s *SettingsService) Domain(name string) (SettingDomain, bool) {
	d, ok := s.domains[name]
	return d, ok
}

// Domains 返回全部设置域的名称。
func (s *SettingsService) Domains() []string {
	names := make([]string, 0, len(s.domains))
	for name := range s.domains {
		names = append(names, name)
	}
	return names
}

// BootstrapDomains 从数据库加载并应用全部设置域。
func (s *SettingsService) BootstrapDomains(ctx context.Context) error {
	for _, d := range s.domains {
		if err := d.Bootstrap(ctx); err != nil {
			return fmt.Errorf("settings: bootstrap domain %s: %w", d.Name(), err)
		}
	}
	return nil
}

// DomainValue 返回指定域的当前运行值（类型化）。
func DomainValue[T any](s *SettingsService, name string) (T, bool) {
	d, ok := s.Domain(name)
	if !ok {
		var zero T
		return zero, false
	}
	v, ok := d.Get().(T)
	return v, ok
}

// Bootstrap 加载数据库中保存的 SMTP 设置并即时应用；没有记录或记录损坏时
// 保持配置文件兜底，不影响启动。
func (s *SettingsService) Bootstrap(ctx context.Context) error {
	raw, err := s.repo.GetSetting(ctx, smtpSettingKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("settings: load smtp: %w", err)
	}
	cfg, err := s.decode(raw)
	if err != nil {
		s.logger.Warn("ignoring corrupt smtp settings", slog.Any("error", err))
		return nil
	}
	if _, err := s.buildSender(cfg); err != nil {
		s.logger.Warn("ignoring unusable stored smtp settings", slog.Any("error", err))
		return nil
	}
	s.apply(cfg)
	return nil
}

// BootstrapPayment 加载数据库中保存的支付设置；首次启动时用配置兜底写库，使其
// 可在后台编辑。记录损坏时保留配置兜底。
func (s *SettingsService) BootstrapPayment(ctx context.Context) error {
	raw, err := s.repo.GetSetting(ctx, paymentSettingKey)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("settings: load payment: %w", err)
		}
		s.mu.RLock()
		fallback := s.paymentDefaults
		s.mu.RUnlock()
		fallback.DefaultGateway = normalizeDefaultGateway(fallback)
		encoded, encErr := s.encodePayment(fallback)
		if encErr != nil {
			return encErr
		}
		if setErr := s.repo.SetSetting(ctx, paymentSettingKey, encoded); setErr != nil {
			return setErr
		}
		s.setCurrentPayment(fallback)
		return nil
	}
	cfg, err := s.decodePayment(raw)
	if err != nil {
		s.logger.Warn("ignoring corrupt payment settings", slog.Any("error", err))
		return nil
	}
	if err := validatePaymentConfig(cfg); err != nil {
		s.logger.Warn("ignoring invalid payment settings", slog.Any("error", err))
		return nil
	}
	cfg.DefaultGateway = normalizeDefaultGateway(cfg)
	s.setCurrentPayment(cfg)
	return nil
}

// SMTP 返回当前生效的 SMTP 配置（密码已脱敏）。
func (s *SettingsService) SMTP() SMTPConfigDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return toSMTPDTO(s.current)
}

// UpdateSMTP 校验并保存 SMTP 设置，成功后即时切换到新的邮件渠道。
func (s *SettingsService) UpdateSMTP(ctx context.Context, in SMTPInput) (SMTPConfigDTO, error) {
	s.mu.RLock()
	previous := s.current
	s.mu.RUnlock()

	cfg := SMTPConfig{
		Enabled:  in.Enabled,
		Host:     strings.TrimSpace(in.Host),
		Port:     in.Port,
		Username: strings.TrimSpace(in.Username),
		From:     strings.TrimSpace(in.From),
		UseTLS:   in.UseTLS,
		// 空密码表示沿用旧密码。
		Password: previous.Password,
	}
	if pwd := in.Password; pwd != "" {
		cfg.Password = pwd
	}
	if cfg.Port <= 0 {
		cfg.Port = 587
	}
	if cfg.From == "" {
		cfg.From = cfg.Username
	}
	if cfg.Enabled {
		if cfg.Host == "" {
			return SMTPConfigDTO{}, fmt.Errorf("%w: host is required", ErrSettingsConfig)
		}
		if cfg.From == "" {
			return SMTPConfigDTO{}, fmt.Errorf("%w: from or username is required", ErrSettingsConfig)
		}
	}
	if _, err := s.buildSender(cfg); err != nil {
		return SMTPConfigDTO{}, err
	}

	raw, err := s.encode(cfg)
	if err != nil {
		return SMTPConfigDTO{}, err
	}
	if err := s.repo.SetSetting(ctx, smtpSettingKey, raw); err != nil {
		return SMTPConfigDTO{}, err
	}
	s.apply(cfg)
	return toSMTPDTO(cfg), nil
}

// buildSender 依据配置构造邮件渠道；未启用时回退到日志渠道。
func (s *SettingsService) buildSender(cfg SMTPConfig) (notify.Sender, error) {
	if !cfg.Enabled {
		return notify.NewLogSender("email", s.logger), nil
	}
	sender, err := notify.NewSMTPSender(notify.SMTPOptions{
		Host:     cfg.Host,
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: cfg.Password,
		From:     cfg.From,
		UseTLS:   cfg.UseTLS,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSettingsConfig, err)
	}
	return sender, nil
}

// apply 把配置切换到运行中的通知服务并更新缓存。
func (s *SettingsService) apply(cfg SMTPConfig) {
	sender, err := s.buildSender(cfg)
	if err != nil {
		// 调用方已在保存前完成校验；此处仅作为防御。
		sender = notify.NewLogSender("email", s.logger)
	}
	s.notify.SetEmail(sender)
	s.mu.Lock()
	s.current = cfg
	s.mu.Unlock()
}

// encode 将配置序列化，密码以密文写入。
func (s *SettingsService) encode(cfg SMTPConfig) (string, error) {
	encrypted, err := s.cipher.EncryptWithAAD(cfg.Password, aadSMTPPassword)
	if err != nil {
		return "", fmt.Errorf("settings: encrypt smtp password: %w", err)
	}
	payload, err := json.Marshal(storedSMTP{
		Enabled:  cfg.Enabled,
		Host:     cfg.Host,
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: encrypted,
		From:     cfg.From,
		UseTLS:   cfg.UseTLS,
	})
	if err != nil {
		return "", fmt.Errorf("settings: encode smtp: %w", err)
	}
	return string(payload), nil
}

// decode 解析落库的设置，并解密密码。
func (s *SettingsService) decode(raw string) (SMTPConfig, error) {
	var stored storedSMTP
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return SMTPConfig{}, fmt.Errorf("settings: decode smtp: %w", err)
	}
	password, err := s.cipher.DecryptWithAAD(stored.Password, aadSMTPPassword)
	if err != nil {
		return SMTPConfig{}, fmt.Errorf("settings: decrypt smtp password: %w", err)
	}
	return SMTPConfig{
		Enabled:  stored.Enabled,
		Host:     stored.Host,
		Port:     stored.Port,
		Username: stored.Username,
		Password: password,
		From:     stored.From,
		UseTLS:   stored.UseTLS,
	}, nil
}

// toSMTPDTO 组装对外表示（密码脱敏）。
func toSMTPDTO(cfg SMTPConfig) SMTPConfigDTO {
	return SMTPConfigDTO{
		Enabled:     cfg.Enabled,
		Host:        cfg.Host,
		Port:        cfg.Port,
		Username:    cfg.Username,
		From:        cfg.From,
		UseTLS:      cfg.UseTLS,
		PasswordSet: strings.TrimSpace(cfg.Password) != "",
	}
}

// storedAuth 是权限设置的落库形态。字段使用指针以区分「未设置」与「显式 false」，
// 从而让旧版本写入的、缺少新增字段的记录能回退到配置默认值。
type storedAuth struct {
	AllowRegistration *bool `json:"allow_registration,omitempty"`
	RequireAuth       *bool `json:"require_auth,omitempty"`
	AllowGuestUpload  *bool `json:"allow_guest_upload,omitempty"`
}

// SetAuthDefaults 记录权限开关的配置兜底值，并作为 BootstrapAuth 之前的当前值。
func (s *SettingsService) SetAuthDefaults(cfg AuthConfig) {
	s.mu.Lock()
	s.authDefaults = cfg
	s.currentAuth = cfg
	s.mu.Unlock()
}

// SetAuthApplier 安装一个回调，用于在权限开关变化时即时应用到运行中的服务。
func (s *SettingsService) SetAuthApplier(fn func(AuthConfig)) {
	s.mu.Lock()
	s.authApplier = fn
	s.mu.Unlock()
}

// BootstrapAuth 加载数据库中保存的权限设置；首次启动时用配置兜底写库，使其可在
// 后台在线切换。缺失字段沿用配置默认，记录损坏时保留配置兜底。
func (s *SettingsService) BootstrapAuth(ctx context.Context) error {
	s.mu.RLock()
	fallback := s.authDefaults
	s.mu.RUnlock()

	raw, err := s.repo.GetSetting(ctx, authSettingKey)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("settings: load auth: %w", err)
		}
		return s.persistAuth(ctx, fallback)
	}
	var stored storedAuth
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		s.logger.Warn("ignoring corrupt auth settings", slog.Any("error", err))
		return nil
	}
	cfg := fallback
	if stored.AllowRegistration != nil {
		cfg.AllowRegistration = *stored.AllowRegistration
	}
	if stored.RequireAuth != nil {
		cfg.RequireAuth = *stored.RequireAuth
	}
	if stored.AllowGuestUpload != nil {
		cfg.AllowGuestUpload = *stored.AllowGuestUpload
	}
	s.applyAuth(cfg)
	// 旧版记录可能缺少新增字段，补齐后回写，避免每次启动都走合并逻辑。
	if stored.AllowRegistration == nil || stored.RequireAuth == nil || stored.AllowGuestUpload == nil {
		if err := s.persistAuth(ctx, cfg); err != nil {
			s.logger.Warn("failed to normalize auth settings", slog.Any("error", err))
		}
	}
	return nil
}

// Auth 返回当前生效的权限开关。
func (s *SettingsService) Auth() AuthConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentAuth
}

// UpdateAuth 保存权限开关并即时生效，无需重启。
func (s *SettingsService) UpdateAuth(ctx context.Context, in AuthConfig) (AuthConfig, error) {
	if err := s.persistAuth(ctx, in); err != nil {
		return AuthConfig{}, err
	}
	return in, nil
}

// persistAuth 将权限设置写入数据库并应用到运行中的服务。
func (s *SettingsService) persistAuth(ctx context.Context, cfg AuthConfig) error {
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("settings: encode auth: %w", err)
	}
	if err := s.repo.SetSetting(ctx, authSettingKey, string(encoded)); err != nil {
		return err
	}
	s.applyAuth(cfg)
	return nil
}

// applyAuth 把权限设置切换到运行中的服务并更新缓存。
func (s *SettingsService) applyAuth(cfg AuthConfig) {
	s.mu.Lock()
	applier := s.authApplier
	s.currentAuth = cfg
	s.mu.Unlock()
	if applier != nil {
		applier(cfg)
	}
}
