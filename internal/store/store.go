// Package store provides persistence for AXmiPic metadata.
package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("store: record not found")

// Repository provides persistence operations for image metadata.
type Repository struct {
	db *gorm.DB
}

// Open opens the SQLite database at dsn, runs migrations, and returns a
// ready-to-use repository.
func Open(dsn string) (*Repository, error) {
	if err := ensureSQLiteDir(dsn); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(withSQLitePragmas(dsn)), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("store: open %q: %w", dsn, err)
	}
	if err := db.AutoMigrate(&Image{}, &PendingUpload{}, &User{}, &APIToken{}); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return &Repository{db: db}, nil
}

// Close releases the underlying database connection.
func (r *Repository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("store: connection: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}

// Create inserts image metadata.
func (r *Repository) Create(ctx context.Context, image *Image) error {
	if err := r.db.WithContext(ctx).Create(image).Error; err != nil {
		return fmt.Errorf("store: create image: %w", err)
	}
	return nil
}

// GetByID returns the image with the given id, or ErrNotFound.
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

// GetByKey returns the image stored under key, or ErrNotFound.
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

// List returns a page of images ordered by creation time (newest first) along
// with the total number of records. When userID is non-empty, only images owned
// by that user are returned.
func (r *Repository) List(ctx context.Context, userID string, offset, limit int) ([]Image, int64, error) {
	countQuery := r.db.WithContext(ctx).Model(&Image{})
	listQuery := r.db.WithContext(ctx).Model(&Image{})
	if userID != "" {
		countQuery = countQuery.Where("user_id = ?", userID)
		listQuery = listQuery.Where("user_id = ?", userID)
	}

	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("store: count images: %w", err)
	}
	var images []Image
	if err := listQuery.
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&images).Error; err != nil {
		return nil, 0, fmt.Errorf("store: list images: %w", err)
	}
	return images, total, nil
}

// Delete removes the image with the given id, or returns ErrNotFound.
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

// CreatePendingUpload records a pending presigned upload.
func (r *Repository) CreatePendingUpload(ctx context.Context, pending *PendingUpload) error {
	if err := r.db.WithContext(ctx).Create(pending).Error; err != nil {
		return fmt.Errorf("store: create pending upload: %w", err)
	}
	return nil
}

// GetPendingUpload returns the pending upload for key, or ErrNotFound.
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

// DeletePendingUpload removes a pending upload. Deleting a missing row is a
// no-op.
func (r *Repository) DeletePendingUpload(ctx context.Context, key string) error {
	if err := r.db.WithContext(ctx).Delete(&PendingUpload{}, "key = ?", key).Error; err != nil {
		return fmt.Errorf("store: delete pending upload %q: %w", key, err)
	}
	return nil
}

// ExpiredPendingUploads returns pending uploads whose expiry is before cutoff.
func (r *Repository) ExpiredPendingUploads(ctx context.Context, cutoff time.Time) ([]PendingUpload, error) {
	var pending []PendingUpload
	if err := r.db.WithContext(ctx).
		Where("expires_at < ?", cutoff).
		Find(&pending).Error; err != nil {
		return nil, fmt.Errorf("store: list expired pending uploads: %w", err)
	}
	return pending, nil
}

// CountUsers returns the number of registered accounts.
func (r *Repository) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&User{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count users: %w", err)
	}
	return count, nil
}

// CountEnabledAdmins returns the number of enabled admin accounts.
func (r *Repository) CountEnabledAdmins(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&User{}).
		Where("role = ? AND disabled = ?", "admin", false).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count enabled admins: %w", err)
	}
	return count, nil
}

// CreateUser inserts an account.
func (r *Repository) CreateUser(ctx context.Context, user *User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("store: create user: %w", err)
	}
	return nil
}

// GetUserByID returns the account with the given id, or ErrNotFound.
func (r *Repository) GetUserByID(ctx context.Context, id string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: user %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get user %q: %w", id, err)
	}
	return &user, nil
}

// GetUserByUsername returns the account with the given username, or ErrNotFound.
func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).First(&user, "username = ?", username).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("store: user %q: %w", username, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: get user by username %q: %w", username, err)
	}
	return &user, nil
}

// ReserveQuota atomically increases a user's used bytes if it stays within
// quota. It returns false when the quota would be exceeded.
func (r *Repository) ReserveQuota(ctx context.Context, userID string, amount int64) (bool, error) {
	result := r.db.WithContext(ctx).Model(&User{}).
		Where("id = ? AND (quota_bytes = 0 OR used_bytes + ? <= quota_bytes)", userID, amount).
		UpdateColumn("used_bytes", gorm.Expr("used_bytes + ?", amount))
	if result.Error != nil {
		return false, fmt.Errorf("store: reserve quota for %q: %w", userID, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// ReleaseQuota decreases a user's used bytes.
func (r *Repository) ReleaseQuota(ctx context.Context, userID string, amount int64) error {
	if err := r.db.WithContext(ctx).Model(&User{}).
		Where("id = ? AND used_bytes >= ?", userID, amount).
		UpdateColumn("used_bytes", gorm.Expr("used_bytes - ?", amount)).Error; err != nil {
		return fmt.Errorf("store: release quota for %q: %w", userID, err)
	}
	return nil
}

// CreateToken inserts an API token.
func (r *Repository) CreateToken(ctx context.Context, token *APIToken) error {
	if err := r.db.WithContext(ctx).Create(token).Error; err != nil {
		return fmt.Errorf("store: create token: %w", err)
	}
	return nil
}

// GetTokenByHash returns the token with the given hash, or ErrNotFound.
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

// ListTokensByUser returns a user's tokens, newest first.
func (r *Repository) ListTokensByUser(ctx context.Context, userID string) ([]APIToken, error) {
	var tokens []APIToken
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&tokens).Error; err != nil {
		return nil, fmt.Errorf("store: list tokens for %q: %w", userID, err)
	}
	return tokens, nil
}

// DeleteToken removes a user's token, or returns ErrNotFound.
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

// TouchToken records a token's last-used time.
func (r *Repository) TouchToken(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Model(&APIToken{}).
		Where("id = ?", id).
		UpdateColumn("last_used_at", time.Now()).Error; err != nil {
		return fmt.Errorf("store: touch token %q: %w", id, err)
	}
	return nil
}

// ListUsers returns all accounts ordered by creation time.
func (r *Repository) ListUsers(ctx context.Context) ([]User, error) {
	var users []User
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("store: list users: %w", err)
	}
	return users, nil
}

// UpdateUser applies optional field changes and returns the updated account.
func (r *Repository) UpdateUser(ctx context.Context, id string, update UserUpdate) (*User, error) {
	if _, err := r.GetUserByID(ctx, id); err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(update).Error; err != nil {
		return nil, fmt.Errorf("store: update user %q: %w", id, err)
	}
	return r.GetUserByID(ctx, id)
}

// ImageStats returns the image count and total stored bytes.
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

// withSQLitePragmas appends connection pragmas to a file-backed DSN so that
// concurrent writers wait for the lock instead of failing, and readers are not
// blocked by an in-progress write (WAL).
func withSQLitePragmas(dsn string) string {
	if dsn == "" || dsn == ":memory:" {
		return dsn
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// ensureSQLiteDir creates the parent directory for a file-backed SQLite DSN.
func ensureSQLiteDir(dsn string) error {
	if dsn == "" || dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return nil
	}
	dir := filepath.Dir(dsn)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("store: create database directory %q: %w", dir, err)
	}
	return nil
}
