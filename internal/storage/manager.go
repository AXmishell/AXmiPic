package storage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/AXmishell/axmipic/internal/config"
)

// 存储池放置策略。
const (
	// PoolPolicyWeightedRR 按权重平滑轮询。
	PoolPolicyWeightedRR = "weighted_rr"
	// PoolPolicyPriority 优先使用 Priority 数值更小的成员，同优先级内加权轮询。
	PoolPolicyPriority = "priority"
)

// 存储池选择错误。
var (
	// ErrNoStorageBackend 表示没有可用的存储后端。
	ErrNoStorageBackend = errors.New("storage: no available backend")
	// ErrNoPresigner 表示没有支持预签名的可用后端。
	ErrNoPresigner = errors.New("storage: no available backend supporting presign")
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
	mu          sync.RWMutex
	backends    map[string]*backend
	pools       map[string]*pool
	current     string
	currentPool string
	fallback    Storage // 无匹配后端时使用的兜底后端（通常为配置文件的存储）
}

// PoolMemberConfig 是存储池成员的放置参数。
type PoolMemberConfig struct {
	BackendID string
	Weight    int
	Priority  int
	Enabled   bool
}

// poolMember 是池内成员的运行时状态。
type poolMember struct {
	backendID string
	weight    int
	priority  int
	enabled   bool
}

// pool 是存储池的运行时状态。rr 保存平滑加权轮询的当前权重。
type pool struct {
	id      string
	name    string
	policy  string
	members []poolMember
	rr      map[string]int
}

// PoolInfo 是存储池的对外描述。
type PoolInfo struct {
	ID      string
	Name    string
	Policy  string
	Current bool
	Members []PoolMemberConfig
}

// NewManager 创建一个 Manager。
func NewManager() *Manager {
	return &Manager{
		backends: make(map[string]*backend),
		pools:    make(map[string]*pool),
	}
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
	case "oss":
		return NewOSS(cfg.OSS)
	case "cos":
		return NewCOS(cfg.COS)
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
	m.currentPool = ""
	return nil
}

// RegisterPool 新增或替换一个存储池。成员必须引用已注册的后端。
func (m *Manager) RegisterPool(id, name, policy string, members []PoolMemberConfig) error {
	if policy == "" {
		policy = PoolPolicyWeightedRR
	}
	if policy != PoolPolicyWeightedRR && policy != PoolPolicyPriority {
		return fmt.Errorf("storage: unknown pool policy %q", policy)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p := &pool{id: id, name: name, policy: policy, rr: make(map[string]int)}
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		if _, ok := m.backends[member.BackendID]; !ok {
			return fmt.Errorf("storage: backend %q is not registered", member.BackendID)
		}
		if _, dup := seen[member.BackendID]; dup {
			return fmt.Errorf("storage: duplicate pool member %q", member.BackendID)
		}
		seen[member.BackendID] = struct{}{}
		if member.Weight <= 0 {
			member.Weight = 1
		}
		p.members = append(p.members, poolMember{
			backendID: member.BackendID,
			weight:    member.Weight,
			priority:  member.Priority,
			enabled:   member.Enabled,
		})
	}
	sort.SliceStable(p.members, func(i, j int) bool {
		if p.members[i].priority != p.members[j].priority {
			return p.members[i].priority < p.members[j].priority
		}
		return p.members[i].backendID < p.members[j].backendID
	})
	m.pools[id] = p
	return nil
}

// RemovePool 删除一个存储池。若它是当前池，则清空当前池。
func (m *Manager) RemovePool(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pools, id)
	if m.currentPool == id {
		m.currentPool = ""
	}
}

// SetCurrentPool 将指定存储池设为当前写入目标。设置后单后端 Current 仍保留
// 用于历史空 storage_id 的读取兜底，但新写入由池调度。
func (m *Manager) SetCurrentPool(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id != "" {
		if _, ok := m.pools[id]; !ok {
			return fmt.Errorf("storage: pool %q not registered", id)
		}
	}
	m.currentPool = id
	return nil
}

