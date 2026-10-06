// Package config 加载并校验 AXmiPic 配置。
package config

import (
	"fmt"
	"net"
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
	"server_host":                        "server.host",
	"server_port":                        "server.port",
	"server_base_url":                    "server.base_url",
	"server_trust_proxy":                 "server.trust_proxy",
	"server_client_ip_source":            "server.client_ip.source",
	"server_client_ip_header":            "server.client_ip.header",
	"server_client_ip_trusted_proxies":   "server.client_ip.trusted_proxies",
	"server_client_ip_xff_depth":         "server.client_ip.xff_depth",
	"server_read_timeout_sec":            "server.read_timeout_sec",
	"server_write_timeout_sec":           "server.write_timeout_sec",
	"server_shutdown_timeout_sec":        "server.shutdown_timeout_sec",
	"server_pprof_enabled":               "server.pprof_enabled",
	"database_driver":                    "database.driver",
	"database_dsn":                       "database.dsn",
	"database_max_open_conns":            "database.max_open_conns",
	"database_max_idle_conns":            "database.max_idle_conns",
	"database_conn_max_lifetime_minutes": "database.conn_max_lifetime_minutes",
	"storage_driver":                     "storage.driver",
	"storage_local_root":                 "storage.local.root",
	"s3_endpoint":                        "storage.s3.endpoint",
	"s3_region":                          "storage.s3.region",
	"s3_bucket":                          "storage.s3.bucket",
	"s3_access_key_id":                   "storage.s3.access_key_id",
	"s3_secret_access_key":               "storage.s3.secret_access_key",
	"s3_secure":                          "storage.s3.secure",
	"s3_use_path_style":                  "storage.s3.use_path_style",
	"s3_public_base_url":                 "storage.s3.public_base_url",
	"s3_presign_expiry_sec":              "storage.s3.presign_expiry_sec",
	"qiniu_access_key":                   "storage.qiniu.access_key",
	"qiniu_secret_key":                   "storage.qiniu.secret_key",
	"qiniu_bucket":                       "storage.qiniu.bucket",
	"qiniu_domain":                       "storage.qiniu.domain",
	"qiniu_upload_host":                  "storage.qiniu.upload_host",
	"qiniu_zone":                         "storage.qiniu.zone",
	"qiniu_private":                      "storage.qiniu.private",
	"qiniu_use_https":                    "storage.qiniu.use_https",
	"qiniu_presign_expiry_sec":           "storage.qiniu.presign_expiry_sec",
	"upload_max_size_mb":                 "upload.max_size_mb",
	"processing_enabled":                 "processing.enabled",
	"processing_driver":                  "processing.driver",
	"processing_max_width":               "processing.max_width",
	"processing_max_height":              "processing.max_height",
	"processing_default_quality":         "processing.default_quality",
	"processing_allow_enlarge":           "processing.allow_enlarge",
	"processing_allow_effects":           "processing.allow_effects",
	"processing_allow_watermark":         "processing.allow_watermark",
	"processing_watermark_text":          "processing.watermark_text",
	"processing_cache_mb":                "processing.cache_mb",
	"processing_max_concurrency":         "processing.max_concurrency",
	"processing_max_decode_pixels":       "processing.max_decode_pixels",
	"processing_max_render_memory_mb":    "processing.max_render_memory_mb",
	"runtime_memory_limit_mb":            "runtime.memory_limit_mb",
	"runtime_gc_percent":                 "runtime.gc_percent",
	"maintenance_orphan_cleanup":         "maintenance.orphan_cleanup",
	"maintenance_orphan_grace_hours":     "maintenance.orphan_grace_hours",
	"maintenance_orphan_interval_hours":  "maintenance.orphan_interval_hours",
	"auth_jwt_secret":                    "auth.jwt_secret",
	"auth_encryption_key":                "auth.encryption_key",
	"auth_session_ttl_hours":             "auth.session_ttl_hours",
	"auth_allow_registration":            "auth.allow_registration",
	"auth_require_auth":                  "auth.require_auth",
	"auth_default_quota_mb":              "auth.default_quota_mb",
	"auth_bootstrap_admin":               "auth.bootstrap_admin",
	"auth_allow_guest_upload":            "auth.allow_guest_upload",
	"auth_guest_quota_mb":                "auth.guest_quota_mb",
	"auth_guest_upload_max_mb":           "auth.guest_upload_max_mb",
	"auth_guest_ip_quota_mb":             "auth.guest_ip_quota_mb",
	"install_lock_file":                  "install.lock_file",
	"install_config_path":                "install.config_path",
	"install_token":                      "install.token",
	"install_disabled":                   "install.disabled",
	"limits_upload_per_minute":           "limits.upload_per_minute",
	"limits_upload_burst":                "limits.upload_burst",
	"limits_guest_per_minute":            "limits.guest_per_minute",
	"limits_guest_burst":                 "limits.guest_burst",
	"limits_image_per_minute":            "limits.image_per_minute",
	"limits_image_burst":                 "limits.image_burst",
	"limits_share_per_minute":            "limits.share_per_minute",
	"limits_share_burst":                 "limits.share_burst",
	"payment_default_gateway":            "payment.default_gateway",
	"payment_alipay_enabled":             "payment.alipay.enabled",
	"payment_alipay_gateway_url":         "payment.alipay.gateway_url",
	"payment_alipay_app_id":              "payment.alipay.app_id",
	"payment_alipay_private_key":         "payment.alipay.private_key",
	"payment_alipay_public_key":          "payment.alipay.public_key",
	"payment_wechat_enabled":             "payment.wechat.enabled",
	"payment_wechat_gateway_url":         "payment.wechat.gateway_url",
	"payment_wechat_app_id":              "payment.wechat.app_id",
	"payment_wechat_mch_id":              "payment.wechat.mch_id",
	"payment_wechat_serial_no":           "payment.wechat.serial_no",
	"payment_wechat_private_key":         "payment.wechat.private_key",
	"payment_wechat_api_v3_key":          "payment.wechat.api_v3_key",
	"payment_wechat_platform_public_key": "payment.wechat.platform_public_key",
	"payment_wechat_platform_serial_no":  "payment.wechat.platform_serial_no",
	"payment_epay_enabled":               "payment.epay.enabled",
	"payment_epay_pid":                   "payment.epay.pid",
	"payment_epay_key":                   "payment.epay.key",
	"payment_epay_gateway_url":           "payment.epay.gateway_url",
	"payment_epay_api_url":               "payment.epay.api_url",
	"payment_epay_submit_url":            "payment.epay.submit_url",
	"payment_epay_pay_type":              "payment.epay.pay_type",
	"security_scanner":                   "security.scanner",
	"security_cloud_processor":           "security.cloud_processor",
	"moderation_enabled":                 "moderation.enabled",
	"moderation_base_url":                "moderation.base_url",
	"moderation_api_key":                 "moderation.api_key",
	"moderation_model":                   "moderation.model",
	"moderation_timeout_sec":             "moderation.timeout_sec",
	"moderation_prompt":                  "moderation.prompt",
	"moderation_max_image_mb":            "moderation.max_image_mb",
	"sms_enabled":                        "sms.enabled",
	"sms_provider":                       "sms.provider",
	"sms_endpoint":                       "sms.endpoint",
	"sms_method":                         "sms.method",
	"email_enabled":                      "email.enabled",
	"email_host":                         "email.host",
	"email_port":                         "email.port",
	"email_username":                     "email.username",
	"email_password":                     "email.password",
	"email_from":                         "email.from",
	"email_use_tls":                      "email.use_tls",
	"plugins_memory_limit_mb":            "plugins.memory_limit_mb",
	"plugins_idle_unload_minutes":        "plugins.idle_unload_minutes",
	"plugins_startup":                    "plugins.startup",
	"plugins_preload":                    "plugins.preload",
	"logging_level":                      "logging.level",
}

