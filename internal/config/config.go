// Package config 加载并校验 AXmiPic 配置。
package config

import (
	"fmt"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/AXmishell/axmipic/internal/imaging"
)

// envPrefix 是用于通过环境变量覆盖配置的前缀。
const envPrefix = "AXMIPIC_"

// envPaths 将每个可通过环境变量设置的 AXMIPIC_* 变量映射到其点分隔的配置路径。
// 叶子节点被显式列出，因此字段名中的下划线（例如 max_size_mb）不会被误认为嵌套。
var envPaths = map[string]string{
	"server_host":                 "server.host",
	"server_port":                 "server.port",
	"server_base_url":             "server.base_url",
	"server_trust_proxy":          "server.trust_proxy",
	"server_read_timeout_sec":     "server.read_timeout_sec",
	"server_write_timeout_sec":    "server.write_timeout_sec",
	"server_shutdown_timeout_sec": "server.shutdown_timeout_sec",
	"database_driver":             "database.driver",
	"database_dsn":                "database.dsn",
	"storage_driver":              "storage.driver",
	"storage_local_root":          "storage.local.root",
	"s3_endpoint":                 "storage.s3.endpoint",
	"s3_region":                   "storage.s3.region",
	"s3_bucket":                   "storage.s3.bucket",
	"s3_access_key_id":            "storage.s3.access_key_id",
	"s3_secret_access_key":        "storage.s3.secret_access_key",
	"s3_secure":                   "storage.s3.secure",
	"s3_use_path_style":           "storage.s3.use_path_style",
	"s3_public_base_url":          "storage.s3.public_base_url",
	"s3_presign_expiry_sec":       "storage.s3.presign_expiry_sec",
	"qiniu_access_key":            "storage.qiniu.access_key",
	"qiniu_secret_key":            "storage.qiniu.secret_key",
	"qiniu_bucket":                "storage.qiniu.bucket",
	"qiniu_domain":                "storage.qiniu.domain",
	"qiniu_upload_host":           "storage.qiniu.upload_host",
	"qiniu_zone":                  "storage.qiniu.zone",
	"qiniu_private":               "storage.qiniu.private",
	"qiniu_use_https":             "storage.qiniu.use_https",
	"qiniu_presign_expiry_sec":    "storage.qiniu.presign_expiry_sec",
	"upload_max_size_mb":          "upload.max_size_mb",
	"processing_enabled":          "processing.enabled",
	"processing_driver":           "processing.driver",
	"processing_max_width":        "processing.max_width",
	"processing_max_height":       "processing.max_height",
	"processing_default_quality":  "processing.default_quality",
	"processing_allow_enlarge":    "processing.allow_enlarge",
	"processing_allow_effects":    "processing.allow_effects",
	"processing_allow_watermark":  "processing.allow_watermark",
	"processing_watermark_text":   "processing.watermark_text",
	"auth_jwt_secret":             "auth.jwt_secret",
	"auth_encryption_key":         "auth.encryption_key",
	"auth_session_ttl_hours":      "auth.session_ttl_hours",
	"auth_allow_registration":     "auth.allow_registration",
	"auth_require_auth":           "auth.require_auth",
	"auth_default_quota_mb":       "auth.default_quota_mb",
	"auth_bootstrap_admin":        "auth.bootstrap_admin",
	"auth_allow_guest_upload":     "auth.allow_guest_upload",
	"auth_guest_quota_mb":         "auth.guest_quota_mb",
	"auth_guest_upload_max_mb":    "auth.guest_upload_max_mb",
	"install_lock_file":           "install.lock_file",
	"install_config_path":         "install.config_path",
	"install_disabled":            "install.disabled",
	"limits_upload_per_minute":    "limits.upload_per_minute",
	"limits_upload_burst":         "limits.upload_burst",
	"limits_guest_per_minute":     "limits.guest_per_minute",
	"limits_guest_burst":          "limits.guest_burst",
	"limits_image_per_minute":     "limits.image_per_minute",
	"limits_image_burst":          "limits.image_burst",
	"payment_default_gateway":     "payment.default_gateway",
	"payment_alipay_enabled":      "payment.alipay.enabled",
	"payment_alipay_gateway_url":  "payment.alipay.gateway_url",
	"payment_alipay_app_id":       "payment.alipay.app_id",
	"payment_alipay_private_key":  "payment.alipay.private_key",
	"payment_alipay_public_key":   "payment.alipay.public_key",
	"payment_wechat_enabled":      "payment.wechat.enabled",
	"payment_wechat_gateway_url":  "payment.wechat.gateway_url",
	"payment_wechat_app_id":       "payment.wechat.app_id",
	"payment_wechat_mch_id":       "payment.wechat.mch_id",
	"payment_wechat_serial_no":    "payment.wechat.serial_no",
	"payment_wechat_private_key":  "payment.wechat.private_key",
	"payment_wechat_api_v3_key":   "payment.wechat.api_v3_key",
	"security_scanner":            "security.scanner",
	"security_cloud_processor":    "security.cloud_processor",
	"sms_enabled":                 "sms.enabled",
	"sms_provider":                "sms.provider",
	"sms_endpoint":                "sms.endpoint",
	"sms_method":                  "sms.method",
	"email_enabled":               "email.enabled",
	"email_host":                  "email.host",
	"email_port":                  "email.port",
	"email_username":              "email.username",
	"email_password":              "email.password",
	"email_from":                  "email.from",
	"email_use_tls":               "email.use_tls",
	"logging_level":               "logging.level",
}

