// Package store 为 AXmiPic 元数据提供持久化。
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ErrNotFound 在请求的记录不存在时返回。
var ErrNotFound = errors.New("store: record not found")

// Repository 提供元数据的持久化操作。
type Repository struct {
	db *gorm.DB
}

// Open 打开由 driver 选择的数据库，运行迁移，并返回一个即用型的
// repository。支持的驱动有 "sqlite"（默认）和 "postgres"。dsn 对于
// sqlite 是文件路径，对于 postgres 是 libpq 连接字符串（或 URL）。
func Open(driver, dsn string) (*Repository, error) {
	dialector, err := newDialector(driver, dsn)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("store: open (%s): %w", driver, err)
	}
	if err := db.AutoMigrate(
		&Image{}, &PendingUpload{}, &Admin{}, &Customer{}, &APIToken{},
		&StorageBackend{}, &Album{}, &RoleGroup{}, &Policy{}, &RoleGroupPolicy{},
		&Share{}, &Announcement{}, &Report{}, &Page{}, &Setting{},
		&Plan{}, &Order{}, &Coupon{}, &CouponRedemption{}, &Ticket{}, &TicketMessage{},
		&EmailCode{}, &EmailCodeStat{}, &NotifyLog{}, &GuestIPUsage{},
		&UserPreference{},
	); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return &Repository{db: db}, nil
}

// newDialector 为请求的驱动构建 gorm dialector。
func newDialector(driver, dsn string) (gorm.Dialector, error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", "sqlite":
		if err := ensureSQLiteDir(dsn); err != nil {
			return nil, err
		}
		return sqlite.Open(withSQLitePragmas(dsn)), nil
	case "postgres", "postgresql", "pgx":
		return postgres.Open(dsn), nil
	default:
		return nil, fmt.Errorf("store: unsupported database driver %q", driver)
	}
}

// Close 释放底层数据库连接。
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

// withSQLitePragmas 向基于文件的 DSN 追加连接 pragma，使并发写入者等待锁
// 而不是失败，并且读者不会被进行中的写入阻塞（WAL）。
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

// ensureSQLiteDir 为基于文件的 SQLite DSN 创建父目录。
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
