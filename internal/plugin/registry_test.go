package plugin

import (
	"context"
	"testing"
)

// fakeProvider 是用于测试的 Provider 实现。
type fakeProvider struct {
	desc    Descriptor
	invoked []string
	closed  bool
	cfg     map[string]string
}

func (f *fakeProvider) Descriptor() Descriptor { return f.desc }
func (f *fakeProvider) Configure(_ context.Context, cfg map[string]string) error {
	f.cfg = cfg
	return nil
}
func (f *fakeProvider) Invoke(_ context.Context, op string, input []byte) ([]byte, error) {
	f.invoked = append(f.invoked, op)
	return input, nil
}
func (f *fakeProvider) Close(context.Context) error { f.closed = true; return nil }

func TestRegistryLifecycle(t *testing.T) {
	reg := NewRegistry()
	p := &fakeProvider{desc: Descriptor{Category: CategoryNotifySMS, Name: "smsbao", Title: "短信宝"}}
	reg.Register(Entry{Descriptor: p.desc, Provider: p})

	if reg.Len() != 1 {
		t.Fatalf("Len = %d, want 1", reg.Len())
	}
	got, ok := reg.Lookup(CategoryNotifySMS, "smsbao")
	if !ok || got.Descriptor.Title != "短信宝" {
		t.Fatalf("lookup failed: %+v ok=%v", got, ok)
	}
	if _, ok := reg.Lookup(CategoryPayment, "smsbao"); ok {
		t.Fatal("lookup should be scoped by category")
	}

	descs := reg.List(CategoryNotifySMS)
	if len(descs) != 1 || descs[0].Name != "smsbao" {
		t.Fatalf("unexpected list: %+v", descs)
	}
	if len(reg.List(CategoryPayment)) != 0 {
		t.Fatal("payment category should be empty")
	}

	if !reg.Unregister(CategoryNotifySMS, "smsbao") {
		t.Fatal("unregister should report true")
	}
	if reg.Unregister(CategoryNotifySMS, "smsbao") {
		t.Fatal("second unregister should report false")
	}
	if reg.Len() != 0 {
		t.Fatalf("Len = %d, want 0", reg.Len())
	}
}

func TestRegistryListSorted(t *testing.T) {
	reg := NewRegistry()
	for _, name := range []string{"z", "a", "m"} {
		reg.Register(Entry{Descriptor: Descriptor{Category: CategoryNotifySMS, Name: name}})
	}
	names := reg.Names(CategoryNotifySMS)
	want := []string{"a", "m", "z"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}
}
