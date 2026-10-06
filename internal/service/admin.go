package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AXmishell/axmipic/internal/imaging"
	"github.com/AXmishell/axmipic/internal/store"
)

// statsCacheTTL 是后台统计结果的缓存时长，避免每次刷新都做全表 COUNT/SUM。
const statsCacheTTL = 30 * time.Second

// StatsDTO 汇总实例级别的指标。
type StatsDTO struct {
	Admins        int64    `json:"admins"`
	Customers     int64    `json:"customers"`
	Users         int64    `json:"users"`
	Images        int64    `json:"images"`
	TotalBytes    int64    `json:"total_bytes"`
	StorageDriver string   `json:"storage_driver"`
	Processor     string   `json:"processor"`
	Formats       []string `json:"formats"`
}

// UpdateUserInput 携带可选的账户变更。
type UpdateUserInput struct {
	Disabled *bool
	// RoleGroupID 非 nil 时把客户转移到该角色组；空字符串表示回退默认组。
	RoleGroupID *string
}

// AdminService 针对两张账户表提供管理操作。
type AdminService struct {
	repo          *store.Repository
	storageDriver string
	processor     imaging.Processor
	policies      *PolicyService

	// purger 在删除客户时清理其图片与存储对象；未安装时仅删除账户记录。
	purger ImagePurger

	statsMu     sync.Mutex
	statsCache  *StatsDTO
	statsExpiry time.Time
}

// ImagePurger 负责清理某个用户拥有的全部图片与存储对象。
type ImagePurger interface {
	PurgeOwner(ctx context.Context, userID string) (int, error)
}

// NewAdminService 构造一个 AdminService。
func NewAdminService(repo *store.Repository, storageDriver string, processor imaging.Processor) *AdminService {
	return &AdminService{repo: repo, storageDriver: storageDriver, processor: processor}
}

// SetPolicyService 安装角色组/策略服务，使管理员能为客户分配角色组。
func (s *AdminService) SetPolicyService(policies *PolicyService) {
	s.policies = policies
}

// SetImagePurger 安装图片清理器，使删除客户时能级联清理其图片与存储对象。
func (s *AdminService) SetImagePurger(purger ImagePurger) {
	s.purger = purger
}

// Stats 返回聚合的实例指标。结果会被短暂缓存，避免频繁的全表扫描。
func (s *AdminService) Stats(ctx context.Context) (*StatsDTO, error) {
	s.statsMu.Lock()
	if s.statsCache != nil && time.Now().Before(s.statsExpiry) {
		cached := s.statsCache
		s.statsMu.Unlock()
		return cached, nil
	}
	s.statsMu.Unlock()

	stats, err := s.computeStats(ctx)
	if err != nil {
		return nil, err
	}
	s.statsMu.Lock()
	s.statsCache = stats
	s.statsExpiry = time.Now().Add(statsCacheTTL)
	s.statsMu.Unlock()
	return stats, nil
}

// computeStats 实际聚合实例指标。
func (s *AdminService) computeStats(ctx context.Context) (*StatsDTO, error) {
	admins, err := s.repo.CountAdmins(ctx)
	if err != nil {
		return nil, fmt.Errorf("stats: count admins: %w", err)
	}
	customers, err := s.repo.CountCustomers(ctx)
	if err != nil {
		return nil, fmt.Errorf("stats: count customers: %w", err)
	}
	imageStats, err := s.repo.ImageStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("stats: image stats: %w", err)
	}

	formats := []string{}
	processorName := ""
	if s.processor != nil {
		capabilities := s.processor.Capabilities()
		processorName = capabilities.Name
		for _, f := range capabilities.OutputFormats {
			formats = append(formats, string(f))
		}
	}
	return &StatsDTO{
		Admins:        admins,
		Customers:     customers,
		Users:         admins + customers,
		Images:        imageStats.Count,
		TotalBytes:    imageStats.TotalBytes,
		StorageDriver: s.storageDriver,
		Processor:     processorName,
		Formats:       formats,
	}, nil
}

// ListCustomers 返回所有客户账户。
func (s *AdminService) ListCustomers(ctx context.Context) ([]UserDTO, error) {
	accounts, err := s.repo.ListCustomers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	return toUserDTOs(accounts), nil
}

// ListAdmins 返回所有管理员账户。
func (s *AdminService) ListAdmins(ctx context.Context) ([]UserDTO, error) {
	accounts, err := s.repo.ListAdmins(ctx)
	if err != nil {
		return nil, fmt.Errorf("list admins: %w", err)
	}
	return toUserDTOs(accounts), nil
}

// RegisterAdmin 在 admins 表中创建一个管理员。
func (s *AdminService) RegisterAdmin(ctx context.Context, accounts *AccountService, username, password string) (*UserDTO, error) {
	return accounts.RegisterAdmin(ctx, username, password)
}

