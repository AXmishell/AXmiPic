package service_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AXmishell/axmipic/internal/plugin"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

// fakePlugin 是用于测试的短信插件。
type fakePlugin struct {
	desc    plugin.Descriptor
	cfg     map[string]string
	invoked []string
}

func (f *fakePlugin) Descriptor() plugin.Descriptor { return f.desc }
func (f *fakePlugin) Configure(_ context.Context, cfg map[string]string) error {
	f.cfg = cfg
	return nil
}
func (f *fakePlugin) Invoke(_ context.Context, op string, _ []byte) ([]byte, error) {
	f.invoked = append(f.invoked, op)
	return []byte(`{"ok":true}`), nil
}
func (f *fakePlugin) Close(context.Context) error { return nil }

// pluginSettingKey 与 service 内部的键约定保持一致。
const pluginSettingKey = "plugin.fake"

func newPluginFixture(t *testing.T) (*service.PluginService, *fakePlugin, *store.Repository) {
	t.Helper()
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	cipher, err := secret.New([]byte("test-master-key-32bytes-minimum"))
	if err != nil {
		t.Fatalf("secret.New: %v", err)
	}
	mgr := plugin.NewManager(plugin.Options{Logger: slog.Default()})
	fake := &fakePlugin{desc: plugin.Descriptor{
		Category: plugin.CategoryNotifySMS,
		Name:     "fake",
		Title:    "测试短信",
		Fields: []plugin.Field{
			{Key: "username", Label: "账号", Required: true},
			{Key: "password", Label: "密码", Type: plugin.FieldPassword, Secret: true},
			{Key: "region", Label: "区域", Default: "cn"},
		},
	}}
	mgr.RegisterProvider(plugin.Manifest{Name: "fake", Category: plugin.CategoryNotifySMS, Runtime: plugin.RuntimeWASM}, fake)
	return service.NewPluginService(mgr, repo, cipher, slog.Default()), fake, repo
}

