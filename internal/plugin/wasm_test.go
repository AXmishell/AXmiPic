package plugin

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// exampleWasm 保存各示例插件的编译产物路径，由 TestMain 准备。
var exampleWasm = map[string]string{}

// exampleBin 保存各进程插件的可执行文件路径，由 TestMain 准备。
var exampleBin = map[string]string{}

// exampleDirs 保存编译临时目录，退出时清理。
var exampleDirs []string

// examplePlugins 是需要预编译的 WASM 示例插件（目录名）。
var examplePlugins = []string{"smsbao", "aliyun-sms", "tencent-sms"}

// exampleProcessPlugins 是需要预编译的进程示例插件（目录名）。
var exampleProcessPlugins = []string{"process-webhook"}

// TestMain 预编译示例插件；失败时相关测试会跳过。
func TestMain(m *testing.M) {
	flag.Parse()
	if os.Getenv("AXMIPIC_SKIP_WASM_TESTS") == "" && !testing.Short() {
		for _, name := range examplePlugins {
			path, dir, err := buildExampleModule(name)
			if err != nil {
				slog.Default().Warn("skipping WASM plugin test for " + name + ": " + err.Error())
				continue
			}
			exampleWasm[name] = path
			exampleDirs = append(exampleDirs, dir)
		}
		for _, name := range exampleProcessPlugins {
			path, dir, err := buildExampleBinary(name)
			if err != nil {
				slog.Default().Warn("skipping process plugin test for " + name + ": " + err.Error())
				continue
			}
			exampleBin[name] = path
			exampleDirs = append(exampleDirs, dir)
		}
	}
	code := m.Run()
	for _, dir := range exampleDirs {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

// buildExampleModule 使用 Go 工具链把示例插件编译为 WASM reactor。
func buildExampleModule(name string) (string, string, error) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		return "", "", err
	}
	exampleDir := filepath.Join(repoRoot, "sdk", "plugin-go", "examples", name)
	if _, err := os.Stat(exampleDir); err != nil {
		return "", "", err
	}
	tmp, err := os.MkdirTemp("", "axmipic-plugin-")
	if err != nil {
		return "", "", err
	}
	out := filepath.Join(tmp, "plugin.wasm")
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", out, ".")
	cmd.Dir = exampleDir
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		return "", tmp, fmt.Errorf("%w: %s", err, buildOut)
	}
	return out, tmp, nil
}

// buildExampleBinary 使用 Go 工具链把进程示例插件编译为宿主可执行文件。
func buildExampleBinary(name string) (string, string, error) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		return "", "", err
	}
	exampleDir := filepath.Join(repoRoot, "sdk", "plugin-go", "examples", name)
	if _, err := os.Stat(exampleDir); err != nil {
		return "", "", err
	}
	tmp, err := os.MkdirTemp("", "axmipic-plugin-")
	if err != nil {
		return "", "", err
	}
	out := filepath.Join(tmp, "plugin-bin")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = exampleDir
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		return "", tmp, fmt.Errorf("%w: %s", err, buildOut)
	}
	return out, tmp, nil
}

func requireExample(t *testing.T) {
	t.Helper()
	requireModule(t, "smsbao")
}

// requireModule 要求指定示例插件已编译。
func requireModule(t *testing.T, name string) {
	t.Helper()
	if exampleWasm[name] == "" {
		t.Skipf("example plugin %q wasm not available", name)
	}
}

