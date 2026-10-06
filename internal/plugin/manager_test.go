package plugin

import (
	"context"
	"testing"
)

// memProvider 是实现了 memoryReporter 的进程内测试插件。
type memProvider struct{ bytes uint64 }

func (memProvider) Descriptor() Descriptor {
	return Descriptor{Category: CategoryNotifySMS, Name: "mem"}
}
func (memProvider) Configure(context.Context, map[string]string) error     { return nil }
func (memProvider) Invoke(context.Context, string, []byte) ([]byte, error) { return nil, nil }
func (memProvider) Close(context.Context) error                            { return nil }
func (m memProvider) MemoryBytes() uint64                                  { return m.bytes }

func TestInstalledReportsLoadedAndMemory(t *testing.T) {
	mgr := NewManager(Options{})
	t.Cleanup(func() { _ = mgr.Close(context.Background()) })

	mgr.RegisterProvider(Manifest{Name: "mem", Category: CategoryNotifySMS, Runtime: RuntimeWASM}, memProvider{bytes: 1234})

	infos := mgr.Installed()
	if len(infos) != 1 {
		t.Fatalf("installed = %d, want 1", len(infos))
	}
	if !infos[0].Enabled || !infos[0].Loaded {
		t.Fatalf("in-process provider should be enabled+loaded: %+v", infos[0])
	}
	if infos[0].MemoryBytes != 1234 {
		t.Fatalf("memory = %d, want 1234", infos[0].MemoryBytes)
	}
}

func TestSetDescriptorOnlyForUnloaded(t *testing.T) {
	mgr := NewManager(Options{})
	t.Cleanup(func() { _ = mgr.Close(context.Background()) })

	// 未加载条目：注入缓存描述应生效。
	mgr.registerUnloaded(Manifest{Name: "lazy", Category: CategoryNotifySMS, Version: "1.0"}, "/tmp/lazy", true)
	mgr.SetDescriptor("lazy", Descriptor{Name: "lazy", Category: CategoryNotifySMS, Title: "缓存标题", Fields: []Field{{Key: "k"}}})
	d, ok := mgr.Describe("lazy")
	if !ok || d.Title != "缓存标题" || len(d.Fields) != 1 {
		t.Fatalf("cached descriptor not applied: %+v", d)
	}

	// 已加载条目：不应被缓存覆盖。
	mgr.RegisterProvider(Manifest{Name: "loaded", Category: CategoryNotifySMS, Runtime: RuntimeWASM}, memProvider{})
	mgr.SetDescriptor("loaded", Descriptor{Name: "loaded", Category: CategoryNotifySMS, Title: "不该覆盖"})
	d, _ = mgr.Describe("loaded")
	if d.Title == "不该覆盖" {
		t.Fatalf("SetDescriptor must not override a loaded plugin: %+v", d)
	}
}

func TestHasRequiresEnabledAndCategory(t *testing.T) {
	mgr := NewManager(Options{})
	t.Cleanup(func() { _ = mgr.Close(context.Background()) })
	ctx := context.Background()

	mgr.RegisterProvider(Manifest{Name: "mem", Category: CategoryNotifySMS, Runtime: RuntimeWASM}, memProvider{})
	if !mgr.Has(CategoryNotifySMS, "mem") {
		t.Fatal("enabled plugin should be available in its category")
	}
	if mgr.Has(CategoryPayment, "mem") {
		t.Fatal("plugin must not be available under a different category")
	}

	// Enable 只把插件置为 standby，不实例化。
	mgr.registerUnloaded(Manifest{Name: "broken", Category: CategoryNotifySMS}, "/nonexistent/broken", true)
	if _, err := mgr.Enable(ctx, "broken"); err != nil {
		t.Fatalf("Enable should only mark standby: %v", err)
	}
	if !mgr.Enabled("broken") || !mgr.Has(CategoryNotifySMS, "broken") {
		t.Fatal("enabled plugin should be available in its category")
	}
	if _, ok := mgr.Provider("broken"); ok {
		t.Fatal("Enable must not instantiate the plugin")
	}

	// 首次使用才加载；入口不存在时失败并记录错误，但保持启用意图。
	if _, err := mgr.Acquire(ctx, "broken"); err == nil {
		t.Fatal("Acquire should fail for a missing entry file")
	}
	for _, info := range mgr.Installed() {
		if info.Manifest.Name == "broken" {
			if info.Loaded || !info.Enabled || info.LastError == "" {
				t.Fatalf("failed activation should stay standby with an error: %+v", info)
			}
		}
	}
}
