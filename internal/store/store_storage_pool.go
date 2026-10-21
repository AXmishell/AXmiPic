package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// ListStoragePools 返回全部存储池，按创建时间升序。
func (r *Repository) ListStoragePools(ctx context.Context) ([]StoragePool, error) {
	var pools []StoragePool
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&pools).Error; err != nil {
		return nil, fmt.Errorf("store: list storage pools: %w", err)
	}
	return pools, nil
}

// GetStoragePool 返回指定存储池，不存在时返回 ErrNotFound。
func (r *Repository) GetStoragePool(ctx context.Context, id string) (*StoragePool, error) {
	var p StoragePool
	if err := r.first(ctx, &p, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: storage pool %q: %w", id, err)
	}
	return &p, nil
}

// CreateStoragePool 新增存储池及其成员。
func (r *Repository) CreateStoragePool(ctx context.Context, p *StoragePool, members []StoragePoolMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			return fmt.Errorf("store: create storage pool: %w", err)
		}
		if len(members) > 0 {
			if err := tx.Create(&members).Error; err != nil {
				return fmt.Errorf("store: create storage pool members: %w", err)
			}
		}
		return nil
	})
}

// UpdateStoragePool 更新存储池并整体替换其成员列表。
func (r *Repository) UpdateStoragePool(ctx context.Context, p *StoragePool, members []StoragePoolMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&StoragePool{}).Where("id = ?", p.ID).Updates(map[string]any{
			"name":   p.Name,
			"policy": p.Policy,
		})
		if result.Error != nil {
			return fmt.Errorf("store: update storage pool %q: %w", p.ID, result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("store: storage pool %q: %w", p.ID, ErrNotFound)
		}
		if err := tx.Delete(&StoragePoolMember{}, "pool_id = ?", p.ID).Error; err != nil {
			return fmt.Errorf("store: clear storage pool members: %w", err)
		}
		if len(members) > 0 {
			if err := tx.Create(&members).Error; err != nil {
				return fmt.Errorf("store: replace storage pool members: %w", err)
			}
		}
		return nil
	})
}

// DeleteStoragePool 删除存储池及其成员；不会删除底层存储后端。
func (r *Repository) DeleteStoragePool(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&StoragePoolMember{}, "pool_id = ?", id).Error; err != nil {
			return fmt.Errorf("store: delete storage pool members: %w", err)
		}
		result := tx.Delete(&StoragePool{}, "id = ?", id)
		if result.Error != nil {
			return fmt.Errorf("store: delete storage pool %q: %w", id, result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("store: storage pool %q: %w", id, ErrNotFound)
		}
		return nil
	})
}

// ListStoragePoolMembers 返回某个存储池的全部成员。
func (r *Repository) ListStoragePoolMembers(ctx context.Context, poolID string) ([]StoragePoolMember, error) {
	var members []StoragePoolMember
	if err := r.db.WithContext(ctx).Where("pool_id = ?", poolID).
		Order("priority ASC, backend_id ASC").Find(&members).Error; err != nil {
		return nil, fmt.Errorf("store: list storage pool members: %w", err)
	}
	return members, nil
}

// CountPoolMemberships 统计某后端被多少个存储池引用。
func (r *Repository) CountPoolMemberships(ctx context.Context, backendID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&StoragePoolMember{}).
		Where("backend_id = ?", backendID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count pool memberships for %q: %w", backendID, err)
	}
	return count, nil
}

// ClearCurrentStorageBackend 清除所有后端的当前默认标记。
func (r *Repository) ClearCurrentStorageBackend(ctx context.Context) error {
	if err := r.db.WithContext(ctx).Model(&StorageBackend{}).
		Where("is_current = ?", true).Update("is_current", false).Error; err != nil {
		return fmt.Errorf("store: clear current storage backend: %w", err)
	}
	return nil
}

// StorageUsage 汇总一组存储后端上的图片数量与字节数。
type StorageUsage struct {
	Count int64
	Bytes int64
}

// StorageUsageByBackend 返回指定后端的图片数量与占用字节数。
func (r *Repository) StorageUsageByBackend(ctx context.Context, ids []string) (map[string]StorageUsage, error) {
	out := make(map[string]StorageUsage, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		StorageID string
		Count     int64
		Bytes     int64
	}
	var rows []row
	if err := r.db.WithContext(ctx).Model(&Image{}).
		Select("storage_id, count(*) AS count, coalesce(sum(size), 0) AS bytes").
		Where("storage_id IN ?", ids).
		Group("storage_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: aggregate storage usage: %w", err)
	}
	for _, row := range rows {
		out[row.StorageID] = StorageUsage{Count: row.Count, Bytes: row.Bytes}
	}
	return out, nil
}

// CountImagesWithoutStorage 统计尚未记录 storage_id 的图片数量。
func (r *Repository) CountImagesWithoutStorage(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Image{}).
		Where("storage_id IS NULL OR storage_id = ''").Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count unassigned images: %w", err)
	}
	return count, nil
}

// BackfillImageStorage 把未记录 storage_id 的图片与待确认上传归入指定后端。
// 返回回填的图片数量。
func (r *Repository) BackfillImageStorage(ctx context.Context, storageID string) (int64, error) {
	if storageID == "" {
		return 0, fmt.Errorf("store: backfill storage id must not be empty")
	}
	var updated int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Image{}).
			Where("storage_id IS NULL OR storage_id = ''").
			Update("storage_id", storageID)
		if result.Error != nil {
			return fmt.Errorf("store: backfill image storage: %w", result.Error)
		}
		updated = result.RowsAffected
		if err := tx.Model(&PendingUpload{}).
			Where("storage_id IS NULL OR storage_id = ''").
			Update("storage_id", storageID).Error; err != nil {
			return fmt.Errorf("store: backfill pending upload storage: %w", err)
		}
		return nil
	})
	return updated, err
}
