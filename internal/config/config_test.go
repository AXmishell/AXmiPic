package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AXmishell/axmipic/internal/config"
)

// writeConfig 将 YAML 内容写入临时文件并返回路径。
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, "{}"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 8080 || cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("unexpected server defaults: %+v", cfg.Server)
	}
	if cfg.Database.Driver != "sqlite" || cfg.Database.DSN == "" {
		t.Fatalf("unexpected database defaults: %+v", cfg.Database)
	}
	if cfg.Storage.Driver != "local" || cfg.Storage.Local.Root == "" {
		t.Fatalf("unexpected storage defaults: %+v", cfg.Storage)
	}
	if cfg.Upload.MaxSizeMB != 20 || len(cfg.Upload.AllowedMIMETypes) != 4 {
		t.Fatalf("unexpected upload defaults: %+v", cfg.Upload)
	}
	if cfg.Auth.RequireAuth != true || cfg.Auth.SessionTTLHours != 24 {
		t.Fatalf("unexpected auth defaults: %+v", cfg.Auth)
	}
}

func TestLoadFileOverrides(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, `
server:
  port: 9000
  base_url: "https://img.example.com"
storage:
  driver: local
  local:
    root: "/srv/uploads"
auth:
  encryption_key: "file-key"
  require_auth: false
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9000 || cfg.Server.BaseURL != "https://img.example.com" {
		t.Fatalf("server not overridden: %+v", cfg.Server)
	}
	if cfg.Storage.Local.Root != "/srv/uploads" {
		t.Fatalf("storage not overridden: %+v", cfg.Storage.Local)
	}
	if cfg.Auth.EncryptionKey != "file-key" || cfg.Auth.RequireAuth {
		t.Fatalf("auth not overridden: %+v", cfg.Auth)
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	t.Setenv("AXMIPIC_SERVER_PORT", "9100")
	t.Setenv("AXMIPIC_AUTH_ENCRYPTION_KEY", "env-key")
	t.Setenv("AXMIPIC_UPLOAD_MAX_SIZE_MB", "42")

	cfg, err := config.Load(writeConfig(t, "{}"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9100 {
		t.Fatalf("port = %d, want 9100", cfg.Server.Port)
	}
	if cfg.Auth.EncryptionKey != "env-key" {
		t.Fatalf("encryption key = %q, want env-key", cfg.Auth.EncryptionKey)
	}
	if cfg.Upload.MaxSizeMB != 42 {
		t.Fatalf("max size = %d, want 42", cfg.Upload.MaxSizeMB)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := config.Load(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestValidateRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"port range", "server:\n  port: 0\n"},
		{"empty base url", "server:\n  base_url: \"   \"\n"},
		{"unknown database driver", "database:\n  driver: mysql\n"},
		{"empty dsn", "database:\n  dsn: \"\"\n"},
		{"unknown storage driver", "storage:\n  driver: ftp\n"},
		{"empty mime types", "upload:\n  allowed_mime_types: []\n"},
		{"bad quality", "processing:\n  default_quality: 200\n"},
		{"unknown format", "processing:\n  allowed_formats: [\"bmp\"]\n"},
		{"zero session ttl", "auth:\n  session_ttl_hours: 0\n"},
		{"negative quota", "auth:\n  default_quota_mb: -1\n"},
		{"negative limit", "limits:\n  upload_burst: -1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := config.Load(writeConfig(t, tc.body)); err == nil {
				t.Fatalf("expected validation error for %q", tc.body)
			}
		})
	}
}