// UpdateCustomer 对某个客户账户应用可选变更。当角色组发生变化时，会依据
// 新角色组的配额策略同步该客户的存储配额。
func (s *AdminService) UpdateCustomer(ctx context.Context, id string, in UpdateUserInput) (*UserDTO, error) {
	update := store.UserUpdate{Disabled: in.Disabled}
	if in.RoleGroupID != nil {
		groupID := strings.TrimSpace(*in.RoleGroupID)
		if groupID == "" {
			// 清空角色组：回退默认组，并同步其配额。
			update.RoleGroupID = &groupID
			if s.policies != nil {
				if effective, err := s.policies.ResolveDefault(ctx); err == nil {
					update.QuotaBytes = &effective.QuotaBytes
				}
			}
		} else {
			if s.policies == nil {
				return nil, fmt.Errorf("%w: role groups are not available", ErrInvalidInput)
			}
			effective, err := s.policies.ResolveForRoleGroupID(ctx, groupID)
			if err != nil {
				return nil, err
			}
			update.RoleGroupID = &groupID
			update.QuotaBytes = &effective.QuotaBytes
		}
	}
	account, err := s.repo.UpdateCustomer(ctx, id, update)
	if err != nil {
		return nil, fmt.Errorf("update customer: %w", err)
	}
	return toUserDTO(account), nil
}

// UpdateAdmin 对某个管理员账户应用可选变更。它拒绝禁用操作者本人或最后
// 一个已启用的管理员，以免实例被锁死。
func (s *AdminService) UpdateAdmin(ctx context.Context, actorID, id string, in UpdateUserInput) (*UserDTO, error) {
	target, err := s.repo.GetAccountByID(ctx, store.RoleAdmin, id)
	if err != nil {
		return nil, fmt.Errorf("update admin: %w", err)
	}
	disabling := in.Disabled != nil && *in.Disabled && !target.Disabled
	if disabling {
		if id == actorID {
			return nil, fmt.Errorf("%w: cannot disable your own account", ErrInvalidInput)
		}
		count, err := s.repo.CountEnabledAdmins(ctx)
		if err != nil {
			return nil, fmt.Errorf("update admin: count admins: %w", err)
		}
		if count <= 1 {
			return nil, fmt.Errorf("%w: cannot disable the last enabled admin", ErrInvalidInput)
		}
	}
	account, err := s.repo.UpdateAdmin(ctx, id, store.UserUpdate{Disabled: in.Disabled})
	if err != nil {
		return nil, fmt.Errorf("update admin: %w", err)
	}
	return toUserDTO(account), nil
}

// DeleteCustomer 移除某个客户账户。
func (s *AdminService) DeleteCustomer(ctx context.Context, id string) error {
	if _, err := s.repo.GetAccountByID(ctx, store.RoleCustomer, id); err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}
	// 先清理该用户的内容，避免留下孤儿图片记录与孤儿存储对象。
	if s.purger != nil {
		if _, err := s.purger.PurgeOwner(ctx, id); err != nil {
			return fmt.Errorf("delete customer: purge content: %w", err)
		}
	} else if _, err := s.repo.DeleteImagesByUser(ctx, id); err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}
	// 移除其余从属记录。这里尽力而为：即使部分失败也不阻塞账户删除。
	_, _ = s.repo.DeleteSharesByUser(ctx, id)
	_, _ = s.repo.DeleteAlbumsByUser(ctx, id)
	_, _ = s.repo.DeleteTokensByUser(ctx, id)
	if err := s.repo.DeleteAccount(ctx, store.RoleCustomer, id); err != nil {
		return fmt.Errorf("delete customer: %w", err)
	}
	s.invalidateStats()
	return nil
}

// invalidateStats 使缓存的后台统计立即失效。
func (s *AdminService) invalidateStats() {
	s.statsMu.Lock()
	s.statsCache = nil
	s.statsExpiry = time.Time{}
	s.statsMu.Unlock()
}

// DeleteAdmin 移除某个管理员账户。它拒绝删除操作者本人或最后一个已启用
// 的管理员。
func (s *AdminService) DeleteAdmin(ctx context.Context, actorID, id string) error {
	target, err := s.repo.GetAccountByID(ctx, store.RoleAdmin, id)
	if err != nil {
		return fmt.Errorf("delete admin: %w", err)
	}
	if id == actorID {
		return fmt.Errorf("%w: cannot delete your own account", ErrInvalidInput)
	}
	if !target.Disabled {
		count, err := s.repo.CountEnabledAdmins(ctx)
		if err != nil {
			return fmt.Errorf("delete admin: count admins: %w", err)
		}
		if count <= 1 {
			return fmt.Errorf("%w: cannot delete the last enabled admin", ErrInvalidInput)
		}
	}
	if err := s.repo.DeleteAccount(ctx, store.RoleAdmin, id); err != nil {
		return fmt.Errorf("delete admin: %w", err)
	}
	return nil
}

func toUserDTOs(accounts []store.Account) []UserDTO {
	dtos := make([]UserDTO, 0, len(accounts))
	for i := range accounts {
		dtos = append(dtos, *toUserDTO(&accounts[i]))
	}
	return dtos
}