// Config 是顶层应用配置。
type Config struct {
	Server     ServerConfig     `koanf:"server"`
	Database   DatabaseConfig   `koanf:"database"`
	Storage    StorageConfig    `koanf:"storage"`
	Upload     UploadConfig     `koanf:"upload"`
	Processing ProcessingConfig `koanf:"processing"`
	Auth       AuthConfig       `koanf:"auth"`
	Limits     LimitsConfig     `koanf:"limits"`
	Payment    PaymentConfig    `koanf:"payment"`
	Security   SecurityConfig   `koanf:"security"`
	SMS        SMSConfig        `koanf:"sms"`
	Email      EmailConfig      `koanf:"email"`
	Install    InstallConfig    `koanf:"install"`
	Logging    LoggingConfig    `koanf:"logging"`
}

// ServerConfig 配置 HTTP 监听器。
type ServerConfig struct {
	Host               string `koanf:"host"`
	Port               int    `koanf:"port"`
	BaseURL            string `koanf:"base_url"`
	TrustProxy         bool   `koanf:"trust_proxy"`
	ReadTimeoutSec     int    `koanf:"read_timeout_sec"`
	WriteTimeoutSec    int    `koanf:"write_timeout_sec"`
	ShutdownTimeoutSec int    `koanf:"shutdown_timeout_sec"`
}

// DatabaseConfig 配置元数据数据库。
type DatabaseConfig struct {
	// Driver 选择后端："sqlite"（默认）或 "postgres"。
	Driver string `koanf:"driver"`
	// DSN 对于 sqlite 是文件路径，对于 postgres 是 libpq 连接字符串/URL。
	DSN string `koanf:"dsn"`
}

// StorageConfig 选择并配置对象存储后端。
type StorageConfig struct {
	Driver string             `koanf:"driver"`
	Local  LocalStorageConfig `koanf:"local"`
	S3     S3Config           `koanf:"s3"`
	Qiniu  QiniuConfig        `koanf:"qiniu"`
}

// LocalStorageConfig 配置本地文件系统后端。
type LocalStorageConfig struct {
	Root string `koanf:"root"`
}

