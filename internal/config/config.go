// Package config loads and validates AXmiPic configuration.
package config

import (
	"fmt"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/axmipic/axmipic/internal/imaging"
)

// envPrefix is the prefix used to override configuration through the environment.
const envPrefix = "AXMIPIC_"

// envPaths maps every env-addressable AXMIPIC_* variable to its dotted
// configuration path. Leaves are listed explicitly so underscores inside a
// field name (for example max_size_mb) are never mistaken for nesting.
var envPaths = map[string]string{
	"server_host":                 "server.host",
	"server_port":                 "server.port",
	"server_base_url":             "server.base_url",
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
	"processing_max_width":        "processing.max_width",
	"processing_max_height":       "processing.max_height",
	"processing_default_quality":  "processing.default_quality",
	"auth_jwt_secret":             "auth.jwt_secret",
	"auth_session_ttl_hours":      "auth.session_ttl_hours",
	"auth_allow_registration":     "auth.allow_registration",
	"auth_require_auth":           "auth.require_auth",
	"auth_default_quota_mb":       "auth.default_quota_mb",
	"auth_bootstrap_admin":        "auth.bootstrap_admin",
	"limits_upload_per_minute":    "limits.upload_per_minute",
	"limits_upload_burst":         "limits.upload_burst",
	"limits_guest_per_minute":     "limits.guest_per_minute",
	"limits_guest_burst":          "limits.guest_burst",
	"logging_level":               "logging.level",
}

// Config is the top-level application configuration.
type Config struct {
	Server     ServerConfig     `koanf:"server"`
	Database   DatabaseConfig   `koanf:"database"`
	Storage    StorageConfig    `koanf:"storage"`
	Upload     UploadConfig     `koanf:"upload"`
	Processing ProcessingConfig `koanf:"processing"`
	Auth       AuthConfig       `koanf:"auth"`
	Limits     LimitsConfig     `koanf:"limits"`
	Logging    LoggingConfig    `koanf:"logging"`
}

// ServerConfig configures the HTTP listener.
type ServerConfig struct {
	Host               string `koanf:"host"`
	Port               int    `koanf:"port"`
	BaseURL            string `koanf:"base_url"`
	ReadTimeoutSec     int    `koanf:"read_timeout_sec"`
	WriteTimeoutSec    int    `koanf:"write_timeout_sec"`
	ShutdownTimeoutSec int    `koanf:"shutdown_timeout_sec"`
}

// DatabaseConfig configures the metadata database.
type DatabaseConfig struct {
	Driver string `koanf:"driver"`
	DSN    string `koanf:"dsn"`
}

// StorageConfig selects and configures the object storage backend.
type StorageConfig struct {
	Driver string             `koanf:"driver"`
	Local  LocalStorageConfig `koanf:"local"`
	S3     S3Config           `koanf:"s3"`
	Qiniu  QiniuConfig        `koanf:"qiniu"`
}

// LocalStorageConfig configures the local filesystem backend.
type LocalStorageConfig struct {
	Root string `koanf:"root"`
}

// S3Config configures an S3-compatible backend.
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

// QiniuConfig configures a Qiniu Kodo backend.
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

// UploadConfig constrains accepted uploads.
type UploadConfig struct {
	MaxSizeMB        int      `koanf:"max_size_mb"`
	AllowedMIMETypes []string `koanf:"allowed_mime_types"`
}

// ProcessingConfig configures on-the-fly image transformation.
type ProcessingConfig struct {
	Enabled        bool     `koanf:"enabled"`
	MaxWidth       int      `koanf:"max_width"`
	MaxHeight      int      `koanf:"max_height"`
	DefaultQuality int      `koanf:"default_quality"`
	AllowedFormats []string `koanf:"allowed_formats"`
}

// AuthConfig configures accounts, sessions, and API tokens.
type AuthConfig struct {
	JWTSecret         string `koanf:"jwt_secret"`
	SessionTTLHours   int    `koanf:"session_ttl_hours"`
	AllowRegistration bool   `koanf:"allow_registration"`
	RequireAuth       bool   `koanf:"require_auth"`
	DefaultQuotaMB    int    `koanf:"default_quota_mb"`
	BootstrapAdmin    string `koanf:"bootstrap_admin"`
}

// LimitsConfig configures per-caller rate limits.
type LimitsConfig struct {
	UploadPerMinute int `koanf:"upload_per_minute"`
	UploadBurst     int `koanf:"upload_burst"`
	GuestPerMinute  int `koanf:"guest_per_minute"`
	GuestBurst      int `koanf:"guest_burst"`
}

// LoggingConfig configures logging.
type LoggingConfig struct {
	Level string `koanf:"level"`
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Host:               "0.0.0.0",
			Port:               8080,
			BaseURL:            "http://localhost:8080",
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
			Enabled:        true,
			MaxWidth:       4096,
			MaxHeight:      4096,
			DefaultQuality: 82,
			AllowedFormats: []string{"jpeg", "png", "gif", "webp", "avif"},
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
		},
		Logging: LoggingConfig{Level: "info"},
	}
}

// Load reads the YAML file at path, applies AXMIPIC_* environment overrides,
// and validates the result.
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
	if c.Database.Driver != "sqlite" {
		return fmt.Errorf("config: database.driver %q is not supported yet", c.Database.Driver)
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
	if c.Auth.SessionTTLHours < 1 {
		return fmt.Errorf("config: auth.session_ttl_hours must be at least 1")
	}
	if c.Auth.DefaultQuotaMB < 0 {
		return fmt.Errorf("config: auth.default_quota_mb must not be negative")
	}
	if c.Limits.UploadPerMinute < 0 || c.Limits.UploadBurst < 0 ||
		c.Limits.GuestPerMinute < 0 || c.Limits.GuestBurst < 0 {
		return fmt.Errorf("config: limits values must not be negative")
	}
	return nil
}

// envKey maps an AXMIPIC_* variable to a dotted configuration path, or returns
// an empty string for variables that are not recognized.
func envKey(raw string) string {
	key := strings.ToLower(strings.TrimPrefix(raw, envPrefix))
	return envPaths[key]
}
