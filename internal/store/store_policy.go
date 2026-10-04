package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// CreateRoleGroup 插入一个角色组。
func (r *Repository) CreateRoleGroup(ctx context.Context, group *RoleGroup) error {
	if err := r.db.WithContext(ctx).Create(group).Error; err != nil {
		return fmt.Errorf("store: create role group: %w", err)
	}
	return nil
}

// GetRoleGroupByID 返回具有给定 id 的角色组，或 ErrNotFound。
func (r *Repository) GetRoleGroupByID(ctx context.Context, id string) (*RoleGroup, error) {
	var group RoleGroup
	if err := r.first(ctx, &group, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: role group %q: %w", id, err)
	}
	return &group, nil
}

// GetDefaultRoleGroup 返回被标记为默认的角色组；当没有角色组被标记时，
// 返回 ErrNotFound。
func (r *Repository) GetDefaultRoleGroup(ctx context.Context) (*RoleGroup, error) {
	var group RoleGroup
	if err := r.first(ctx, &group, "is_default = ?", true); err != nil {
		return nil, fmt.Errorf("store: default role group: %w", err)
	}
	return &group, nil
}

// RoleGroupWithCount 是角色组及其关联的客户数与策略数。
type RoleGroupWithCount struct {
	RoleGroup     RoleGroup
	CustomerCount int64
	PolicyCount   int64
}

// ListRoleGroups 返回全部角色组（按创建时间），并附带客户数与策略数。
func (r *Repository) ListRoleGroups(ctx context.Context) ([]RoleGroupWithCount, error) {
	var groups []RoleGroup
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("store: list role groups: %w", err)
	}
	// 单次聚合查询统计每个角色组的客户数，避免 N+1。
	customerCounts := map[string]int64{}
	type countRow struct {
		RoleGroupID string
		Count       int64
	}
	var rows []countRow
	if err := r.db.WithContext(ctx).Model(&Customer{}).
		Select("role_group_id, COUNT(*) AS count").
		Where("role_group_id IS NOT NULL").
		Group("role_group_id").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: count role group customers: %w", err)
	}
	for _, row := range rows {
		customerCounts[row.RoleGroupID] = row.Count
	}
	policyCounts := map[string]int64{}
	var policyRows []countRow
	if err := r.db.WithContext(ctx).Model(&RoleGroupPolicy{}).
		Select("role_group_id, COUNT(*) AS count").
		Group("role_group_id").
		Scan(&policyRows).Error; err != nil {
		return nil, fmt.Errorf("store: count role group policies: %w", err)
	}
	for _, row := range policyRows {
		policyCounts[row.RoleGroupID] = row.Count
	}
	result := make([]RoleGroupWithCount, 0, len(groups))
	for i := range groups {
		result = append(result, RoleGroupWithCount{
			RoleGroup:     groups[i],
			CustomerCount: customerCounts[groups[i].ID],
			PolicyCount:   policyCounts[groups[i].ID],
		})
	}
	return result, nil
}

// RoleGroupUpdate 携带可选的字段更改。Nil 字段将被忽略。
type RoleGroupUpdate struct {
	Name        *string
	Description *string
	IsDefault   *bool
}

// UpdateRoleGroup 应用可选字段更改并返回更新后的记录。当 IsDefault 被设为
// true 时，会在同一事务中清除其他角色组的默认标记，保证全库至多一个默认组。
func (r *Repository) UpdateRoleGroup(ctx context.Context, id string, update RoleGroupUpdate) (*RoleGroup, error) {
	return r.updateRoleGroup(ctx, id, update, true)
}

// updateRoleGroup 在 withDefault 为 true 时处理默认组唯一性；内部复用。
func (r *Repository) updateRoleGroup(ctx context.Context, id string, update RoleGroupUpdate, withDefault bool) (*RoleGroup, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if update.IsDefault != nil && *update.IsDefault && withDefault {
			if err := tx.Model(&RoleGroup{}).Where("id <> ?", id).
				UpdateColumn("is_default", false).Error; err != nil {
				return err
			}
		}
		fields := map[string]any{}
		if update.Name != nil {
			fields["name"] = *update.Name
		}
		if update.Description != nil {
			fields["description"] = *update.Description
		}
		if update.IsDefault != nil {
			fields["is_default"] = *update.IsDefault
		}
		if len(fields) == 0 {
			return nil
		}
		result := tx.Model(&RoleGroup{}).Where("id = ?", id).Updates(fields)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			// 记录可能已存在但没有任何字段发生变化。
			var exists int64
			if err := tx.Model(&RoleGroup{}).Where("id = ?", id).Count(&exists).Error; err != nil {
				return err
			}
			if exists == 0 {
				return ErrNotFound
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("store: role group %q: %w", id, err)
		}
		return nil, fmt.Errorf("store: update role group %q: %w", id, err)
	}
	return r.GetRoleGroupByID(ctx, id)
}