// S3Config 配置兼容 S3 的后端。
type S3Config struct {
	Endpoint         string `koanf:"endpoint"`
	Region           string `koanf:"region"`
	Bucket           string `koanf:"bucket"`
	AccessKeyID      string `koanf:"access_key_id"`
	SecretAccessKey  string `koanf:"secret_access_key"`
	Secure           bool   `koanf:"secure"`
	UsePathStyle     bool   `koanf:"use_path_style"`
	PublicBaseURL    string `koanf:"public_base_url"`
	PresignExpirySec int    `koanf:"presign_expiry_sec"`
}

// QiniuConfig 配置七牛云 Kodo 后端。
type QiniuConfig struct {
	AccessKey        string `koanf:"access_key"`
	SecretKey        string `koanf:"secret_key"`
	Bucket           string `koanf:"bucket"`
	Domain           string `koanf:"domain"`
	UploadHost       string `koanf:"upload_host"`
	Zone             string `koanf:"zone"`
	Private          bool   `koanf:"private"`
	UseHTTPS         bool   `koanf:"use_https"`
	PresignExpirySec int    `koanf:"presign_expiry_sec"`
}

// UploadConfig 约束可接受的上传。
type UploadConfig struct {
	MaxSizeMB        int      `koanf:"max_size_mb"`
	AllowedMIMETypes []string `koanf:"allowed_mime_types"`
}

// ProcessingConfig 配置即时图像变换。
type ProcessingConfig struct {
	// Driver 选择处理器：purego（默认）/ libvips / magick。不可用时回退默认。
	Driver         string   `koanf:"driver"`
	Enabled        bool     `koanf:"enabled"`
	MaxWidth       int      `koanf:"max_width"`
	MaxHeight      int      `koanf:"max_height"`
	DefaultQuality int      `koanf:"default_quality"`
	AllowedFormats []string `koanf:"allowed_formats"`
	// AllowEnlarge 允许放大图像；默认 false。
	AllowEnlarge bool `koanf:"allow_enlarge"`
	// AllowEffects 允许灰度/模糊/锐化等滤镜；默认 true。
	AllowEffects bool `koanf:"allow_effects"`
	// AllowWatermark 允许通过 URL 叠加文字水印；默认 true。
	AllowWatermark bool `koanf:"allow_watermark"`
	// WatermarkText 为强制水印文字；非空时所有变换结果都会叠加。
	WatermarkText string `koanf:"watermark_text"`
}

// AuthConfig 配置账户、会话和 API 令牌。
type AuthConfig struct {
	JWTSecret string `koanf:"jwt_secret"`
	// EncryptionKey 是可选的独立主密钥，仅用于加密存储后端密钥。留空时
	// 回退使用 jwt_secret；两者都留空时会生成并持久化一个密钥文件，从而
	// 保证重启后仍能解密，避免存储后端配置失效。
	EncryptionKey     string `koanf:"encryption_key"`
	SessionTTLHours   int    `koanf:"session_ttl_hours"`
	AllowRegistration bool   `koanf:"allow_registration"`
	RequireAuth       bool   `koanf:"require_auth"`
	DefaultQuotaMB    int    `koanf:"default_quota_mb"`
	BootstrapAdmin    string `koanf:"bootstrap_admin"`
	// AllowGuestUpload 允许未登录访客上传（使用 Guest 角色策略）。
	AllowGuestUpload bool `koanf:"allow_guest_upload"`
	// GuestQuotaMB 为 Guest 角色的存储配额（0 表示不限）。
	GuestQuotaMB int `koanf:"guest_quota_mb"`
	// GuestUploadMaxMB 为 Guest 单文件大小上限。
	GuestUploadMaxMB int `koanf:"guest_upload_max_mb"`
}

// InstallConfig 配置安装向导与锁文件。
type InstallConfig struct {
	// LockFile 为安装锁文件路径；存在即视为已安装。
	LockFile string `koanf:"lock_file"`
	// ConfigPath 为安装向导写入的配置文件路径。
	ConfigPath string `koanf:"config_path"`
	// Disabled 为 true 时跳过安装检查（用于测试或容器编排）。
	Disabled bool `koanf:"disabled"`
}

