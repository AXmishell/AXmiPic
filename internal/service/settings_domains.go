package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/store"
)

// SettingDomain 是一个可在后台读取、更新并（可选）热应用的系统设置域。
type SettingDomain interface {
	// Name 返回域标识（同时也是 settings 表中的键）。
	Name() string
	// Get 返回当前运行值。
	Get() any
	// Update 校验并持久化新值，成功后返回生效值。
	Update(ctx context.Context, raw json.RawMessage) (any, error)
	// Bootstrap 从数据库加载（无记录时用默认值播种）并应用。
	Bootstrap(ctx context.Context) error
}

// settingDomain 是 SettingDomain 的类型化实现。
type settingDomain[T any] struct {
	repo     *store.Repository
	key      string
	defaults T
	validate func(T) error
	apply    func(T)

	mu      sync.RWMutex
	current T
}

// NewSettingDomain 构造一个类型化的设置域。validate 与 apply 均可为 nil。
func NewSettingDomain[T any](
	repo *store.Repository,
	key string,
	defaults T,
	validate func(T) error,
	apply func(T),
) SettingDomain {
	return &settingDomain[T]{
		repo:     repo,
		key:      key,
		defaults: defaults,
		validate: validate,
		apply:    apply,
		current:  defaults,
	}
}

func (d *settingDomain[T]) Name() string { return d.key }

func (d *settingDomain[T]) Get() any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.current
}

func (d *settingDomain[T]) Bootstrap(ctx context.Context) error {
	raw, err := d.repo.GetSetting(ctx, d.key)
	switch {
	case errors.Is(err, store.ErrNotFound):
		if err := d.persist(ctx, d.defaults); err != nil {
			return err
		}
		d.set(d.defaults)
	case err != nil:
		return err
	default:
		var loaded T
		if err := json.Unmarshal([]byte(raw), &loaded); err != nil {
			// 记录损坏时回退默认值，不影响启动。
			d.set(d.defaults)
		} else {
			d.set(loaded)
		}
	}
	d.applyCurrent()
	return nil
}

func (d *settingDomain[T]) Update(ctx context.Context, raw json.RawMessage) (any, error) {
	var in T
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON body", ErrSettingsConfig)
	}
	if d.validate != nil {
		if err := d.validate(in); err != nil {
			return nil, err
		}
	}
	if err := d.persist(ctx, in); err != nil {
		return nil, err
	}
	d.set(in)
	d.applyCurrent()
	return in, nil
}

func (d *settingDomain[T]) persist(ctx context.Context, v T) error {
	encoded, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("settings: encode %s: %w", d.key, err)
	}
	return d.repo.SetSetting(ctx, d.key, string(encoded))
}

func (d *settingDomain[T]) set(v T) {
	d.mu.Lock()
	d.current = v
	d.mu.Unlock()
}

func (d *settingDomain[T]) applyCurrent() {
	if d.apply == nil {
		return
	}
	d.apply(d.Get().(T))
}

// ---- 各设置域的类型与校验 ----

// UploadSettings 控制上传的大小与类型限制。
type UploadSettings struct {
	MaxSizeMB        int      `json:"max_size_mb"`
	AllowedMIMETypes []string `json:"allowed_mime_types"`
}

// ValidateUploadSettings 校验上传设置。
func ValidateUploadSettings(v UploadSettings) error {
	if v.MaxSizeMB < 1 {
		return fmt.Errorf("%w: max_size_mb must be at least 1", ErrSettingsConfig)
	}
	if len(v.AllowedMIMETypes) == 0 {
		return fmt.Errorf("%w: allowed_mime_types must not be empty", ErrSettingsConfig)
	}
	return nil
}

// ImagingSettings 控制即时图像变换（driver 需重启生效，不在此域内）。
type ImagingSettings struct {
	Enabled        bool     `json:"enabled"`
	MaxWidth       int      `json:"max_width"`
	MaxHeight      int      `json:"max_height"`
	DefaultQuality int      `json:"default_quality"`
	AllowedFormats []string `json:"allowed_formats"`
	AllowEnlarge   bool     `json:"allow_enlarge"`
	AllowEffects   bool     `json:"allow_effects"`
	AllowWatermark bool     `json:"allow_watermark"`
	WatermarkText  string   `json:"watermark_text"`
}

