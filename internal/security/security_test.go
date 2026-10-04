package security

import (
	"context"
	"testing"
)

func TestAllowAllScanner(t *testing.T) {
	result, err := AllowAllScanner{}.Scan(context.Background(), []byte("anything"), "image/png")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if result.IsBlocked() {
		t.Fatalf("expected safe, got %+v", result)
	}
}

func TestBlockingScannerRejectsExecutable(t *testing.T) {
	scanner := NewBlockingScanner([]string{"image/png", "image/jpeg"})

	safe, err := scanner.Scan(context.Background(), []byte("plain text"), "image/png")
	if err != nil || safe.IsBlocked() {
		t.Fatalf("safe result = %+v, err = %v", safe, err)
	}

	blocked, err := scanner.Scan(context.Background(), []byte{0x7F, 'E', 'L', 'F', 0x02}, "image/png")
	if err != nil || !blocked.IsBlocked() {
		t.Fatalf("elf result = %+v, err = %v", blocked, err)
	}

	// 非白名单类型被拒绝。
	unsupported, err := scanner.Scan(context.Background(), []byte("data"), "application/x-msdownload")
	if err != nil || !unsupported.IsBlocked() {
		t.Fatalf("unsupported result = %+v, err = %v", unsupported, err)
	}

	empty, err := scanner.Scan(context.Background(), nil, "image/png")
	if err != nil || !empty.IsBlocked() {
		t.Fatalf("empty result = %+v, err = %v", empty, err)
	}
}

func TestCloudProcessorUnavailable(t *testing.T) {
	proc := NewLoggingCloudProcessor(nil)
	if proc.Available() {
		t.Fatal("logging cloud processor should report unavailable")
	}
	if _, err := proc.Process(context.Background(), nil, nil); err == nil {
		t.Fatal("expected error from unconfigured cloud processor")
	}
}
