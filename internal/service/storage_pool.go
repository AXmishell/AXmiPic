package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
)

// ErrStorageUnassigned 表示仍有图片未记录 storage_id，不能安全启用存储池。
var ErrStorageUnassigned = errors.New("service: images without storage id must be backfilled first")

// StoragePoolMemberDTO 是存储池成员的对外表示。
type StoragePoolMemberDTO struct {
	BackendID  string `json:"backend_id"`
	Backend    string `json:"backend"`
	Driver     string `json:"driver"`
	Weight     int    `json:"weight"`
	Priority   int    `json:"priority"`
	Enabled    bool   `json:"enabled"`
	ImageCount int64  `json:"image_count"`
	SizeBytes  int64  `json:"size_bytes"`
}

// StoragePoolDTO 是存储池的对外表示。
type StoragePoolDTO struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Policy     string                 `json:"policy"`
	IsCurrent  bool                   `json:"is_current"`
	Members    []StoragePoolMemberDTO `json:"members"`
	ImageCount int64                  `json:"image_count"`
	SizeBytes  int64                  `json:"size_bytes"`
	CreatedAt  string                 `json:"created_at"`
}

// StoragePoolMemberInput 是创建/更新存储池时的成员输入。
type StoragePoolMemberInput struct {
	BackendID string `json:"backend_id"`
	Weight    int    `json:"weight"`
	Priority  int    `json:"priority"`
	Enabled   bool   `json:"enabled"`
}

// StoragePoolInput 是创建/更新存储池的输入。
type StoragePoolInput struct {
	Name    string                   `json:"name"`
	Policy  string                   `json:"policy"`
	Members []StoragePoolMemberInput `json:"members"`
}

// ListPools 返回全部存储池及其用量。
func (s *StorageService) ListPools(ctx context.Context) ([]StoragePoolDTO, error) {
	pools, err := s.repo.ListStoragePools(ctx)
	if err != nil {
		return nil, err
	}
	backends, err := s.repo.ListStorageBackends(ctx)
	if err != nil {
		return nil, err
	}
	backendByID := make(map[string]*store.StorageBackend, len(backends))
	for i := range backends {
		backendByID[backends[i].ID] = &backends[i]
	}
	currentPool := s.manager.CurrentPoolID()
	out := make([]StoragePoolDTO, 0, len(pools))
	for i := range pools {
		p := &pools[i]
		members, err := s.repo.ListStoragePoolMembers(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		dto := StoragePoolDTO{
			ID:        p.ID,
			Name:      p.Name,
			Policy:    normalizePoolPolicy(p.Policy),
			IsCurrent: p.ID == currentPool,
			CreatedAt: p.CreatedAt.Format(time.RFC3339),
		}
		ids := make([]string, 0, len(members))
		for _, member := range members {
			ids = append(ids, member.BackendID)
		}
		usage, err := s.repo.StorageUsageByBackend(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			backend := backendByID[member.BackendID]
			memberDTO := StoragePoolMemberDTO{
				BackendID:  member.BackendID,
				Weight:     member.Weight,
				Priority:   member.Priority,
				Enabled:    member.Enabled,
				ImageCount: usage[member.BackendID].Count,
				SizeBytes:  usage[member.BackendID].Bytes,
			}
			if backend != nil {
				memberDTO.Backend = backend.Name
				memberDTO.Driver = backend.Driver
			}
			dto.ImageCount += memberDTO.ImageCount
			dto.SizeBytes += memberDTO.SizeBytes
			dto.Members = append(dto.Members, memberDTO)
		}
		out = append(out, dto)
	}
	return out, nil
}

// CreatePool 创建存储池并注册到运行时。
func (s *StorageService) CreatePool(ctx context.Context, in StoragePoolInput) (*StoragePoolDTO, error) {
	name, policy, members, err := s.validatePoolInput(ctx, in)
	if err != nil {
		return nil, err
	}
	pool := &store.StoragePool{ID: uuid.NewString(), Name: name, Policy: policy}
	for i := range members {
		members[i].PoolID = pool.ID
	}
	if err := s.repo.CreateStoragePool(ctx, pool, members); err != nil {
		return nil, err
	}
	if err := s.manager.RegisterPool(pool.ID, pool.Name, pool.Policy, poolMemberConfigs(members)); err != nil {
		_ = s.repo.DeleteStoragePool(ctx, pool.ID)
		return nil, fmt.Errorf("%w: %v", ErrStorageConfig, err)
	}
	return s.poolDTO(ctx, pool.ID)
}

// UpdatePool 更新存储池名称、策略与成员。
func (s *StorageService) UpdatePool(ctx context.Context, id string, in StoragePoolInput) (*StoragePoolDTO, error) {
	existing, err := s.repo.GetStoragePool(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrStorageNotFound
		}
		return nil, err
	}
	name, policy, members, err := s.validatePoolInput(ctx, in)
	if err != nil {
		return nil, err
	}
	existing.Name = name
	existing.Policy = policy
	for i := range members {
		members[i].PoolID = existing.ID
	}
	if err := s.repo.UpdateStoragePool(ctx, existing, members); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrStorageNotFound
		}
		return nil, err
	}
	if err := s.manager.RegisterPool(existing.ID, existing.Name, existing.Policy, poolMemberConfigs(members)); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageConfig, err)
	}
	return s.poolDTO(ctx, existing.ID)
}

