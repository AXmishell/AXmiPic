package storage

import (
	"fmt"
	"sync"

	"github.com/AXmishell/axmipic/internal/config"
)

// backend 描述一个已命名的存储后端配置。
type backend struct {
	id      string
	name    string
	driver  string
	storage Storage
	presign Presigner
}

// Manager 持有一组已命名的存储后端，并跟踪当前默认后端。运行中切换默认
// 后端是原子操作，无需重启即可生效。每个后端按 id 缓存，读取旧图片时可
// 按图片记录的 storage_id 路由到对应后端。
type Manager struct {
	mu       sync.RWMutex
	backends map[string]*backend
	current  string
	fallback Storage // 无匹配后端时使用的兜底后端（通常为配置文件的存储）
}

// NewManager 创建一个 Manager。
func NewManager() *Manager {
	return &Manager{backends: make(map[string]*backend)}
}

// newBackend 依据配置构建底层 Storage 实例。
func newBackend(baseURL string, cfg config.StorageConfig) (Storage, error) {
	switch cfg.Driver {
	case "local":
		return NewLocal(cfg.Local.Root, baseURL)
	case "s3":
		return NewS3(cfg.S3)
	case "qiniu":
		return NewQiniu(cfg.Qiniu)
	default:
		return nil, fmt.Errorf("storage: unknown driver %q", cfg.Driver)
	}
}

// Register 新增或替换一个命名后端，并返回已构建的实例。
func (m *Manager) Register(id, name string, baseURL string, cfg config.StorageConfig) (Storage, error) {
	store, err := newBackend(baseURL, cfg)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backends[id] = &backend{
		id:      id,
		name:    name,
		driver:  cfg.Driver,
		storage: store,
		presign: asPresigner(store),
	}
	return store, nil
}

// Remove 删除一个命名后端。若它是当前默认，则清空默认。
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.backends, id)
	if m.current == id {
		m.current = ""
	}
}

// SetCurrent 将指定 id 的后端设为当前默认。id 为空表示清空默认。
func (m *Manager) SetCurrent(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id != "" {
		if _, ok := m.backends[id]; !ok {
			return fmt.Errorf("storage: backend %q not registered", id)
		}
	}
	m.current = id
	return nil
}

// SetFallback 设置无匹配后端时使用的兜底后端。
func (m *Manager) SetFallback(store Storage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fallback = store
}

// Current 返回当前默认后端，未设置时返回兜底后端。
func (m *Manager) Current() Storage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if b, ok := m.backends[m.current]; ok {
		return b.storage
	}
	return m.fallback
}

// Resolve 返回 id 对应的后端：id 为空或未找到时回退到当前默认后端。
func (m *Manager) Resolve(id string) Storage {
	m.mu.RLock()
	b, ok := m.backends[id]
	m.mu.RUnlock()
	if ok {
		return b.storage
	}
	return m.Current()
}

// CurrentID 返回当前默认后端的 id。
func (m *Manager) CurrentID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Drivers 返回当前默认后端的驱动名，以及是否已设置默认后端。
func (m *Manager) DriverOf(id string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if b, ok := m.backends[id]; ok {
		return b.driver, true
	}
	return "", false
}

// List 返回所有已注册后端的简要描述，供管理接口展示。
func (m *Manager) List() []BackendInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	infos := make([]BackendInfo, 0, len(m.backends))
	for _, b := range m.backends {
		infos = append(infos, BackendInfo{
			ID:      b.id,
			Name:    b.name,
			Driver:  b.driver,
			Current: b.id == m.current,
		})
	}
	return infos
}

// BackendInfo 是后端配置的对外描述。
type BackendInfo struct {
	ID      string
	Name    string
	Driver  string
	Current bool
}

// Backend 是一个已注册后端的快照，供后台维护任务遍历。
type Backend struct {
	ID      string
	Name    string
	Driver  string
	Storage Storage
}

// All 返回所有已注册后端的快照。它用于孤儿对象对账等需要遍历后端的维护任务。
func (m *Manager) All() []Backend {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Backend, 0, len(m.backends))
	for _, b := range m.backends {
		out = append(out, Backend{ID: b.id, Name: b.name, Driver: b.driver, Storage: b.storage})
	}
	return out
}

// asPresigner 将支持直传的存储实例转换为 Presigner，否则返回 nil。
func asPresigner(store Storage) Presigner {
	if p, ok := store.(Presigner); ok {
		return p
	}
	return nil
}

// PresignerFor 返回指定 id 后端对应的 Presigner；无匹配时返回当前默认后端的。
// 若后端未注册（例如仅设置了兜底后端），则尝试把兜底后端转换为 Presigner。
func (m *Manager) PresignerFor(id string) Presigner {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if b, ok := m.backends[id]; ok {
		return b.presign
	}
	if b, ok := m.backends[m.current]; ok {
		return b.presign
	}
	return asPresigner(m.fallback)
}
