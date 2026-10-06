package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	err := r.db.WithContext(ctx).Where(map[string]any{"key": key}).First(&image).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: image key %q: %w", key, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get image by key %q: %w", key, err)
	}
	return &image, nil
}

// GetByHashAndUser 返回由 ownerID 拥有、内容哈希为 hash 的图像，或 ErrNotFound。
// ownerID 为空字符串时匹配 user_id IS NULL（匿名上传）。同一所有者存在多张相同
// 内容的记录时返回最早创建的一张。它用于按所有者的内容去重。
func (r *Repository) GetByHashAndUser(ctx context.Context, hash, ownerID string) (*Image, error) {
	query := r.db.WithContext(ctx).Where("hash = ?", hash)
	if ownerID == "" {
		query = query.Where("user_id IS NULL")
	} else {
		query = query.Where("user_id = ?", ownerID)
	}
	var image Image
	err := query.Order("created_at ASC").First(&image).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: image hash %q: %w", hash, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get image by hash %q: %w", hash, err)
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
	// Permission 非空时仅返回该可见性的图片（public/private）。
	Permission string
	// AlbumID 非空时仅返回属于该相册的图片。
	AlbumID *string
	// Cursor 非空时使用 keyset 游标翻页（Offset 被忽略），并跳过总数统计。
	Cursor *ImageCursor
}

// ImageCursor 是一个 keyset 翻页锚点。Order 记录生成该游标的排序方式，仅在
// 与本次查询的排序一致时生效。
type ImageCursor struct {
	Order     string
	CreatedAt time.Time
	Size      int64
	ID        string
}

// ListImages 返回一页图片以及记录总数。排序与关键字由 opts 控制。
// 使用游标翻页时跳过总数统计（返回 0），以避免昂贵的全表 COUNT。
func (r *Repository) ListImages(ctx context.Context, opts ImageListOptions) ([]Image, int64, error) {
	order := canonicalOrder(opts.Order)
	countQuery := r.db.WithContext(ctx).Model(&Image{})
	listQuery := r.db.WithContext(ctx).Model(&Image{})
	if opts.UserID != "" {
		countQuery = countQuery.Where("user_id = ?", opts.UserID)
		listQuery = listQuery.Where("images.user_id = ?", opts.UserID)
	}
	if opts.Permission != "" {
		countQuery = countQuery.Where("permission = ?", opts.Permission)
		listQuery = listQuery.Where("images.permission = ?", opts.Permission)
	}
	if opts.AlbumID != nil {
		countQuery = countQuery.Where("album_id = ?", *opts.AlbumID)
		listQuery = listQuery.Where("images.album_id = ?", *opts.AlbumID)
	}
	if opts.Keyword != "" {
		like := "%" + opts.Keyword + "%"
		countQuery = countQuery.Where(clause.Or(
			clause.Like{Column: clause.Column{Name: "original_name"}, Value: like},
			clause.Like{Column: clause.Column{Name: "filename"}, Value: like},
			clause.Like{Column: clause.Column{Name: "key"}, Value: like},
		))
		listQuery = listQuery.Where(clause.Or(
			clause.Like{Column: clause.Column{Table: "images", Name: "original_name"}, Value: like},
			clause.Like{Column: clause.Column{Table: "images", Name: "filename"}, Value: like},
			clause.Like{Column: clause.Column{Table: "images", Name: "key"}, Value: like},
		))
	}

	useCursor := opts.Cursor != nil && opts.Cursor.Order == order
	offset := opts.Offset
	if useCursor {
		listQuery = listQuery.Where(keysetClause(order), keysetArgs(order, opts.Cursor)...)
		offset = 0
	}

	var total int64
	if !useCursor {
		if err := countQuery.Count(&total).Error; err != nil {
			return nil, 0, fmt.Errorf("store: count images: %w", err)
		}
	}
	var images []Image
	if err := listQuery.
		Order(orderClause(order)).
		Offset(offset).
		Limit(opts.Limit).
		Find(&images).Error; err != nil {
		return nil, 0, fmt.Errorf("store: list images: %w", err)
	}
	return images, total, nil
}

// canonicalOrder 把排序标识收敛到白名单中的一个已知取值。
func canonicalOrder(order string) string {
	switch order {
	case "earliest", "largest", "smallest":
		return order
	default:
		return "newest"
	}
}

// keysetClause 返回给定排序方式下基于 (排序列, id) 行值的游标比较子句。
// 加入 id 作为决胜字段，保证排序稳定、翻页不漏不重。
func keysetClause(order string) string {
	switch order {
	case "earliest":
		return "(images.created_at, images.id) > (?, ?)"
	case "largest":
		return "(images.size, images.id) < (?, ?)"
	case "smallest":
		return "(images.size, images.id) > (?, ?)"
	default:
		return "(images.created_at, images.id) < (?, ?)"
	}
}

// keysetArgs 返回 keysetClause 对应的绑定参数。
func keysetArgs(order string, cursor *ImageCursor) []any {
	switch order {
	case "largest", "smallest":
		return []any{cursor.Size, cursor.ID}
	default:
		return []any{cursor.CreatedAt, cursor.ID}
	}
}

// orderClause 将排序标识映射为安全的 SQL 排序子句（白名单，避免注入）。
// 每种排序都以 id 作为决胜字段，保证游标翻页稳定。
func orderClause(order string) string {
	switch order {
	case "earliest":
		return "images.created_at ASC, images.id ASC"
	case "largest":
		return "images.size DESC, images.id DESC"
	case "smallest":
		return "images.size ASC, images.id ASC"
	default:
		return "images.created_at DESC, images.id DESC"
	}
}

// ListImagesByIDs 返回具有给定 id 的图片。它用于批量操作前的所有权校验。
func (r *Repository) ListImagesByIDs(ctx context.Context, ids []string) ([]Image, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var images []Image
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&images).Error; err != nil {
		return nil, fmt.Errorf("store: list images by ids: %w", err)
	}
	return images, nil
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
	err := r.db.WithContext(ctx).Where(map[string]any{"key": key}).First(&pending).Error
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
	if err := r.db.WithContext(ctx).Where(map[string]any{"key": key}).Delete(&PendingUpload{}).Error; err != nil {
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