// ValidateImagingSettings 校验图像处理设置。
func ValidateImagingSettings(v ImagingSettings) error {
	if !v.Enabled {
		return nil
	}
	if v.MaxWidth < 1 || v.MaxHeight < 1 {
		return fmt.Errorf("%w: max dimensions must be at least 1", ErrSettingsConfig)
	}
	if v.DefaultQuality < 1 || v.DefaultQuality > 100 {
		return fmt.Errorf("%w: default_quality must be between 1 and 100", ErrSettingsConfig)
	}
	if len(v.AllowedFormats) == 0 {
		return fmt.Errorf("%w: allowed_formats must not be empty", ErrSettingsConfig)
	}
	for _, name := range v.AllowedFormats {
		if _, ok := imaging.ParseFormat(name); !ok {
			return fmt.Errorf("%w: unknown format %q", ErrSettingsConfig, name)
		}
	}
	return nil
}

// SecuritySettings 控制上传内容扫描与云处理。
type SecuritySettings struct {
	Scanner        string `json:"scanner"`
	CloudProcessor string `json:"cloud_processor"`
}

// ValidateSecuritySettings 校验安全设置。
func ValidateSecuritySettings(v SecuritySettings) error {
	switch v.Scanner {
	case "", "none", "builtin":
	default:
		return fmt.Errorf("%w: scanner %q is not supported", ErrSettingsConfig, v.Scanner)
	}
	switch v.CloudProcessor {
	case "", "local":
	default:
		return fmt.Errorf("%w: cloud_processor %q is not supported", ErrSettingsConfig, v.CloudProcessor)
	}
	return nil
}

// SMSSettings 配置短信渠道。
type SMSSettings struct {
	Enabled bool `json:"enabled"`
	// Channel 指定渠道类型："" 或 "http" 使用通用 HTTP 网关；"log" 使用日志
	// 兜底；其它值视为已加载的短信插件名（如 "smsbao"）。旧配置可省略。
	Channel  string `json:"channel"`
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	Method   string `json:"method"`
}

// ValidateSMSSettings 校验短信设置的基本约束。
func ValidateSMSSettings(v SMSSettings) error {
	if !v.Enabled {
		return nil
	}
	if strings.TrimSpace(v.Channel) == "" && strings.TrimSpace(v.Endpoint) == "" {
		return fmt.Errorf("%w: sms.endpoint or sms.channel must not be empty when enabled", ErrSettingsConfig)
	}
	return nil
}

// ValidateSMSSettingsWithPlugins 在基本校验之外，进一步确认被选为渠道的插件
// 已安装且已启用；暂停或未安装的插件会拒绝保存，避免静默回退到日志渠道。
// 插件是否已填写配置交由管理端提示（部分字段可省略），不在此拦截。
func ValidateSMSSettingsWithPlugins(v SMSSettings, pluginSvc *PluginService) error {
	if err := ValidateSMSSettings(v); err != nil {
		return err
	}
	channel := strings.TrimSpace(v.Channel)
	if !v.Enabled || channel == "" || channel == "http" || channel == "log" {
		return nil
	}
	if pluginSvc == nil {
		return fmt.Errorf("%w: sms plugin %q is unavailable", ErrSettingsConfig, channel)
	}
	r := pluginSvc.Readiness(context.Background(), channel)
	if !r.Installed {
		return fmt.Errorf("%w: sms plugin %q is not installed", ErrSettingsConfig, channel)
	}
	if !r.Enabled {
		return fmt.Errorf("%w: sms plugin %q is paused; enable it in the plugin marketplace", ErrSettingsConfig, channel)
	}
	return nil
}

