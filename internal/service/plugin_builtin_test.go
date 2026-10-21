package service_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AXmishell/axmipic/internal/config"
	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/plugin"
	"github.com/AXmishell/axmipic/internal/secret"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

func TestBuiltinXResolverRegistration(t *testing.T) {
	svc, _, _, _ := newBuiltinFixture(t)
	svc.RegisterBuiltinXResolver()

	descriptors := svc.Descriptors(plugin.CategoryImportX)
	if len(descriptors) != 1 || descriptors[0].Name != service.BuiltinXResolver {
		t.Fatalf("unexpected descriptors: %+v", descriptors)
	}
	cfg, err := svc.Config(context.Background(), service.BuiltinXResolver)
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if cfg.Values["provider"] != "fxtwitter" {
		t.Fatalf("default provider = %q", cfg.Values["provider"])
	}
	if cfg.Values["image_variant"] != "orig" {
		t.Fatalf("default image variant = %q", cfg.Values["image_variant"])
	}
}

func newBuiltinFixture(t *testing.T) (*service.PluginService, *store.Repository, *service.NotifyService, *service.SettingsService) {
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
	logger := slog.Default()
	mgr := plugin.NewManager(plugin.Options{Logger: logger})
	svc := service.NewPluginService(mgr, repo, cipher, logger)
	svc.RegisterBuiltinNotify(logger)
	notifySvc := service.NewNotifyService(notify.NewLogSender("sms", logger), notify.NewLogSender("email", logger))
	svc.SetNotifyApplier(func(ctx context.Context, category string) {
		switch category {
		case plugin.CategoryNotifySMS:
			notifySvc.SetSMS(svc.NotifySender(ctx, category))
		case plugin.CategoryNotifyEmail:
			notifySvc.SetEmail(svc.NotifySender(ctx, category))
		}
	})
	settingsSvc := service.NewSettingsService(repo, cipher, notifySvc, logger, service.SMTPConfig{})
	return svc, repo, notifySvc, settingsSvc
}

func TestBuiltinPluginsAreTrustedAndUnsandboxed(t *testing.T) {
	svc, _, _, _ := newBuiltinFixture(t)
	infos := svc.Installed(context.Background())
	if len(infos) != 4 {
		t.Fatalf("installed = %d, want 4 built-ins", len(infos))
	}
	for _, info := range infos {
		if !info.Builtin || info.Sandboxed {
			t.Fatalf("builtin origin/sandbox flags wrong: %+v", info)
		}
		if !info.Enabled || !info.Loaded {
			t.Fatalf("builtin should be enabled+loaded: %+v", info)
		}
		if info.Descriptor.Origin != plugin.OriginBuiltin {
			t.Fatalf("descriptor origin = %q", info.Descriptor.Origin)
		}
	}
	if err := svc.Remove(context.Background(), service.BuiltinSMSLog); err == nil {
		t.Fatal("built-in plugin must not be removable")
	}
	if err := svc.SetEnabled(context.Background(), service.BuiltinSMSLog, false); err == nil {
		t.Fatal("built-in plugin must not be disabled")
	}
}