// Config 是顶层应用配置。
type Config struct {
	Server      ServerConfig      `koanf:"server"`
	Database    DatabaseConfig    `koanf:"database"`
	Storage     StorageConfig     `koanf:"storage"`
	Upload      UploadConfig      `koanf:"upload"`
	Processing  ProcessingConfig  `koanf:"processing"`
	Auth        AuthConfig        `koanf:"auth"`
	Limits      LimitsConfig      `koanf:"limits"`
	Payment     PaymentConfig     `koanf:"payment"`
	Security    SecurityConfig    `koanf:"security"`
	Moderation  ModerationConfig  `koanf:"moderation"`
	SMS         SMSConfig         `koanf:"sms"`
	Email       EmailConfig       `koanf:"email"`
	Install     InstallConfig     `koanf:"install"`
	Maintenance MaintenanceConfig `koanf:"maintenance"`
	Plugins     PluginsConfig     `koanf:"plugins"`
	Runtime     RuntimeConfig     `koanf:"runtime"`
	Logging     LoggingConfig     `koanf:"logging"`
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
	// PprofEnabled 在管理端挂载受管理员鉴权保护的 net/http/pprof 端点
	// （/api/v1/admin/pprof/）。默认关闭。
	PprofEnabled bool `koanf:"pprof_enabled"`
	// ClientIP 控制如何解析客户端真实 IP（用于限流、访客配额与日志）。
	ClientIP ClientIPConfig `koanf:"client_ip"`
}