// CurrentPoolID 返回当前存储池 id；未启用池时为空。
func (m *Manager) CurrentPoolID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentPool
}

// Pools 返回全部已注册存储池的只读快照。
func (m *Manager) Pools() []PoolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]PoolInfo, 0, len(m.pools))
	for _, p := range m.pools {
		members := make([]PoolMemberConfig, 0, len(p.members))
		for _, member := range p.members {
			members = append(members, PoolMemberConfig{
				BackendID: member.backendID,
				Weight:    member.weight,
				Priority:  member.priority,
				Enabled:   member.enabled,
			})
		}
		out = append(out, PoolInfo{
			ID: p.id, Name: p.name, Policy: p.policy,
			Current: p.id == m.currentPool, Members: members,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Pick 为新对象选择写入后端。当前池存在时按池策略选择；否则使用单后端
// 当前默认或兜底后端。
func (m *Manager) Pick(ctx context.Context) (Storage, string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.currentPool != "" {
		if p, ok := m.pools[m.currentPool]; ok {
			b := m.pickMemberLocked(p, false)
			if b == nil {
				return nil, "", "", ErrNoStorageBackend
			}
			return b.storage, b.id, p.id, nil
		}
	}
	if b, ok := m.backends[m.current]; ok {
		return b.storage, b.id, "", nil
	}
	if m.fallback != nil {
		return m.fallback, "", "", nil
	}
	return nil, "", "", ErrNoStorageBackend
}

// PickPresigner 为直传选择支持预签名的后端。池内会跳过不支持预签名的成员。
func (m *Manager) PickPresigner(ctx context.Context) (Presigner, Storage, string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.currentPool != "" {
		if p, ok := m.pools[m.currentPool]; ok {
			b := m.pickMemberLocked(p, true)
			if b == nil {
				return nil, nil, "", "", ErrNoPresigner
			}
			return b.presign, b.storage, b.id, p.id, nil
		}
	}
	if b, ok := m.backends[m.current]; ok {
		if b.presign == nil {
			return nil, nil, "", "", ErrNoPresigner
		}
		return b.presign, b.storage, b.id, "", nil
	}
	if p := asPresigner(m.fallback); p != nil {
		return p, m.fallback, "", "", nil
	}
	return nil, nil, "", "", ErrNoPresigner
}

// pickMemberLocked 依据池策略选择一个可用成员。requirePresign 为 true 时只考虑
// 支持预签名的后端。调用方必须持有 m.mu。
func (m *Manager) pickMemberLocked(p *pool, requirePresign bool) *backend {
	eligible := make([]poolMember, 0, len(p.members))
	minPriority := 0
	for _, member := range p.members {
		if !member.enabled {
			continue
		}
		b, ok := m.backends[member.backendID]
		if !ok {
			continue
		}
		if requirePresign && b.presign == nil {
			continue
		}
		if len(eligible) == 0 || member.priority < minPriority {
			minPriority = member.priority
		}
		eligible = append(eligible, member)
	}
	if len(eligible) == 0 {
		return nil
	}
	if p.policy == PoolPolicyPriority {
		filtered := eligible[:0]
		for _, member := range eligible {
			if member.priority == minPriority {
				filtered = append(filtered, member)
			}
		}
		eligible = filtered
	}
	// 平滑加权轮询：current += weight；选中 current 最大者并减去总权重。
	total := 0
	selected := eligible[0]
	selectedWeight := 0
	for _, member := range eligible {
		weight := member.weight
		if weight <= 0 {
			weight = 1
		}
		p.rr[member.backendID] += weight
		total += weight
		if p.rr[member.backendID] > selectedWeight {
			selected = member
			selectedWeight = p.rr[member.backendID]
		}
	}
	p.rr[selected.backendID] -= total
	return m.backends[selected.backendID]
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

// Has 报告指定 id 的后端是否已注册。
func (m *Manager) Has(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.backends[id]
	return ok
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
