package service

import (
	"container/list"
	"sync"
	"time"

	"github.com/AXmishell/axmipic/internal/imaging"
)

// renderCacheEntry 是 LRU 中的一条派生图缓存。
type renderCacheEntry struct {
	key     string
	result  *imaging.Result
	expires time.Time
}

// renderCache 是一个按字节上限约束的进程内 LRU，用于缓存即时变换的派生图。
// 它是并发安全的：读取不改变顺序，写入会把条目移到队首并按需逐出。
type renderCache struct {
	mu       sync.Mutex
	maxBytes int64
	ttl      time.Duration
	bytes    int64
	order    *list.List
	items    map[string]*list.Element
}

// newRenderCache 创建一个容量以字节计（<=0 表示禁用）的 LRU 缓存。ttl<=0
// 表示条目不过期。
func newRenderCache(maxBytes int64, ttl time.Duration) *renderCache {
	if maxBytes <= 0 {
		return nil
	}
	return &renderCache{
		maxBytes: maxBytes,
		ttl:      ttl,
		order:    list.New(),
		items:    make(map[string]*list.Element),
	}
}

// Get 返回缓存命中的派生图。命中会把条目提升到队首。
func (c *renderCache) Get(key string) (*imaging.Result, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	entry := el.Value.(*renderCacheEntry)
	if !entry.expires.IsZero() && time.Now().After(entry.expires) {
		c.removeElement(el)
		return nil, false
	}
	c.order.MoveToFront(el)
	return entry.result, true
}

// Put 写入一条派生图。结果超过总容量上限时会被忽略；写入会逐出队尾条目直到
// 满足容量约束。
func (c *renderCache) Put(key string, result *imaging.Result) {
	if c == nil || result == nil {
		return
	}
	size := int64(len(result.Data))
	if size == 0 || size > c.maxBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeElement(el)
	}
	entry := &renderCacheEntry{key: key, result: result}
	if c.ttl > 0 {
		entry.expires = time.Now().Add(c.ttl)
	}
	el := c.order.PushFront(entry)
	c.items[key] = el
	c.bytes += size
	for c.bytes > c.maxBytes {
		back := c.order.Back()
		if back == nil {
			break
		}
		c.removeElement(back)
	}
}

// removeElement 从 LRU 中移除一个元素并回退其占用的字节数。调用方必须持有锁。
func (c *renderCache) removeElement(el *list.Element) {
	entry := el.Value.(*renderCacheEntry)
	delete(c.items, entry.key)
	c.order.Remove(el)
	c.bytes -= int64(len(entry.result.Data))
	if c.bytes < 0 {
		c.bytes = 0
	}
}