// ClientIPConfig 控制客户端真实 IP 的来源与可信代理。
type ClientIPConfig struct {
	// Source 选择来源：remote（默认，忽略转发头）、x-forwarded-for、x-real-ip、
	// cf-connecting-ip、true-client-ip、x-client-ip、forwarded（RFC 7239）、custom。
	Source string `koanf:"source"`
	// Header 在 Source=custom 时指定头名。
	Header string `koanf:"header"`
	// TrustedProxies 为可信代理的 CIDR 列表；仅当直接对端在其中时才解析转发头，
	// 否则回退到对端地址（防止伪造）。
	TrustedProxies []string `koanf:"trusted_proxies"`
	// XFFDepth 为 X-Forwarded-For / Forwarded 右侧跳过的可信代理数量；
	// 0 表示取右起第一个不可信地址。
	XFFDepth int `koanf:"xff_depth"`
}

// DatabaseConfig 配置元数据数据库。
type DatabaseConfig struct {
	// Driver 选择后端："sqlite"（默认）或 "postgres"。
	Driver string `koanf:"driver"`
	// DSN 对于 sqlite 是文件路径，对于 postgres 是 libpq 连接字符串/URL。
	DSN string `koanf:"dsn"`
	// MaxOpenConns 限制底层连接池的最大打开连接数；0 表示不限制。
	MaxOpenConns int `koanf:"max_open_conns"`
	// MaxIdleConns 限制空闲连接数。
	MaxIdleConns int `koanf:"max_idle_conns"`
	// ConnMaxLifetimeMinutes 为连接最长存活时间（分钟）；0 表示不限制。
	ConnMaxLifetimeMinutes int `koanf:"conn_max_lifetime_minutes"`
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
	// CacheMB 为派生图（缩略图/转换结果）进程内缓存容量（MiB）；0 表示禁用。
	CacheMB int `koanf:"cache_mb"`
	// MaxConcurrency 限制同时进行的图片渲染数量；0 表示按 CPU 核数自动设置。
	MaxConcurrency int `koanf:"max_concurrency"`
	// MaxDecodePixels 限制单张变换可解码的像素总数，用于防范解压炸弹与
	// 控制瞬时内存；0 表示使用内置默认值。
	MaxDecodePixels int `koanf:"max_decode_pixels"`
	// MaxRenderMemoryMB 为并发渲染的总内存预算（MiB）。渲染会按其预估内存
	// 占用（解码后位图 + 源字节）加权占用该预算；0 表示使用内置默认值。
	MaxRenderMemoryMB int `koanf:"max_render_memory_mb"`
}