// installModule 把指定示例插件的 wasm 与清单复制到临时目录，返回该目录。
func installModule(t *testing.T, name, manifest string) string {
	t.Helper()
	requireModule(t, name)
	dir := t.TempDir()
	data, err := os.ReadFile(exampleWasm[name])
	if err != nil {
		t.Fatalf("read example wasm: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.wasm"), data, 0o600); err != nil {
		t.Fatalf("write example wasm: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}

// requireBinary 要求指定进程插件已编译。
func requireBinary(t *testing.T, name string) {
	t.Helper()
	if exampleBin[name] == "" {
		t.Skipf("process plugin %q binary not available", name)
	}
}

// installProcess 把进程插件可执行文件与清单复制到临时目录，返回该目录。
func installProcess(t *testing.T, name, manifest string) string {
	t.Helper()
	requireBinary(t, name)
	dir := t.TempDir()
	data, err := os.ReadFile(exampleBin[name])
	if err != nil {
		t.Fatalf("read example binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin-bin"), data, 0o755); err != nil {
		t.Fatalf("write example binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}

// installExample 把示例插件（短信宝）复制到临时目录。
func installExample(t *testing.T, manifest string) string {
	t.Helper()
	return installModule(t, "smsbao", manifest)
}

const allowLocalManifest = `
name: smsbao
version: 0.1.0
category: notify.sms
runtime: wasm
entry: plugin.wasm
abi: 1
capabilities:
  http:
    hosts: ["127.0.0.1"]
`

func newTestManager(t *testing.T, dir string) *Manager {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManager(Options{Dir: dir, Logger: logger, HTTPTimeout: 5 * time.Second})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mgr.Close(ctx); err != nil {
			t.Errorf("manager close: %v", err)
		}
	})
	return mgr
}

func TestWASMPluginDescribeAndSend(t *testing.T) {
	requireExample(t)

	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = map[string][]string(r.URL.Query())
		_, _ = w.Write([]byte("0"))
	}))
	defer srv.Close()

	dir := installExample(t, allowLocalManifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()

	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}

	entry, ok := mgr.Registry().Lookup(CategoryNotifySMS, "smsbao")
	if !ok {
		t.Fatal("plugin not registered")
	}
	desc := entry.Descriptor
	if desc.Title != "短信宝" {
		t.Fatalf("unexpected title: %q", desc.Title)
	}
	keys := map[string]Field{}
	for _, f := range desc.Fields {
		keys[f.Key] = f
	}
	if !keys["password"].Secret {
		t.Error("password field should be secret")
	}
	if _, ok := keys["endpoint"]; !ok {
		t.Error("endpoint field missing")
	}

	if err := mgr.Configure(ctx, "smsbao", map[string]string{
		"username": "acct",
		"password": "secret",
		"endpoint": srv.URL,
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}

	input, _ := json.Marshal(map[string]string{"to": "13800000000", "body": "hello"})
	out, err := mgr.Invoke(ctx, CategoryNotifySMS, "smsbao", "send", input)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("invalid result: %v (%s)", err, out)
	}
	if result["ok"] != true {
		t.Fatalf("unexpected result: %s", out)
	}

	if gotQuery["u"][0] != "acct" || gotQuery["m"][0] != "13800000000" {
		t.Fatalf("unexpected query: %v", gotQuery)
	}
	sum := md5.Sum([]byte("secret"))
	if gotQuery["p"][0] != hex.EncodeToString(sum[:]) {
		t.Fatalf("password not md5-hashed: %v", gotQuery["p"])
	}
	if !strings.Contains(gotQuery["c"][0], "hello") {
		t.Fatalf("content missing body: %v", gotQuery["c"])
	}
}

func TestWASMPluginCapabilityDenied(t *testing.T) {
	requireExample(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0"))
	}))
	defer srv.Close()

	// 清单未声明 http 能力，网络访问应被拒绝。
	dir := installExample(t, `
name: smsbao
category: notify.sms
runtime: wasm
entry: plugin.wasm
abi: 1
`)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Configure(ctx, "smsbao", map[string]string{
		"username": "acct", "password": "secret", "endpoint": srv.URL,
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	input, _ := json.Marshal(map[string]string{"to": "13800000000", "body": "hello"})
	_, err := mgr.Invoke(ctx, CategoryNotifySMS, "smsbao", "send", input)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("want capability denial, got %v", err)
	}
}

func TestWASMPluginConfigureValidation(t *testing.T) {
	requireExample(t)
	dir := installExample(t, allowLocalManifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Configure(ctx, "smsbao", map[string]string{"username": ""}); err == nil {
		t.Fatal("configure with empty username should fail")
	}
}

func TestWASMPluginReloadAndUnload(t *testing.T) {
	requireExample(t)
	dir := installExample(t, allowLocalManifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Reload(ctx, "smsbao"); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "smsbao"); !ok {
		t.Fatal("plugin missing after reload")
	}
	if err := mgr.Unload(ctx, "smsbao"); err != nil {
		t.Fatalf("unload: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "smsbao"); ok {
		t.Fatal("plugin still registered after unload")
	}
}

func TestWASMPluginLazyLoadAndSuspend(t *testing.T) {
	requireExample(t)
	dir := installExample(t, allowLocalManifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()

	// 启动只登记不实例化，但保留启用意图（enabled=true）。
	if err := mgr.LoadFiltered(ctx, func(string) bool { return false }, func(string) bool { return true }); err != nil {
		t.Fatalf("load filtered: %v", err)
	}
	infos := mgr.Installed()
	if len(infos) != 1 || infos[0].Loaded || !infos[0].Enabled {
		t.Fatalf("expected enabled-but-not-loaded standby: %+v", infos)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "smsbao"); ok {
		t.Fatal("standby plugin should not be registered")
	}

	// Enable 是幂等的：不触发实例化。
	if _, err := mgr.Enable(ctx, "smsbao"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "smsbao"); ok {
		t.Fatal("enable must not instantiate the plugin")
	}
	if infos := mgr.Installed(); !infos[0].Enabled || infos[0].Loaded {
		t.Fatalf("expected standby after enable: %+v", infos[0])
	}

	// 首次使用才按需加载并注册。
	if _, err := mgr.Acquire(ctx, "smsbao"); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "smsbao"); !ok {
		t.Fatal("plugin should be registered after first use")
	}
	if infos := mgr.Installed(); infos[0].MemoryBytes == 0 {
		t.Fatal("loaded WASM plugin should report non-zero memory")
	}

	// 空闲卸载：注销并释放，但保持逻辑启用。
	if err := mgr.Suspend(ctx, "smsbao"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "smsbao"); ok {
		t.Fatal("suspended plugin should be unregistered")
	}
	if !mgr.Enabled("smsbao") {
		t.Fatal("suspend must keep the plugin enabled")
	}

	// 再次使用时应按需重载。
	if _, err := mgr.Acquire(ctx, "smsbao"); err != nil {
		t.Fatalf("acquire after suspend: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "smsbao"); !ok {
		t.Fatal("plugin should be re-registered after acquire")
	}
}

func TestWASMPluginSuspendIdle(t *testing.T) {
	requireExample(t)
	dir := installExample(t, allowLocalManifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	time.Sleep(15 * time.Millisecond)
	names := mgr.SuspendIdle(ctx, 5*time.Millisecond)
	if len(names) != 1 || names[0] != "smsbao" {
		t.Fatalf("expected smsbao suspended, got %v", names)
	}
	infos := mgr.Installed()
	if len(infos) != 1 || !infos[0].Enabled || infos[0].Loaded {
		t.Fatalf("expected enabled-but-unloaded: %+v", infos)
	}
}
