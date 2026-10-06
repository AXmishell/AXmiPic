package service

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// byteLimiter 是一个以字节为单位计量的并发信号量：调用方按预估字节数申请
// 配额，超过总预算时排队等待（尊重 ctx 取消）。它以固定单位（默认 1 MiB）
// 把字节数折算为权重，底层用加权信号量实现，避免部分占用导致的相互等待。
type byteLimiter struct {
	unit     int64
	capacity int64
	sem      *semaphore.Weighted
}

// newByteLimiter 创建一个总预算为 capacityBytes 的限流器。capacityBytes<=0
// 时返回 nil（表示不限制）。
func newByteLimiter(capacityBytes, unit int64) *byteLimiter {
	if capacityBytes <= 0 {
		return nil
	}
	if unit <= 0 {
		unit = 1 << 20
	}
	n := capacityBytes / unit
	if n < 1 {
		n = 1
	}
	return &byteLimiter{unit: unit, capacity: n, sem: semaphore.NewWeighted(n)}
}

// cost 把字节数折算为权重，至少为 1，且不超过总容量（单个超大请求也能独占
// 预算执行，而不是永久等待）。
func (b *byteLimiter) cost(n int64) int64 {
	if n <= 0 {
		return 1
	}
	want := (n + b.unit - 1) / b.unit
	if want < 1 {
		want = 1
	}
	if want > b.capacity {
		want = b.capacity
	}
	return want
}

// Acquire 申请 n 字节的配额，直到可用或 ctx 取消。
func (b *byteLimiter) Acquire(ctx context.Context, n int64) error {
	if b == nil {
		return nil
	}
	return b.sem.Acquire(ctx, b.cost(n))
}

// Release 归还 n 字节的配额。
func (b *byteLimiter) Release(n int64) {
	if b == nil {
		return
	}
	b.sem.Release(b.cost(n))
}
