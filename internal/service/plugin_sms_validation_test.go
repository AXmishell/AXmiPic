package service_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/AXmishell/axmipic/internal/plugin"
	"github.com/AXmishell/axmipic/internal/service"
	"github.com/AXmishell/axmipic/internal/store"
)

// newSMSPluginService 构造一个带有若干已安装（且启用）短信插件的服务。
func newSMSPluginService(t *testing.T, names ...string) (*service.PluginService, *plugin.Manager) {
	t.Helper()
	repo, err := store.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	mgr := plugin.NewManager(plugin.Options{Logger: slog.Default()})
	for _, name := range names {
		mgr.RegisterProvider(
			plugin.Manifest{Name: name, Category: plugin.CategoryNotifySMS, Runtime: plugin.RuntimeWASM},
			&fakePlugin{desc: plugin.Descriptor{
				Name:     name,
				Category: plugin.CategoryNotifySMS,
				Fields:   []plugin.Field{{Key: "username", Label: "账号"}},
			}},
		)
	}
	return service.NewPluginService(mgr, repo, nil, slog.Default()), mgr
}

func TestValidateSMSSettingsWithPlugins(t *testing.T) {
	svc, mgr := newSMSPluginService(t, "smsbao")
	ctx := context.Background()

	// 关闭短信：不校验渠道。
	if err := service.ValidateSMSSettingsWithPlugins(service.SMSSettings{Enabled: false}, svc); err != nil {
		t.Fatalf("disabled settings: %v", err)
	}
	// 通用 HTTP 网关：与插件无关。
	if err := service.ValidateSMSSettingsWithPlugins(service.SMSSettings{Enabled: true, Channel: "http", Endpoint: "https://sms.example.com/send"}, svc); err != nil {
		t.Fatalf("http channel: %v", err)
	}
	// 已安装且已启用的插件渠道：通过。
	if err := service.ValidateSMSSettingsWithPlugins(service.SMSSettings{Enabled: true, Channel: "smsbao"}, svc); err != nil {
		t.Fatalf("installed+enabled plugin: %v", err)
	}
	// 未安装的插件：拒绝。
	if err := service.ValidateSMSSettingsWithPlugins(service.SMSSettings{Enabled: true, Channel: "missing"}, svc); err == nil {
		t.Fatal("unknown plugin should be rejected")
	}
	// 已暂停的插件：拒绝。
	if err := mgr.Disable(ctx, "smsbao"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := service.ValidateSMSSettingsWithPlugins(service.SMSSettings{Enabled: true, Channel: "smsbao"}, svc); err == nil {
		t.Fatal("paused plugin should be rejected")
	}
	// 插件系统不可用：拒绝。
	if err := service.ValidateSMSSettingsWithPlugins(service.SMSSettings{Enabled: true, Channel: "smsbao"}, nil); err == nil {
		t.Fatal("nil plugin service should be rejected")
	}
}

func TestPluginReadiness(t *testing.T) {
	svc, _ := newSMSPluginService(t, "smsbao")
	ctx := context.Background()

	r := svc.Readiness(ctx, "smsbao")
	if !r.Installed || !r.Enabled || r.State != "active" {
		t.Fatalf("unexpected readiness: %+v", r)
	}
	if r.Configured {
		t.Fatal("plugin should not be configured yet")
	}
	if err := svc.SaveConfig(ctx, "smsbao", map[string]string{"username": "u"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if r := svc.Readiness(ctx, "smsbao"); !r.Configured {
		t.Fatalf("expected configured after save: %+v", r)
	}
	if r := svc.Readiness(ctx, "nope"); r.Installed {
		t.Fatalf("unknown plugin should not be installed: %+v", r)
	}
}
