package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Create 插入图像元数据。
func (r *Repository) Create(ctx context.Context, image *Image) error {
	if err := r.db.WithContext(ctx).Create(image).Error; err != nil {
		return fmt.Errorf("store: create image: %w", err)
	}
	return nil
}

// GetByID 返回具有给定 id 的图像，或 ErrNotFound。
func (r *Repository) GetByID(ctx context.Context, id string) (*Image, error) {
	var image Image
	err := r.db.WithContext(ctx).First(&image, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: image %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get image %q: %w", id, err)
	}
	return &image, nil
}

// GetByKey 返回存储在 key 下的图像，或 ErrNotFound。
func (r *Repository) GetByKey(ctx context.Context, key string) (*Image, error) {
	var image Image
	err := r.db.WithContext(ctx).First(&image, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: image key %q: %w", key, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get image by key %q: %w", key, err)
	}
	return &image, nil
}

// ImageListOptions 约束图片列表查询。
type ImageListOptions struct {
	// UserID 非空时仅返回该用户拥有的图片。
	UserID string
	Offset int
	Limit  int
	// Order 为排序方式：newest（默认，最新在前）、earliest、largest、smallest。
	Order string
	// Keyword 非空时按原文件名、存储文件名或键进行模糊匹配。
	Keyword string
}

// ListImages 返回一页图片以及记录总数。排序与关键字由 opts 控制。
func (r *Repository) ListImages(ctx context.Context, opts ImageListOptions) ([]Image, int64, error) {
	countQuery := r.db.WithContext(ctx).Model(&Image{})
	listQuery := r.db.WithContext(ctx).Model(&Image{})
	if opts.UserID != "" {
		countQuery = countQuery.Where("user_id = ?", opts.UserID)
		listQuery = listQuery.Where("user_id = ?", opts.UserID)
	}
	if opts.Keyword != "" {
		like := "%" + opts.Keyword + "%"
		cond := "original_name LIKE ? OR filename LIKE ? OR key LIKE ?"
		countQuery = countQuery.Where(cond, like, like, like)
		listQuery = listQuery.Where(cond, like, like, like)
	}

	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("store: count images: %w", err)
	}
	var images []Image
	if err := listQuery.
		Order(orderClause(opts.Order)).
		Offset(opts.Offset).
		Limit(opts.Limit).
		Find(&images).Error; err != nil {
		return nil, 0, fmt.Errorf("store: list images: %w", err)
	}
	return images, total, nil
}

// orderClause 将排序标识映射为安全的 SQL 排序子句（白名单，避免注入）。
func orderClause(order string) string {
	switch order {
	case "earliest":
		return "created_at ASC"
	case "largest":
		return "size DESC"
	case "smallest":
		return "size ASC"
	default:
		return "created_at DESC"
	}
}

// RenameImage 更新图片的原文件名并返回更新后的记录；name 由调用方负责校验。
func (r *Repository) RenameImage(ctx context.Context, id, name string) (*Image, error) {
	result := r.db.WithContext(ctx).Model(&Image{}).Where("id = ?", id).
		UpdateColumn("original_name", name)
	if result.Error != nil {
		return nil, fmt.Errorf("store: rename image %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("store: image %q: %w", id, ErrNotFound)
	}
	return r.GetByID(ctx, id)
}

// Delete 移除具有给定 id 的图像，或返回 ErrNotFound。
func (r *Repository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&Image{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("store: delete image %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: image %q: %w", id, ErrNotFound)
	}
	return nil
}

// CreatePendingUpload 记录一个待处理的预签名上传。
func (r *Repository) CreatePendingUpload(ctx context.Context, pending *PendingUpload) error {
	if err := r.db.WithContext(ctx).Create(pending).Error; err != nil {
		return fmt.Errorf("store: create pending upload: %w", err)
	}
	return nil
}

// GetPendingUpload 返回 key 对应的待处理上传，或 ErrNotFound。
func (r *Repository) GetPendingUpload(ctx context.Context, key string) (*PendingUpload, error) {
	var pending PendingUpload
	err := r.db.WithContext(ctx).First(&pending, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: pending upload %q: %w", key, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get pending upload %q: %w", key, err)
	}
	return &pending, nil
}

// DeletePendingUpload 移除一个待处理上传。删除不存在的记录是
// 无操作。
func (r *Repository) DeletePendingUpload(ctx context.Context, key string) error {
	if err := r.db.WithContext(ctx).Delete(&PendingUpload{}, "key = ?", key).Error; err != nil {
		return fmt.Errorf("store: delete pending upload %q: %w", key, err)
	}
	return nil
}

// ExpiredPendingUploads 返回到期时间早于 cutoff 的待处理上传。
func (r *Repository) ExpiredPendingUploads(ctx context.Context, cutoff time.Time) ([]PendingUpload, error) {
	var pending []PendingUpload
	if err := r.db.WithContext(ctx).
		Where("expires_at < ?", cutoff).
		Find(&pending).Error; err != nil {
		return nil, fmt.Errorf("store: list expired pending uploads: %w", err)
	}
	return pending, nil
}

// CreateToken 插入一个 API 令牌。
func (r *Repository) CreateToken(ctx context.Context, token *APIToken) error {
	if err := r.db.WithContext(ctx).Create(token).Error; err != nil {
		return fmt.Errorf("store: create token: %w", err)
	}
	return nil
}

// GetTokenByHash 返回具有给定哈希的令牌，或 ErrNotFound。
func (r *Repository) GetTokenByHash(ctx context.Context, hash string) (*APIToken, error) {
	var token APIToken
	err := r.db.WithContext(ctx).First(&token, "token_hash = ?", hash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: token: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get token: %w", err)
	}
	return &token, nil
}

// ListTokensByUser 返回某个用户的令牌，最新的在前。
func (r *Repository) ListTokensByUser(ctx context.Context, userID string) ([]APIToken, error) {
	var tokens []APIToken
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error; err != nil {
		return nil, fmt.Errorf("store: list tokens for %q: %w", userID, err)
	}
	return tokens, nil
}

// DeleteToken 移除某个用户的令牌，或返回 ErrNotFound。
func (r *Repository) DeleteToken(ctx context.Context, userID, id string) error {
	result := r.db.WithContext(ctx).Delete(&APIToken{}, "id = ? AND user_id = ?", id, userID)
	if result.Error != nil {
		return fmt.Errorf("store: delete token %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: token %q: %w", id, ErrNotFound)
	}
	return nil
}

// TouchToken 记录某个令牌的最后使用时间。
func (r *Repository) TouchToken(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&APIToken{}).
		Where("id = ?", id).
		UpdateColumn("last_used_at", time.Now()).Error; err != nil {
		return fmt.Errorf("store: touch token %q: %w", id, err)
	}
	return nil
}

// ImageStats 返回图像数量和已存储的总字节数。
func (r *Repository) ImageStats(ctx context.Context) (ImageStats, error) {
	var stats ImageStats
	if err := r.db.WithContext(ctx).Model(&Image{}).Count(&stats.Count).Error; err != nil {
		return ImageStats{}, fmt.Errorf("store: count images: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&Image{}).
		Select("COALESCE(SUM(size), 0)").
		Scan(&stats.TotalBytes).Error; err != nil {
		return ImageStats{}, fmt.Errorf("store: sum image size: %w", err)
	}
	return stats, nil
}