// LimitsConfig 配置按调用方的速率限制。
type LimitsConfig struct {
	UploadPerMinute int `koanf:"upload_per_minute"`
	UploadBurst     int `koanf:"upload_burst"`
	GuestPerMinute  int `koanf:"guest_per_minute"`
	GuestBurst      int `koanf:"guest_burst"`
	ImagePerMinute  int `koanf:"image_per_minute"`
	ImageBurst      int `koanf:"image_burst"`
}

// LoggingConfig 配置日志。
type LoggingConfig struct {
	Level string `koanf:"level"`
}

// SecurityConfig 配置上传内容安全扫描与云处理。
type SecurityConfig struct {
	// Scanner 选择扫描器：none（默认，放行）、builtin（白名单+魔数检测）。
	Scanner string `koanf:"scanner"`
	// CloudProcessor 选择云处理器：local（默认，使用本地成像）。
	CloudProcessor string `koanf:"cloud_processor"`
}

// SMSConfig 配置短信渠道。
type SMSConfig struct {
	Enabled  bool   `koanf:"enabled"`
	Provider string `koanf:"provider"`
	// Endpoint 为通用 HTTP 短信网关地址。
	Endpoint string `koanf:"endpoint"`
	// Method 为 GET 或 POST。
	Method string `koanf:"method"`
}

// EmailConfig 配置邮件渠道。
type EmailConfig struct {
	Enabled  bool   `koanf:"enabled"`
	Host     string `koanf:"host"`
	Port     int    `koanf:"port"`
	Username string `koanf:"username"`
	Password string `koanf:"password"`
	From     string `koanf:"from"`
	UseTLS   bool   `koanf:"use_tls"`
}

// PaymentConfig 配置支付渠道。默认渠道需在已注册的渠道（manual、mock、
// alipay、wechat）中选择。
type PaymentConfig struct {
	DefaultGateway string       `koanf:"default_gateway"`
	Alipay         AlipayConfig `koanf:"alipay"`
	Wechat         WechatConfig `koanf:"wechat"`
}

// AlipayConfig 配置支付宝当面付（扫码支付）。凭据齐备（AppID、应用私钥、
// 支付宝公钥）时启用。
type AlipayConfig struct {
	Enabled bool `koanf:"enabled"`
	// GatewayURL 为支付宝网关地址；留空使用生产地址。
	GatewayURL string `koanf:"gateway_url"`
	AppID      string `koanf:"app_id"`
	// PrivateKey 为应用私钥（PKCS1/PKCS8，PEM 或裸 base64）。
	PrivateKey string `koanf:"private_key"`
	// PublicKey 为支付宝公钥，用于校验回调签名。
	PublicKey string `koanf:"public_key"`
}