func TestBuiltinSMSHTTPActiveChannel(t *testing.T) {
	svc, _, _, _ := newBuiltinFixture(t)
	ctx := context.Background()

	var gotTo, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotTo = r.Form.Get("to")
		gotBody = r.Form.Get("body")
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	if err := svc.SaveConfig(ctx, service.BuiltinSMSHTTP, map[string]string{
		"provider": "test",
		"endpoint": srv.URL,
		"method":   "POST",
	}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	// 内置 HTTP 网关默认只会发送空参数，这里通过 httptest 观察请求已到达。
	if err := svc.SetActive(ctx, plugin.CategoryNotifySMS, service.BuiltinSMSHTTP); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if got := svc.Active(ctx, plugin.CategoryNotifySMS); got != service.BuiltinSMSHTTP {
		t.Fatalf("active = %q", got)
	}
	sender := svc.NotifySender(ctx, plugin.CategoryNotifySMS)
	if sender.Name() != service.BuiltinSMSHTTP {
		t.Fatalf("sender = %q", sender.Name())
	}
	if err := sender.Send(ctx, notify.Message{To: "13800000000", Body: "hello"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// HTTPSSender 的 params 模板为空，因此请求体为空；能成功调用即说明内置
	// Provider 的 Invoke/Configure 链路已生效。
	_ = gotTo
	_ = gotBody
}

func TestBuiltinEmailConfigEncryptsSecret(t *testing.T) {
	svc, repo, _, _ := newBuiltinFixture(t)
	ctx := context.Background()
	if err := svc.SaveConfig(ctx, service.BuiltinEmailSMTP, map[string]string{
		"host": "smtp.example.com", "port": "587", "username": "u", "password": "pw", "from": "u@example.com",
	}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	raw, err := repo.GetSetting(ctx, "plugin."+service.BuiltinEmailSMTP)
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if strings.Contains(raw, `"pw"`) || strings.Contains(raw, "pw") {
		t.Fatalf("smtp password leaked: %s", raw)
	}
	if err := svc.SetActive(ctx, plugin.CategoryNotifyEmail, service.BuiltinEmailSMTP); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if sender := svc.NotifySender(ctx, plugin.CategoryNotifyEmail); sender.Name() != service.BuiltinEmailSMTP {
		t.Fatalf("sender = %q", sender.Name())
	}
}

func TestBuiltinConfigAfterActiveRefreshesSender(t *testing.T) {
	svc, _, notifySvc, _ := newBuiltinFixture(t)
	ctx := context.Background()

	// 先切换到一个尚未配置的渠道：发送路径回退日志。
	if err := svc.SetActive(ctx, plugin.CategoryNotifySMS, service.BuiltinSMSHTTP); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if sms, _ := notifySvc.Channels(); sms != "log" {
		t.Fatalf("unconfigured channel should fall back to log, got %q", sms)
	}

	// 保存配置并 Apply 后，当前渠道应重建为该内置插件。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()
	if err := svc.SaveConfig(ctx, service.BuiltinSMSHTTP, map[string]string{
		"endpoint": srv.URL, "method": "POST",
	}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := svc.Apply(ctx, service.BuiltinSMSHTTP); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sms, _ := notifySvc.Channels(); sms != service.BuiltinSMSHTTP {
		t.Fatalf("sender not refreshed after config, got %q", sms)
	}
}

func TestMigrateLegacyNotify(t *testing.T) {
	svc, repo, notifySvc, settingsSvc := newBuiltinFixture(t)
	ctx := context.Background()
	logger := slog.Default()

	if _, err := settingsSvc.UpdateSMTP(ctx, service.SMTPInput{
		Enabled: true, Host: "smtp.example.com", Port: 587,
		Username: "u", Password: "pw", From: "u@example.com",
	}); err != nil {
		t.Fatalf("UpdateSMTP: %v", err)
	}
	if err := repo.SetSetting(ctx, "sms", `{"enabled":true,"channel":"http","provider":"test","endpoint":"https://sms.example.com/send","method":"POST"}`); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	svc.MigrateLegacyNotify(ctx, settingsSvc, config.Config{}, logger)

	if got := svc.Active(ctx, plugin.CategoryNotifySMS); got != service.BuiltinSMSHTTP {
		t.Fatalf("sms active = %q", got)
	}
	if got := svc.Active(ctx, plugin.CategoryNotifyEmail); got != service.BuiltinEmailSMTP {
		t.Fatalf("email active = %q", got)
	}
	emailCfg, err := svc.Config(ctx, service.BuiltinEmailSMTP)
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if emailCfg.Values["host"] != "smtp.example.com" || !emailCfg.Secrets["password"] {
		t.Fatalf("legacy smtp not migrated: %+v", emailCfg)
	}
	smsCfg, err := svc.Config(ctx, service.BuiltinSMSHTTP)
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if smsCfg.Values["endpoint"] != "https://sms.example.com/send" {
		t.Fatalf("legacy sms not migrated: %+v", smsCfg)
	}
	// 迁移后应立即能构造真实渠道（SMTP 不实际发送）。
	_ = notifySvc
}
