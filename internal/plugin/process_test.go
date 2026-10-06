package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const processManifest = `
name: process-webhook
version: 0.1.0
category: notify.sms
runtime: process
entry: plugin-bin
abi: 1
`

func TestProcessPluginEndToEnd(t *testing.T) {
	requireBinary(t, "process-webhook")

	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := installProcess(t, "process-webhook", processManifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}

	entry, ok := mgr.Registry().Lookup(CategoryNotifySMS, "process-webhook")
	if !ok {
		t.Fatal("process plugin not registered")
	}
	if entry.Descriptor.Title == "" || len(entry.Descriptor.Fields) == 0 {
		t.Fatalf("unexpected descriptor: %+v", entry.Descriptor)
	}

	if err := mgr.Configure(ctx, "process-webhook", map[string]string{
		"url": srv.URL, "token": "secret-token",
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}

	input, _ := json.Marshal(map[string]string{"to": "13800000000", "body": "hello-process"})
	out, err := mgr.Invoke(ctx, CategoryNotifySMS, "process-webhook", "send", input)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(string(out), `"ok":true`) {
		t.Fatalf("unexpected result: %s", out)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if gotBody["to"] != "13800000000" || gotBody["body"] != "hello-process" {
		t.Errorf("unexpected body: %v", gotBody)
	}
}

func TestProcessPluginErrorPropagates(t *testing.T) {
	requireBinary(t, "process-webhook")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	dir := installProcess(t, "process-webhook", processManifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Configure(ctx, "process-webhook", map[string]string{"url": srv.URL}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	input, _ := json.Marshal(map[string]string{"to": "13800000000", "body": "x"})
	if _, err := mgr.Invoke(ctx, CategoryNotifySMS, "process-webhook", "send", input); err == nil {
		t.Fatal("expected error from failing webhook")
	}
}

func TestProcessPluginReload(t *testing.T) {
	requireBinary(t, "process-webhook")
	dir := installProcess(t, "process-webhook", processManifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Reload(ctx, "process-webhook"); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "process-webhook"); !ok {
		t.Fatal("plugin missing after reload")
	}
	if err := mgr.Unload(ctx, "process-webhook"); err != nil {
		t.Fatalf("unload: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "process-webhook"); ok {
		t.Fatal("plugin still registered after unload")
	}
}

func TestProcessPluginRelativePluginDir(t *testing.T) {
	requireBinary(t, "process-webhook")
	base := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(base); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	if err := os.MkdirAll(filepath.Join("plugins", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exampleBin["process-webhook"])
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join("plugins", "demo", "plugin-bin"), data, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(processManifest, "name: process-webhook", "name: demo", 1)
	if err := os.WriteFile(filepath.Join("plugins", "demo", ManifestFileName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// 相对插件目录：进程入口必须在子进程工作目录下正确解析。
	mgr := newTestManager(t, "plugins")
	if err := mgr.Load(context.Background()); err != nil {
		t.Fatalf("load with relative dir: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "demo"); !ok {
		t.Fatal("plugin not registered with relative dir")
	}
}

func TestProcessPluginDisableEnable(t *testing.T) {
	requireBinary(t, "process-webhook")
	dir := installProcess(t, "process-webhook", processManifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	if !mgr.Enabled("process-webhook") {
		t.Fatal("plugin should be enabled after load")
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "process-webhook"); !ok {
		t.Fatal("plugin not registered after load")
	}

	if err := mgr.Disable(ctx, "process-webhook"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if mgr.Enabled("process-webhook") {
		t.Fatal("plugin should be disabled")
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "process-webhook"); ok {
		t.Fatal("disabled plugin should not be registered")
	}
	if _, ok := mgr.Provider("process-webhook"); ok {
		t.Fatal("disabled plugin should have no provider")
	}
	// 描述仍可读（来自缓存），便于后台展示与配置。
	if desc, ok := mgr.Describe("process-webhook"); !ok || desc.Name != "process-webhook" {
		t.Fatalf("descriptor should remain available: %+v ok=%v", desc, ok)
	}
	found := false
	for _, info := range mgr.Installed() {
		if info.Manifest.Name == "process-webhook" {
			found = true
			if info.Enabled {
				t.Fatal("installed info should report disabled")
			}
		}
	}
	if !found {
		t.Fatal("installed list missing plugin")
	}

	if _, err := mgr.Enable(ctx, "process-webhook"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !mgr.Enabled("process-webhook") {
		t.Fatal("plugin should be enabled after enable")
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "process-webhook"); !ok {
		t.Fatal("plugin should be re-registered after enable")
	}
	// 重新启用后应可正常调用。
	if err := mgr.Configure(ctx, "process-webhook", map[string]string{"url": "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("configure after enable: %v", err)
	}
}

func TestProcessPluginReloadKeepsDisabled(t *testing.T) {
	requireBinary(t, "process-webhook")
	dir := installProcess(t, "process-webhook", processManifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Disable(ctx, "process-webhook"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := mgr.Reload(ctx, "process-webhook"); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if mgr.Enabled("process-webhook") {
		t.Fatal("reload should keep the plugin disabled")
	}
}

func TestProcessPluginRequiresExecutable(t *testing.T) {
	requireBinary(t, "process-webhook")
	dir := installProcess(t, "process-webhook", processManifest)
	// 去掉可执行位。
	if err := os.Chmod(filepath.Join(dir, "plugin-bin"), 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	mgr := newTestManager(t, dir)
	if err := mgr.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("want not-executable error, got %v", err)
	}
}