// WechatConfig 配置微信支付 v3。凭据齐备（商户号、证书序列号、商户私钥、
// APIv3 密钥）时启用。
type WechatConfig struct {
	Enabled bool `koanf:"enabled"`
	// GatewayURL 为微信支付 API 基础地址；留空使用生产地址。
	GatewayURL string `koanf:"gateway_url"`
	AppID      string `koanf:"app_id"`
	MchID      string `koanf:"mch_id"`
	// SerialNo 为商户 API 证书序列号。
	SerialNo string `koanf:"serial_no"`
	// PrivateKey 为商户 API 私钥（PEM 或裸 base64）。
	PrivateKey string `koanf:"private_key"`
	// APIv3Key 用于解密回调中的敏感信息与校验回调签名。
	APIv3Key string `koanf:"api_v3_key"`
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Host:               "0.0.0.0",
			Port:               8080,
			BaseURL:            "http://localhost:8080",
			TrustProxy:         false,
			ReadTimeoutSec:     30,
			WriteTimeoutSec:    30,
			ShutdownTimeoutSec: 10,
		},
		Database: DatabaseConfig{Driver: "sqlite", DSN: "./data/axmipic.db"},
		Storage: StorageConfig{
			Driver: "local",
			Local:  LocalStorageConfig{Root: "./data/uploads"},
			S3:     S3Config{Region: "us-east-1", Secure: true, PresignExpirySec: 900},
			Qiniu:  QiniuConfig{UseHTTPS: true, PresignExpirySec: 3600},
		},
		Upload: UploadConfig{
			MaxSizeMB:        20,
			AllowedMIMETypes: []string{"image/jpeg", "image/png", "image/gif", "image/webp"},
		},
		Processing: ProcessingConfig{
			Driver:         "purego",
			Enabled:        true,
			MaxWidth:       4096,
			MaxHeight:      4096,
			DefaultQuality: 82,
			AllowedFormats: []string{"jpeg", "png", "gif", "webp", "avif"},
			AllowEnlarge:   false,
			AllowEffects:   true,
			AllowWatermark: true,
		},
		Auth: AuthConfig{
			SessionTTLHours:   24,
			AllowRegistration: true,
			RequireAuth:       true,
			DefaultQuotaMB:    1024,
		},
		Limits: LimitsConfig{
			UploadPerMinute: 30,
			UploadBurst:     5,
			GuestPerMinute:  6,
			GuestBurst:      2,
			ImagePerMinute:  600,
			ImageBurst:      120,
		},
		Payment:  PaymentConfig{DefaultGateway: "manual"},
		Security: SecurityConfig{Scanner: "builtin", CloudProcessor: "local"},
		SMS:      SMSConfig{Method: "POST"},
		Email:    EmailConfig{Port: 587},
		Install:  InstallConfig{LockFile: "./data/install.lock", ConfigPath: "./configs/config.yaml"},
		Logging:  LoggingConfig{Level: "info"},
	}
}

