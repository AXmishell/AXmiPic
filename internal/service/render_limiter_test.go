package service

import (
	"context"
	"testing"
	"time"
)

func TestByteLimiterAcquireRelease(t *testing.T) {
	lim := newByteLimiter(4<<20, 1<<20) // 4 个 1 MiB 令牌
	if lim == nil {
		t.Fatal("expected a limiter")
	}
	ctx := context.Background()
	if err := lim.Acquire(ctx, 3<<20); err != nil {
		t.Fatalf("Acquire 3MiB: %v", err)
	}

	done := make(chan struct{})
	go func() {
		if err := lim.Acquire(ctx, 2<<20); err != nil {
			t.Errorf("Acquire 2MiB: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Acquire should block while the budget is exhausted")
	case <-time.After(50 * time.Millisecond):
	}

	lim.Release(3 << 20)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Acquire should proceed after Release")
	}
}

func TestByteLimiterAcquireRespectsContext(t *testing.T) {
	lim := newByteLimiter(1<<20, 1<<20)
	if err := lim.Acquire(context.Background(), 1<<20); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := lim.Acquire(ctx, 1<<20); err == nil {
		t.Fatal("expected a context error, got nil")
	}
}

func TestByteLimiterNilIsUnlimited(t *testing.T) {
	var lim *byteLimiter
	if err := lim.Acquire(context.Background(), 1<<30); err != nil {
		t.Fatalf("nil limiter should never block: %v", err)
	}
	lim.Release(1 << 30)
}

func TestRenderMemoryEstimate(t *testing.T) {
	if got := renderMemoryEstimate(1<<20, 10_000); got != 1<<20+40_000 {
		t.Fatalf("estimate = %d", got)
	}
	if got := renderMemoryEstimate(1<<20, 0); got != 3<<20 {
		t.Fatalf("fallback estimate = %d", got)
	}
}