// DeleteRoleGroup 删除一个角色组；当仍有客户属于该组时返回 ErrRoleGroupInUse。
func (r *Repository) DeleteRoleGroup(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var customers int64
		if err := tx.Model(&Customer{}).Where("role_group_id = ?", id).Count(&customers).Error; err != nil {
			return err
		}
		if customers > 0 {
			return ErrRoleGroupInUse
		}
		if err := tx.Where("role_group_id = ?", id).Delete(&RoleGroupPolicy{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&RoleGroup{}, "id = ?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// CountCustomersInRoleGroup 统计属于某个角色组的客户数量。
func (r *Repository) CountCustomersInRoleGroup(ctx context.Context, id string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Customer{}).
		Where("role_group_id = ?", id).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count role group customers: %w", err)
	}
	return count, nil
}

// ClearDefaultRoleGroups 清除所有角色组的默认标记，供设置新的默认组前调用。
func (r *Repository) ClearDefaultRoleGroups(ctx context.Context) error {
	if err := r.db.WithContext(ctx).Model(&RoleGroup{}).
		Where("is_default = ?", true).
		UpdateColumn("is_default", false).Error; err != nil {
		return fmt.Errorf("store: clear default role groups: %w", err)
	}
	return nil
}

// CreatePolicy 插入一个策略。
func (r *Repository) CreatePolicy(ctx context.Context, policy *Policy) error {
	if err := r.db.WithContext(ctx).Create(policy).Error; err != nil {
		return fmt.Errorf("store: create policy: %w", err)
	}
	return nil
}

// GetPolicyByID 返回具有给定 id 的策略，或 ErrNotFound。
func (r *Repository) GetPolicyByID(ctx context.Context, id string) (*Policy, error) {
	var policy Policy
	if err := r.first(ctx, &policy, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: policy %q: %w", id, err)
	}
	return &policy, nil
}

// ListPolicies 返回全部策略，并可按可选的类型过滤。
func (r *Repository) ListPolicies(ctx context.Context, policyType string) ([]Policy, error) {
	query := r.db.WithContext(ctx).Order("created_at ASC")
	if policyType != "" {
		query = query.Where("type = ?", policyType)
	}
	var policies []Policy
	if err := query.Find(&policies).Error; err != nil {
		return nil, fmt.Errorf("store: list policies: %w", err)
	}
	return policies, nil
}

// PolicyUpdate 携带可选的策略字段更改。Nil 字段将被忽略。
type PolicyUpdate struct {
	Name        *string
	Type        *string
	Description *string
	Enabled     *bool
	Settings    *string
}

// UpdatePolicy 应用可选字段更改并返回更新后的记录。
func (r *Repository) UpdatePolicy(ctx context.Context, id string, update PolicyUpdate) (*Policy, error) {
	fields := map[string]any{}
	if update.Name != nil {
		fields["name"] = *update.Name
	}
	if update.Type != nil {
		fields["type"] = *update.Type
	}
	if update.Description != nil {
		fields["description"] = *update.Description
	}
	if update.Enabled != nil {
		fields["enabled"] = *update.Enabled
	}
	if update.Settings != nil {
		fields["settings"] = *update.Settings
	}
	if len(fields) > 0 {
		result := r.db.WithContext(ctx).Model(&Policy{}).Where("id = ?", id).Updates(fields)
		if result.Error != nil {
			return nil, fmt.Errorf("store: update policy %q: %w", id, result.Error)
		}
		if result.RowsAffected == 0 {
			var exists int64
			if err := r.db.WithContext(ctx).Model(&Policy{}).Where("id = ?", id).Count(&exists).Error; err != nil {
				return nil, fmt.Errorf("store: update policy %q: %w", id, err)
			}
			if exists == 0 {
				return nil, fmt.Errorf("store: policy %q: %w", id, ErrNotFound)
			}
		}
	}
	return r.GetPolicyByID(ctx, id)
}

// DeletePolicy 删除一个策略，并移除其与角色组的所有关联。当策略仍被某个
// 角色组引用时返回 ErrPolicyInUse。
func (r *Repository) DeletePolicy(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var links int64
		if err := tx.Model(&RoleGroupPolicy{}).Where("policy_id = ?", id).Count(&links).Error; err != nil {
			return err
		}
		if links > 0 {
			return ErrPolicyInUse
		}
		result := tx.Delete(&Policy{}, "id = ?", id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ListPoliciesByRoleGroup 返回绑定到某个角色组的策略（按类型排序）。
func (r *Repository) ListPoliciesByRoleGroup(ctx context.Context, roleGroupID string) ([]Policy, error) {
	var policies []Policy
	if err := r.db.WithContext(ctx).
		Model(&Policy{}).
		Joins("JOIN role_group_policies ON role_group_policies.policy_id = policies.id").
		Where("role_group_policies.role_group_id = ?", roleGroupID).
		Order("policies.type ASC").
		Find(&policies).Error; err != nil {
		return nil, fmt.Errorf("store: list role group policies: %w", err)
	}
	return policies, nil
}

// AttachPolicy 把策略绑定到角色组。为保证「每个类型至多一个策略」的确定性，
// 若该组已绑定同类型的策略，则先移除旧关联再建立新关联。
func (r *Repository) AttachPolicy(ctx context.Context, roleGroupID, policyID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var target Policy
		if err := tx.First(&target, "id = ?", policyID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if err := tx.Where("role_group_id = ?", roleGroupID).
			Where("policy_id IN (?)", tx.Model(&Policy{}).Select("id").Where("type = ?", target.Type)).
			Delete(&RoleGroupPolicy{}).Error; err != nil {
			return err
		}
		link := &RoleGroupPolicy{RoleGroupID: roleGroupID, PolicyID: policyID}
		if err := tx.Create(link).Error; err != nil {
			return err
		}
		return nil
	})
}

// DetachPolicy 移除角色组与策略之间的关联。删除不存在的关联是无操作。
func (r *Repository) DetachPolicy(ctx context.Context, roleGroupID, policyID string) error {
	if err := r.db.WithContext(ctx).
		Where("role_group_id = ? AND policy_id = ?", roleGroupID, policyID).
		Delete(&RoleGroupPolicy{}).Error; err != nil {
		return fmt.Errorf("store: detach policy: %w", err)
	}
	return nil
}

// MigrateCustomerRoleGroups 把 role_group_id 为空的历史客户统一归入给定角色组。
// 它返回被更新的行数。
func (r *Repository) MigrateCustomerRoleGroups(ctx context.Context, roleGroupID string) (int64, error) {
	result := r.db.WithContext(ctx).Model(&Customer{}).
		Where("role_group_id IS NULL").
		Update("role_group_id", roleGroupID)
	if result.Error != nil {
		return 0, fmt.Errorf("store: migrate customer role groups: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// ListRoleGroupIDsByPolicy 返回引用了某个策略的全部角色组 id。
func (r *Repository) ListRoleGroupIDsByPolicy(ctx context.Context, policyID string) ([]string, error) {
	var ids []string
	if err := r.db.WithContext(ctx).Model(&RoleGroupPolicy{}).
		Where("policy_id = ?", policyID).
		Pluck("role_group_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("store: list role groups for policy: %w", err)
	}
	return ids, nil
}

// SetCustomersQuotaByRoleGroup 把某个角色组（以及在 includeUnassigned 为真时
// 未指定角色组）的全部客户的配额批量设为 quota。它返回被更新的行数。
func (r *Repository) SetCustomersQuotaByRoleGroup(ctx context.Context, roleGroupID string, quota int64, includeUnassigned bool) (int64, error) {
	query := r.db.WithContext(ctx).Model(&Customer{})
	if includeUnassigned {
		query = query.Where("role_group_id = ? OR role_group_id IS NULL", roleGroupID)
	} else {
		query = query.Where("role_group_id = ?", roleGroupID)
	}
	result := query.Update("quota_bytes", quota)
	if result.Error != nil {
		return 0, fmt.Errorf("store: set customers quota for role group %q: %w", roleGroupID, result.Error)
	}
	return result.RowsAffected, nil
}

// 由角色组/策略层使用的 sentinel 错误。
var (
	// ErrRoleGroupInUse 在角色组仍被客户引用时返回。
	ErrRoleGroupInUse = errors.New("store: role group is still assigned to customers")
	// ErrPolicyInUse 在策略仍被角色组引用时返回。
	ErrPolicyInUse = errors.New("store: policy is still attached to a role group")
)