// MaintenanceConfig 配置后台维护任务。
type MaintenanceConfig struct {
	// OrphanCleanup 启用每日孤儿对象对账（存储中存在但数据库无记录的图片
	// 对象会被删除）。
	OrphanCleanup bool `koanf:"orphan_cleanup"`
	// OrphanGraceHours 为孤儿对象的最短保留时长；早于该时长的对象才会被删除，
	// 以避免误删尚未确认的直传对象。
	OrphanGraceHours int `koanf:"orphan_grace_hours"`
	// OrphanIntervalHours 为对账任务的执行间隔。
	OrphanIntervalHours int `koanf:"orphan_interval_hours"`
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
	// GuestIPQuotaMB 为单个客户端 IP 在固定窗口内允许的访客上传总量（MiB，
	// 0 表示不限）。窗口为 24 小时。
	GuestIPQuotaMB int `koanf:"guest_ip_quota_mb"`
}

// InstallConfig 配置安装向导与锁文件。
type InstallConfig struct {
	// LockFile 为安装锁文件路径；存在即视为已安装。
	LockFile string `koanf:"lock_file"`
	// ConfigPath 为安装向导写入的配置文件路径。
	ConfigPath string `koanf:"config_path"`
	// Token 为安装向导的访问令牌。为空且未禁用安装时，服务会生成一个随机令牌
	// 并在启动日志中输出，需在安装界面填写后才能执行初始化。
	Token string `koanf:"token"`
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
	// SharePerMinute 与 ShareBurst 限制单个 IP 对分享访问接口（密码校验）的
	// 请求速率，避免暴力破解。
	SharePerMinute int `koanf:"share_per_minute"`
	ShareBurst     int `koanf:"share_burst"`
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

// ModerationConfig 配置基于标准 OpenAI 兼容接口的图片内容审查。它仅作用于
// 「开放到图片广场」的图片：审查不通过时图片保持私有。
type ModerationConfig struct {
	// Enabled 是否启用图片广场 AI 审查。
	Enabled bool `koanf:"enabled"`
	// BaseURL 为 OpenAI 兼容接口根地址（例如 https://api.openai.com/v1）。
	BaseURL string `koanf:"base_url"`
	// APIKey 为接口密钥。
	APIKey string `koanf:"api_key"`
	// Model 为视觉模型名（例如 gpt-4o-mini）。
	Model string `koanf:"model"`
	// TimeoutSec 为单次审查请求超时（秒）。
	TimeoutSec int `koanf:"timeout_sec"`
	// Prompt 覆盖默认审查提示词。
	Prompt string `koanf:"prompt"`
	// MaxImageMB 限制送审图片的最大体积（MiB）；超过时按审查不通过处理。
	MaxImageMB int `koanf:"max_image_mb"`
}

// SMSConfig 配置短信渠道。
type SMSConfig struct {
	Enabled bool `koanf:"enabled"`
	// Channel 指定渠道类型："" 或 "http" 使用通用 HTTP 网关；"log" 使用日志
	// 兜底；其它值视为运行时加载的短信插件名。
	Channel  string `koanf:"channel"`
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

// PluginsConfig 配置运行时插件系统。
type PluginsConfig struct {
	// Enabled 为 true 时扫描并加载插件目录。
	Enabled bool `koanf:"enabled"`
	// Dir 为插件根目录，每个子目录为一个插件。
	Dir string `koanf:"dir"`
	// HTTPTimeoutSec 为插件发起 HTTP 请求的超时（秒）。
	HTTPTimeoutSec int `koanf:"http_timeout_sec"`
	// MaxHTTPBodyKB 为插件单次 HTTP 请求/响应体的上限（KiB）。
	MaxHTTPBodyKB int `koanf:"max_http_body_kb"`
	// TrustedKeys 为 Ed25519 可信公钥（base64），用于校验在线安装的插件签名。
	TrustedKeys []string `koanf:"trusted_keys"`
	// RequireSignature 为 true 时，安装插件必须提供有效签名。
	RequireSignature bool `koanf:"require_signature"`
	// IndexURL 为插件市场索引（JSON）地址，供在线浏览与按名安装。
	IndexURL string `koanf:"index_url"`
	// MaxArchiveMB 为插件归档解压后的最大体积（MiB）。
	MaxArchiveMB int `koanf:"max_archive_mb"`
	// MemoryLimitMB 为每个 WASM 插件线性内存的上限（MiB）；0 表示内置默认值
	// （64 MiB）。降低它可约束单个失控插件占用的内存。
	MemoryLimitMB int `koanf:"memory_limit_mb"`
	// IdleUnloadMinutes 为已启用插件的空闲自动卸载时长（分钟）；0 表示关闭。
	// 卸载后再次调用会按需重新加载，从而在不使用时释放其内存。
	IdleUnloadMinutes int `koanf:"idle_unload_minutes"`
	// Startup 选择启动加载模式："lazy"（默认，仅登记不实例化，首次使用才加载）
	// 或 "eager"（启动即实例化所有已启用插件）。
	Startup string `koanf:"startup"`
	// Preload 为无论启动模式都立即预热的插件名列表（例如活跃的短信渠道），
	// 使其在启动后即处于可调用状态。
	Preload []string `koanf:"preload"`
}

// RuntimeConfig 配置 Go 运行时与进程内存行为。
type RuntimeConfig struct {
	// MemoryLimitMB 为进程软内存上限（MiB），映射到 GOMEMLIMIT，使 GC 在接近
	// 该上限时更积极地回收，避免 RSS 长期居高不下。0 表示不设置（保持 Go 默认）。
	MemoryLimitMB int `koanf:"memory_limit_mb"`
	// GCPercent 映射到 GOGC。50 表示每分配 1 字节存活数据触发一次 GC（更省内存、
	// 更耗 CPU）；0 表示不设置（保持 Go 默认 100）。
	GCPercent int `koanf:"gc_percent"`
}

// PaymentConfig 配置支付渠道。默认渠道需在已注册的渠道（manual、mock、
// alipay、wechat、epay）中选择。
type PaymentConfig struct {
	DefaultGateway string       `koanf:"default_gateway"`
	Alipay         AlipayConfig `koanf:"alipay"`
	Wechat         WechatConfig `koanf:"wechat"`
	Epay           EpayConfig   `koanf:"epay"`
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
	// PlatformPublicKey 为微信支付平台证书公钥（PEM 或裸 base64）；配置后
	// 对回调的 Wechatpay-Signature 做 RSA 验签（强烈建议配置）。
	PlatformPublicKey string `koanf:"platform_public_key"`
	// PlatformSerialNo 为平台证书序列号；配置后校验回调头中的 serial。
	PlatformSerialNo string `koanf:"platform_serial_no"`
}

// EpayConfig 配置易支付（彩虹易支付兼容）聚合支付。PID、密钥与网关地址齐备
// 时启用。
type EpayConfig struct {
	Enabled bool `koanf:"enabled"`
	// PID 为易支付商户号。
	PID string `koanf:"pid"`
	// Key 为商户密钥（MD5 签名）。
	Key string `koanf:"key"`
	// GatewayURL 为易支付站点根地址，例如 https://pay.example.com。
	GatewayURL string `koanf:"gateway_url"`
	// APIURL 为下单接口；留空使用 <GatewayURL>/mapi.php。
	APIURL string `koanf:"api_url"`
	// SubmitURL 为收银台地址；留空使用 <GatewayURL>/submit.php。
	SubmitURL string `koanf:"submit_url"`
	// PayType 为支付通道：alipay、wxpay、qqpay 等；留空默认 alipay。
	PayType string `koanf:"pay_type"`
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
			ClientIP:           ClientIPConfig{Source: "remote"},
		},
		Database: DatabaseConfig{
			Driver:                 "sqlite",
			DSN:                    "./data/axmipic.db",
			MaxOpenConns:           25,
			MaxIdleConns:           5,
			ConnMaxLifetimeMinutes: 30,
		},
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
			CacheMB:        64,
			MaxConcurrency: 0,
			// 默认解码上限约为 16 MP，足以覆盖常见相机照片，同时把单次渲染的
			// 瞬时内存控制在合理范围。需要处理更大原图时可调高。
			MaxDecodePixels:   16_000_000,
			MaxRenderMemoryMB: 256,
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
			SharePerMinute:  30,
			ShareBurst:      10,
		},
		Payment:  PaymentConfig{DefaultGateway: "manual"},
		Security: SecurityConfig{Scanner: "builtin", CloudProcessor: "local"},
		Moderation: ModerationConfig{
			BaseURL:    "https://api.openai.com/v1",
			Model:      "gpt-4o-mini",
			TimeoutSec: 30,
			MaxImageMB: 10,
		},
		SMS:     SMSConfig{Method: "POST"},
		Email:   EmailConfig{Port: 587},
		Install: InstallConfig{LockFile: "./data/install.lock", ConfigPath: "./configs/config.yaml"},
		Plugins: PluginsConfig{
			Enabled:           true,
			Dir:               "./plugins",
			HTTPTimeoutSec:    10,
			MaxHTTPBodyKB:     1024,
			MaxArchiveMB:      64,
			MemoryLimitMB:     64,
			IdleUnloadMinutes: 0,
			Startup:           "lazy",
		},
		Runtime: RuntimeConfig{},
		Maintenance: MaintenanceConfig{
			OrphanCleanup:       true,
			OrphanGraceHours:    72,
			OrphanIntervalHours: 24,
		},
		Logging: LoggingConfig{Level: "info"},
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
	switch strings.ToLower(strings.TrimSpace(c.Server.ClientIP.Source)) {
	case "", "remote", "x-forwarded-for", "x-real-ip", "cf-connecting-ip", "true-client-ip", "x-client-ip", "forwarded", "custom":
	default:
		return fmt.Errorf("config: server.client_ip.source %q is not supported", c.Server.ClientIP.Source)
	}
	if strings.EqualFold(strings.TrimSpace(c.Server.ClientIP.Source), "custom") &&
		strings.TrimSpace(c.Server.ClientIP.Header) == "" {
		return fmt.Errorf("config: server.client_ip.header is required when source is custom")
	}
	if c.Server.ClientIP.XFFDepth < 0 {
		return fmt.Errorf("config: server.client_ip.xff_depth must not be negative")
	}
	for _, cidr := range c.Server.ClientIP.TrustedProxies {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(cidr)); err != nil {
			return fmt.Errorf("config: server.client_ip.trusted_proxies contains invalid CIDR %q", cidr)
		}
	}
	switch strings.ToLower(strings.TrimSpace(c.Database.Driver)) {
	case "", "sqlite", "postgres", "postgresql", "pgx", "mysql", "mariadb":
	default:
		return fmt.Errorf("config: database.driver %q is not supported (want sqlite, postgres or mysql)", c.Database.Driver)
	}
	if strings.TrimSpace(c.Database.DSN) == "" {
		return fmt.Errorf("config: database.dsn must not be empty")
	}
	if c.Database.MaxOpenConns < 0 {
		return fmt.Errorf("config: database.max_open_conns must not be negative")
	}
	if c.Database.MaxIdleConns < 0 {
		return fmt.Errorf("config: database.max_idle_conns must not be negative")
	}
	if c.Database.ConnMaxLifetimeMinutes < 0 {
		return fmt.Errorf("config: database.conn_max_lifetime_minutes must not be negative")
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
	if c.Processing.CacheMB < 0 {
		return fmt.Errorf("config: processing.cache_mb must not be negative")
	}
	if c.Processing.MaxConcurrency < 0 {
		return fmt.Errorf("config: processing.max_concurrency must not be negative")
	}
	if c.Processing.MaxDecodePixels < 0 {
		return fmt.Errorf("config: processing.max_decode_pixels must not be negative")
	}
	if c.Processing.MaxRenderMemoryMB < 0 {
		return fmt.Errorf("config: processing.max_render_memory_mb must not be negative")
	}
	if c.Runtime.MemoryLimitMB < 0 {
		return fmt.Errorf("config: runtime.memory_limit_mb must not be negative")
	}
	if c.Runtime.GCPercent < 0 {
		return fmt.Errorf("config: runtime.gc_percent must not be negative")
	}
	if c.Plugins.MemoryLimitMB < 0 {
		return fmt.Errorf("config: plugins.memory_limit_mb must not be negative")
	}
	if c.Plugins.IdleUnloadMinutes < 0 {
		return fmt.Errorf("config: plugins.idle_unload_minutes must not be negative")
	}
	switch strings.ToLower(strings.TrimSpace(c.Plugins.Startup)) {
	case "", "lazy", "eager":
	default:
		return fmt.Errorf("config: plugins.startup %q is not supported (want lazy or eager)", c.Plugins.Startup)
	}
	for _, name := range c.Plugins.Preload {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return fmt.Errorf("config: plugins.preload contains an empty name")
		}
		if strings.ContainsAny(trimmed, "/\\ \t") {
			return fmt.Errorf("config: plugins.preload name %q must not contain separators or spaces", trimmed)
		}
	}
	if c.Maintenance.OrphanGraceHours < 0 {
		return fmt.Errorf("config: maintenance.orphan_grace_hours must not be negative")
	}
	if c.Maintenance.OrphanCleanup && c.Maintenance.OrphanIntervalHours < 1 {
		return fmt.Errorf("config: maintenance.orphan_interval_hours must be at least 1")
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
		c.Limits.ImagePerMinute < 0 || c.Limits.ImageBurst < 0 ||
		c.Limits.SharePerMinute < 0 || c.Limits.ShareBurst < 0 {
		return fmt.Errorf("config: limits values must not be negative")
	}
	switch c.Payment.DefaultGateway {
	case "", "manual", "mock", "alipay", "wechat", "epay":
	default:
		return fmt.Errorf("config: payment.default_gateway %q is not supported", c.Payment.DefaultGateway)
	}
	if c.Payment.Epay.Enabled {
		if strings.TrimSpace(c.Payment.Epay.PID) == "" || strings.TrimSpace(c.Payment.Epay.Key) == "" {
			return fmt.Errorf("config: payment.epay.pid and payment.epay.key must not be empty when epay is enabled")
		}
		if strings.TrimSpace(c.Payment.Epay.GatewayURL) == "" &&
			strings.TrimSpace(c.Payment.Epay.APIURL) == "" && strings.TrimSpace(c.Payment.Epay.SubmitURL) == "" {
			return fmt.Errorf("config: payment.epay.gateway_url must not be empty when epay is enabled")
		}
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
	if c.SMS.Enabled && strings.TrimSpace(c.SMS.Endpoint) == "" && strings.TrimSpace(c.SMS.Channel) == "" {
		return fmt.Errorf("config: sms.endpoint or sms.channel must not be empty when sms is enabled")
	}
	return nil
}

// envKey 将 AXMIPIC_* 变量映射到点分隔的配置路径，对于无法识别的变量返回空字符串。
func envKey(raw string) string {
	key := strings.ToLower(strings.TrimPrefix(raw, envPrefix))
	return envPaths[key]
}