func TestPluginConfigMasksSecrets(t *testing.T) {
	svc, _, _ := newPluginFixture(t)
	ctx := context.Background()

	cfg, err := svc.Config(ctx, "fake")
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if cfg.Configured {
		t.Error("should not be configured yet")
	}
	if cfg.Values["region"] != "cn" {
		t.Errorf("default not applied: %v", cfg.Values)
	}
	if cfg.Secrets["password"] {
		t.Error("secret should not be set yet")
	}

	if err := svc.SaveConfig(ctx, "fake", map[string]string{
		"username": "acct", "password": "s3cr3t",
	}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	cfg, err = svc.Config(ctx, "fake")
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if !cfg.Configured {
		t.Error("should be configured")
	}
	if cfg.Values["username"] != "acct" {
		t.Errorf("username not stored: %v", cfg.Values)
	}
	if _, leaked := cfg.Values["password"]; leaked {
		t.Error("secret value must not be returned")
	}
	if !cfg.Secrets["password"] {
		t.Error("secret should be set")
	}
}

func TestPluginSecretEncryptedAtRest(t *testing.T) {
	svc, _, repo := newPluginFixture(t)
	ctx := context.Background()
	if err := svc.SaveConfig(ctx, "fake", map[string]string{"username": "acct", "password": "s3cr3t"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	raw, err := repo.GetSetting(ctx, pluginSettingKey)
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if strings.Contains(raw, "s3cr3t") {
		t.Fatalf("secret leaked to storage: %s", raw)
	}
	if !strings.Contains(raw, "acct") {
		t.Fatalf("non-secret should be stored in clear: %s", raw)
	}
	if !strings.Contains(raw, "v1:") {
		t.Fatalf("secret should be stored as versioned ciphertext: %s", raw)
	}
}

func TestPluginApplyDecrypts(t *testing.T) {
	svc, fake, _ := newPluginFixture(t)
	ctx := context.Background()
	if err := svc.SaveConfig(ctx, "fake", map[string]string{"username": "acct", "password": "s3cr3t"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := svc.Apply(ctx, "fake"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if fake.cfg["username"] != "acct" || fake.cfg["password"] != "s3cr3t" {
		t.Fatalf("plugin received wrong config: %v", fake.cfg)
	}
	if fake.cfg["region"] != "cn" {
		t.Fatalf("default region not applied: %v", fake.cfg)
	}

	// 空密码表示保持原值。
	if err := svc.SaveConfig(ctx, "fake", map[string]string{"username": "acct2"}); err != nil {
		t.Fatalf("SaveConfig keep: %v", err)
	}
	if err := svc.Apply(ctx, "fake"); err != nil {
		t.Fatalf("Apply keep: %v", err)
	}
	if fake.cfg["password"] != "s3cr3t" {
		t.Fatalf("secret should be preserved, got %q", fake.cfg["password"])
	}
	if fake.cfg["username"] != "acct2" {
		t.Fatalf("username not updated: %q", fake.cfg["username"])
	}
}

func TestPluginSenderAndTest(t *testing.T) {
	svc, fake, _ := newPluginFixture(t)
	ctx := context.Background()
	if err := svc.SaveConfig(ctx, "fake", map[string]string{"username": "acct", "password": "pw"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	sender, err := svc.Sender(ctx, "fake")
	if err != nil {
		t.Fatalf("Sender: %v", err)
	}
	if sender.Name() != "fake" {
		t.Fatalf("sender name = %q", sender.Name())
	}
	if _, err := svc.Test(ctx, "fake", "13800000000", "", "hello"); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if len(fake.invoked) == 0 || fake.invoked[len(fake.invoked)-1] != "send" {
		t.Fatalf("expected send op, got %v", fake.invoked)
	}
}

func TestPluginSetEnabledPersists(t *testing.T) {
	svc, _, repo := newPluginFixture(t)
	ctx := context.Background()

	statuses := svc.Installed(ctx)
	if len(statuses) != 1 || !statuses[0].Enabled {
		t.Fatalf("expected one enabled plugin, got %+v", statuses)
	}

	if err := svc.SetEnabled(ctx, "fake", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	statuses = svc.Installed(ctx)
	if len(statuses) != 1 || statuses[0].Enabled {
		t.Fatalf("expected disabled, got %+v", statuses)
	}
	raw, err := repo.GetSetting(ctx, "plugin.state.fake")
	if err != nil || raw != "disabled" {
		t.Fatalf("state not persisted: %q err=%v", raw, err)
	}

	if err := svc.SetEnabled(ctx, "fake", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	statuses = svc.Installed(ctx)
	if !statuses[0].Enabled {
		t.Fatalf("expected enabled, got %+v", statuses)
	}

	// 保存配置后 Configured 应为 true。
	if err := svc.SaveConfig(ctx, "fake", map[string]string{"username": "u", "password": "p"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	statuses = svc.Installed(ctx)
	if !statuses[0].Configured {
		t.Fatalf("expected configured, got %+v", statuses)
	}
}

func TestPluginNotFound(t *testing.T) {
	svc, _, _ := newPluginFixture(t)
	if _, err := svc.Config(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for missing plugin")
	}
}

func TestBuildSMSSenderChannels(t *testing.T) {
	svc, _, _ := newPluginFixture(t)
	ctx := context.Background()

	// 未启用 -> 日志渠道。
	if got := service.BuildSMSSender(ctx, service.SMSSettings{Enabled: false}, svc, slog.Default()); got.Name() != "log" {
		t.Errorf("disabled channel = %q", got.Name())
	}
	// 显式 log。
	if got := service.BuildSMSSender(ctx, service.SMSSettings{Enabled: true, Channel: "log"}, svc, slog.Default()); got.Name() != "log" {
		t.Errorf("log channel = %q", got.Name())
	}
	// 插件渠道。
	if err := svc.SaveConfig(ctx, "fake", map[string]string{"username": "a", "password": "b"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if got := service.BuildSMSSender(ctx, service.SMSSettings{Enabled: true, Channel: "fake"}, svc, slog.Default()); got.Name() != "fake" {
		t.Errorf("plugin channel = %q", got.Name())
	}
	// 通用 HTTP 网关。
	got := service.BuildSMSSender(ctx, service.SMSSettings{Enabled: true, Endpoint: "https://sms.example.com/send", Method: "POST"}, svc, slog.Default())
	if got.Name() != "http" {
		t.Errorf("http channel = %q", got.Name())
	}
}

func TestValidateSMSSettingsChannel(t *testing.T) {
	if err := service.ValidateSMSSettings(service.SMSSettings{Enabled: true, Channel: "fake"}); err != nil {
		t.Errorf("channel-only settings should be valid: %v", err)
	}
	if err := service.ValidateSMSSettings(service.SMSSettings{Enabled: true}); err == nil {
		t.Error("enabled without channel or endpoint should be invalid")
	}
	if err := service.ValidateSMSSettings(service.SMSSettings{Enabled: false}); err != nil {
		t.Errorf("disabled settings should be valid: %v", err)
	}
}