// DeletePool 删除存储池；底层后端与对象保留。当前池不能直接删除。
func (s *StorageService) DeletePool(ctx context.Context, id string) error {
	if _, err := s.repo.GetStoragePool(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrStorageNotFound
		}
		return err
	}
	if s.manager.CurrentPoolID() == id {
		return fmt.Errorf("%w: storage pool is currently active", ErrStorageInUse)
	}
	if target, ok := s.loadCurrentTarget(ctx); ok && target.Kind == "pool" && target.ID == id {
		return fmt.Errorf("%w: storage pool is currently active", ErrStorageInUse)
	}
	if err := s.repo.DeleteStoragePool(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrStorageNotFound
		}
		return err
	}
	s.manager.RemovePool(id)
	return nil
}

// SetCurrentPool 将存储池设为当前写入目标。要求所有历史图片已有 storage_id。
func (s *StorageService) SetCurrentPool(ctx context.Context, id string) error {
	if _, err := s.repo.GetStoragePool(ctx, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrStorageNotFound
		}
		return err
	}
	members, err := s.repo.ListStoragePoolMembers(ctx, id)
	if err != nil {
		return err
	}
	hasEnabled := false
	for _, member := range members {
		if member.Enabled && s.manager.Has(member.BackendID) {
			hasEnabled = true
			break
		}
	}
	if !hasEnabled {
		return fmt.Errorf("%w: storage pool has no enabled backend", ErrStorageConfig)
	}
	if count, err := s.repo.CountImagesWithoutStorage(ctx); err != nil {
		return err
	} else if count > 0 {
		return fmt.Errorf("%w: %d images", ErrStorageUnassigned, count)
	}
	if err := s.saveCurrentTarget(ctx, storageCurrentTarget{Kind: "pool", ID: id}); err != nil {
		return err
	}
	if err := s.repo.ClearCurrentStorageBackend(ctx); err != nil {
		return err
	}
	return s.manager.SetCurrentPool(id)
}

// poolDTO 读取单个存储池的对外表示。
func (s *StorageService) poolDTO(ctx context.Context, id string) (*StoragePoolDTO, error) {
	pools, err := s.ListPools(ctx)
	if err != nil {
		return nil, err
	}
	for i := range pools {
		if pools[i].ID == id {
			return &pools[i], nil
		}
	}
	return nil, ErrStorageNotFound
}

// validatePoolInput 校验并规范化存储池输入。
func (s *StorageService) validatePoolInput(ctx context.Context, in StoragePoolInput) (string, string, []store.StoragePoolMember, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return "", "", nil, fmt.Errorf("%w: pool name is required", ErrStorageConfig)
	}
	policy := normalizePoolPolicy(in.Policy)
	if policy != storage.PoolPolicyWeightedRR && policy != storage.PoolPolicyPriority {
		return "", "", nil, fmt.Errorf("%w: unknown pool policy %q", ErrStorageConfig, in.Policy)
	}
	if len(in.Members) == 0 {
		return "", "", nil, fmt.Errorf("%w: pool must contain at least one backend", ErrStorageConfig)
	}
	seen := make(map[string]struct{}, len(in.Members))
	enabled := 0
	members := make([]store.StoragePoolMember, 0, len(in.Members))
	for _, member := range in.Members {
		backendID := strings.TrimSpace(member.BackendID)
		if backendID == "" {
			return "", "", nil, fmt.Errorf("%w: member backend_id is required", ErrStorageConfig)
		}
		if _, dup := seen[backendID]; dup {
			return "", "", nil, fmt.Errorf("%w: duplicate member %q", ErrStorageConfig, backendID)
		}
		seen[backendID] = struct{}{}
		if !s.manager.Has(backendID) {
			return "", "", nil, fmt.Errorf("%w: backend %q is not registered", ErrStorageConfig, backendID)
		}
		weight := member.Weight
		if weight <= 0 {
			weight = 100
		}
		if member.Priority < 0 {
			return "", "", nil, fmt.Errorf("%w: member priority must not be negative", ErrStorageConfig)
		}
		if member.Enabled {
			enabled++
		}
		members = append(members, store.StoragePoolMember{
			BackendID: backendID,
			Weight:    weight,
			Priority:  member.Priority,
			Enabled:   member.Enabled,
		})
	}
	if enabled == 0 {
		return "", "", nil, fmt.Errorf("%w: pool must have at least one enabled backend", ErrStorageConfig)
	}
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].Priority != members[j].Priority {
			return members[i].Priority < members[j].Priority
		}
		return members[i].BackendID < members[j].BackendID
	})
	return name, policy, members, nil
}

// poolMemberConfigs 把数据库成员转换为 Manager 的运行时配置。
func poolMemberConfigs(members []store.StoragePoolMember) []storage.PoolMemberConfig {
	out := make([]storage.PoolMemberConfig, 0, len(members))
	for _, member := range members {
		out = append(out, storage.PoolMemberConfig{
			BackendID: member.BackendID,
			Weight:    member.Weight,
			Priority:  member.Priority,
			Enabled:   member.Enabled,
		})
	}
	return out
}

// normalizePoolPolicy 规范化策略名，空值回退到加权轮询。
func normalizePoolPolicy(policy string) string {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if policy == "" {
		return storage.PoolPolicyWeightedRR
	}
	return policy
}
