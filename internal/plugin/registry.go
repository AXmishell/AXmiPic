package plugin

import (
	"sort"
	"sync"
)

// registryKey 唯一定位一个插件：类别 + 名称。
type registryKey struct {
	category string
	name     string
}

// Entry 是注册表中的一个已加载插件。
type Entry struct {
	Manifest   Manifest
	Descriptor Descriptor
	Provider   Provider
	Dir        string
}

// key 返回该条目的注册表键。
func (e Entry) key() registryKey {
	return registryKey{category: e.Descriptor.Category, name: e.Descriptor.Name}
}

// Registry 是线程安全的插件注册表。
type Registry struct {
	mu      sync.RWMutex
	entries map[registryKey]Entry
}

// NewRegistry 创建一个空注册表。
func NewRegistry() *Registry {
	return &Registry{entries: make(map[registryKey]Entry)}
}

// Register 新增或替换一个插件条目。
func (r *Registry) Register(e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[e.key()] = e
}

// Unregister 移除一个插件条目，返回是否存在。
func (r *Registry) Unregister(category, name string) bool {
	k := registryKey{category: category, name: name}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[k]; !ok {
		return false
	}
	delete(r.entries, k)
	return true
}

// Lookup 按类别与名称查找插件。
func (r *Registry) Lookup(category, name string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[registryKey{category: category, name: name}]
	return e, ok
}

// List 返回指定类别的所有插件，按名称排序；category 为空时返回全部。
func (r *Registry) List(category string) []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Descriptor, 0, len(r.entries))
	for k, e := range r.entries {
		if category != "" && k.category != category {
			continue
		}
		out = append(out, e.Descriptor)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Names 返回指定类别下已注册插件的名称集合。
func (r *Registry) Names(category string) []string {
	descs := r.List(category)
	names := make([]string, 0, len(descs))
	for _, d := range descs {
		names = append(names, d.Name)
	}
	return names
}

// Len 返回已注册插件数量。
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}
