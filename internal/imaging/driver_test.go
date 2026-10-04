package imaging_test

import (
	"testing"

	"github.com/AXmishell/axmipic/internal/imaging"
)

func TestResolvePureGo(t *testing.T) {
	processor, err := imaging.Resolve("purego")
	if err != nil {
		t.Fatalf("Resolve(purego): %v", err)
	}
	if processor.Capabilities().Name != "purego" {
		t.Fatalf("name = %q", processor.Capabilities().Name)
	}
}

func TestResolveUnknownFallsBack(t *testing.T) {
	processor, name, err := imaging.ResolveWithFallback("does-not-exist")
	if err != nil {
		t.Fatalf("ResolveWithFallback: %v", err)
	}
	if processor == nil || name == "" {
		t.Fatalf("fallback = %v, %q", processor, name)
	}
	if name != "purego" {
		t.Fatalf("expected purego fallback, got %q", name)
	}
}

func TestResolveEmptyReturnsDefault(t *testing.T) {
	processor, err := imaging.Resolve("")
	if err != nil || processor == nil {
		t.Fatalf("Resolve(\"\") = %v, %v", processor, err)
	}
}

func TestAvailableDriversIncludesPureGo(t *testing.T) {
	found := false
	for _, d := range imaging.AvailableDrivers() {
		if d == imaging.DriverPureGo {
			found = true
		}
	}
	if !found {
		t.Fatalf("purego not in available drivers: %v", imaging.AvailableDrivers())
	}
}