// Load 读取 path 处的 YAML 文件，应用 AXMIPIC_* 环境变量覆盖，并校验结果。
func Load(path string) (Config, error) {
	k := koanf.New(".")

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return Config{}, fmt.Errorf("config: load file %q: %w", path, err)
	}
	if err := k.Load(env.Provider(envPrefix, ".", envKey), nil); err != nil {
		return Config{}, fmt.Errorf("config: load environment overrides: %w", err)
	}

	cfg := defaultConfig()
	decoderConfig := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           &cfg,
		TagName:          "koanf",
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	}
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{DecoderConfig: decoderConfig}); err != nil {
		return Config{}, fmt.Errorf("config: decode: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("config: server.port %d out of range", c.Server.Port)
	}
	if strings.TrimSpace(c.Server.BaseURL) == "" {
		return fmt.Errorf("config: server.base_url must not be empty")
	}
	switch strings.ToLower(strings.TrimSpace(c.Database.Driver)) {
	case "", "sqlite", "postgres", "postgresql", "pgx":
	default:
		return fmt.Errorf("config: database.driver %q is not supported (want sqlite or postgres)", c.Database.Driver)
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		return fmt.Errorf("config: database.dsn must not be empty")
	}
	switch c.Storage.Driver {
	case "local":
		if strings.TrimSpace(c.Storage.Local.Root) == "" {
			return fmt.Errorf("config: storage.local.root must not be empty")
		}
	case "s3":
		if strings.TrimSpace(c.Storage.S3.Endpoint) == "" {
			return fmt.Errorf("config: storage.s3.endpoint must not be empty")
		}
		if strings.TrimSpace(c.Storage.S3.Bucket) == "" {
			return fmt.Errorf("config: storage.s3.bucket must not be empty")
		}
		if c.Storage.S3.PresignExpirySec < 1 {
			return fmt.Errorf("config: storage.s3.presign_expiry_sec must be at least 1")
		}
	case "qiniu":
		if strings.TrimSpace(c.Storage.Qiniu.Bucket) == "" {
			return fmt.Errorf("config: storage.qiniu.bucket must not be empty")
		}
		if strings.TrimSpace(c.Storage.Qiniu.Domain) == "" {
			return fmt.Errorf("config: storage.qiniu.domain must not be empty")
		}
		if strings.TrimSpace(c.Storage.Qiniu.AccessKey) == "" || strings.TrimSpace(c.Storage.Qiniu.SecretKey) == "" {
			return fmt.Errorf("config: storage.qiniu.access_key and secret_key must not be empty")
		}
		if c.Storage.Qiniu.PresignExpirySec < 1 {
			return fmt.Errorf("config: storage.qiniu.presign_expiry_sec must be at least 1")
		}
	default:
		return fmt.Errorf("config: storage.driver %q is not supported", c.Storage.Driver)
	}
	if c.Upload.MaxSizeMB < 1 {
		return fmt.Errorf("config: upload.max_size_mb must be at least 1")
	}
	if len(c.Upload.AllowedMIMETypes) == 0 {
		return fmt.Errorf("config: upload.allowed_mime_types must not be empty")
	}
	if c.Processing.Enabled {
		if c.Processing.MaxWidth < 1 || c.Processing.MaxHeight < 1 {
			return fmt.Errorf("config: processing max dimensions must be at least 1")
		}
		if c.Processing.DefaultQuality < 1 || c.Processing.DefaultQuality > 100 {
			return fmt.Errorf("config: processing.default_quality must be between 1 and 100")
		}
		if len(c.Processing.AllowedFormats) == 0 {
			return fmt.Errorf("config: processing.allowed_formats must not be empty")
		}
		for _, name := range c.Processing.AllowedFormats {
			if _, ok := imaging.ParseFormat(name); !ok {
				return fmt.Errorf("config: processing.allowed_formats contains unknown format %q", name)
			}
		}
	}
	switch strings.ToLower(strings.TrimSpace(c.Processing.Driver)) {
	case "", "purego", "libvips", "magick":
	default:
		return fmt.Errorf("config: processing.driver %q is not supported", c.Processing.Driver)
	}
	if c.Auth.SessionTTLHours < 1 {
		return fmt.Errorf("config: auth.session_ttl_hours must be at least 1")
	}
	if c.Auth.DefaultQuotaMB < 0 {
		return fmt.Errorf("config: auth.default_quota_mb must not be negative")
	}
	if c.Limits.UploadPerMinute < 0 || c.Limits.UploadBurst < 0 ||
		c.Limits.GuestPerMinute < 0 || c.Limits.GuestBurst < 0 ||
		c.Limits.ImagePerMinute < 0 || c.Limits.ImageBurst < 0 {
		return fmt.Errorf("config: limits values must not be negative")
	}
	switch c.Payment.DefaultGateway {
	case "", "manual", "mock", "alipay", "wechat":
	default:
		return fmt.Errorf("config: payment.default_gateway %q is not supported", c.Payment.DefaultGateway)
	}
	switch c.Security.Scanner {
	case "", "none", "builtin":
	default:
		return fmt.Errorf("config: security.scanner %q is not supported", c.Security.Scanner)
	}
	switch c.Security.CloudProcessor {
	case "", "local":
	default:
		return fmt.Errorf("config: security.cloud_processor %q is not supported", c.Security.CloudProcessor)
	}
	if c.Email.Enabled && strings.TrimSpace(c.Email.Host) == "" {
		return fmt.Errorf("config: email.host must not be empty when email is enabled")
	}
	if c.SMS.Enabled && strings.TrimSpace(c.SMS.Endpoint) == "" {
		return fmt.Errorf("config: sms.endpoint must not be empty when sms is enabled")
	}
	return nil
}

// envKey 将 AXMIPIC_* 变量映射到点分隔的配置路径，对于无法识别的变量返回空字符串。
func envKey(raw string) string {
	key := strings.ToLower(strings.TrimPrefix(raw, envPrefix))
	return envPaths[key]
}