// BuildSMSSender 依据短信设置与插件服务构造运行中的短信渠道。channel 为插件名
// 时使用对应插件；否则回退到通用 HTTP 网关或日志渠道。构造失败时返回日志渠道，
// 保证发送路径始终可用。
func BuildSMSSender(ctx context.Context, v SMSSettings, pluginSvc *PluginService, logger *slog.Logger) notify.Sender {
	if !v.Enabled {
		return notify.NewLogSender("sms", logger)
	}
	switch channel := strings.TrimSpace(v.Channel); {
	case channel == "log":
		return notify.NewLogSender("sms", logger)
	case channel != "" && channel != "http":
		if pluginSvc == nil {
			logger.Warn("sms plugin requested but plugins are unavailable", slog.String("channel", channel))
			return notify.NewLogSender("sms", logger)
		}
		applyCtx, cancel := context.WithTimeout(ctx, pluginApplyTimeout)
		defer cancel()
		sender, err := pluginSvc.Sender(applyCtx, channel)
		if err != nil {
			logger.Warn("failed to initialize sms plugin", slog.String("channel", channel), slog.Any("error", err))
			return notify.NewLogSender("sms", logger)
		}
		return sender
	}
	if strings.TrimSpace(v.Endpoint) != "" {
		httpSender, err := notify.NewHTTPSSender(notify.HTTPOptions{
			Name:     v.Provider,
			Endpoint: v.Endpoint,
			Method:   v.Method,
		})
		if err != nil {
			logger.Warn("failed to initialize http sms channel", slog.Any("error", err))
			return notify.NewLogSender("sms", logger)
		}
		return httpSender
	}
	return notify.NewLogSender("sms", logger)
}

// LimitsSettings 配置按调用方的速率限制（重启后生效）。
type LimitsSettings struct {
	UploadPerMinute int `json:"upload_per_minute"`
	UploadBurst     int `json:"upload_burst"`
	GuestPerMinute  int `json:"guest_per_minute"`
	GuestBurst      int `json:"guest_burst"`
	ImagePerMinute  int `json:"image_per_minute"`
	ImageBurst      int `json:"image_burst"`
	SharePerMinute  int `json:"share_per_minute"`
	ShareBurst      int `json:"share_burst"`
}

// ValidateLimitsSettings 校验限流设置。
func ValidateLimitsSettings(v LimitsSettings) error {
	values := []int{
		v.UploadPerMinute, v.UploadBurst, v.GuestPerMinute, v.GuestBurst,
		v.ImagePerMinute, v.ImageBurst, v.SharePerMinute, v.ShareBurst,
	}
	for _, n := range values {
		if n < 0 {
			return fmt.Errorf("%w: limits values must not be negative", ErrSettingsConfig)
		}
	}
	return nil
}

// MaintenanceSettings 配置后台维护任务（重启后生效）。
type MaintenanceSettings struct {
	OrphanCleanup       bool `json:"orphan_cleanup"`
	OrphanGraceHours    int  `json:"orphan_grace_hours"`
	OrphanIntervalHours int  `json:"orphan_interval_hours"`
}

// ValidateMaintenanceSettings 校验维护设置。
func ValidateMaintenanceSettings(v MaintenanceSettings) error {
	if v.OrphanGraceHours < 0 {
		return fmt.Errorf("%w: orphan_grace_hours must not be negative", ErrSettingsConfig)
	}
	if v.OrphanCleanup && v.OrphanIntervalHours < 1 {
		return fmt.Errorf("%w: orphan_interval_hours must be at least 1", ErrSettingsConfig)
	}
	return nil
}

// SiteSettings 是站点对外信息。
type SiteSettings struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ClientIPSettings 控制客户端真实 IP 的来源与可信代理。
type ClientIPSettings struct {
	Source         string   `json:"source"`
	Header         string   `json:"header"`
	TrustedProxies []string `json:"trusted_proxies"`
	XFFDepth       int      `json:"xff_depth"`
}

// ValidateClientIPSettings 校验客户端 IP 设置。
func ValidateClientIPSettings(v ClientIPSettings) error {
	switch strings.ToLower(strings.TrimSpace(v.Source)) {
	case "", "remote", "x-forwarded-for", "x-real-ip", "cf-connecting-ip", "true-client-ip", "x-client-ip", "forwarded", "custom":
	default:
		return fmt.Errorf("%w: unknown client ip source %q", ErrSettingsConfig, v.Source)
	}
	if strings.EqualFold(strings.TrimSpace(v.Source), "custom") && strings.TrimSpace(v.Header) == "" {
		return fmt.Errorf("%w: header is required when source is custom", ErrSettingsConfig)
	}
	if v.XFFDepth < 0 {
		return fmt.Errorf("%w: xff_depth must not be negative", ErrSettingsConfig)
	}
	for _, cidr := range v.TrustedProxies {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(cidr)); err != nil {
			return fmt.Errorf("%w: invalid CIDR %q", ErrSettingsConfig, cidr)
		}
	}
	return nil
}
