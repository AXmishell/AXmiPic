// Package store provides persistence for AXmiPic metadata.
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

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("store: record not found")

// Repository provides persistence operations for metadata.
type Repository struct {
	db *gorm.DB
}

// Open opens the database selected by driver, runs migrations, and returns a
// ready-to-use repository. Supported drivers are "sqlite" (default) and
// "postgres". The dsn is a file path for sqlite or a libpq connection string
// (or URL) for postgres.
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
	if err := db.AutoMigrate(&Image{}, &PendingUpload{}, &Admin{}, &Customer{}, &APIToken{}); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return &Repository{db: db}, nil
}

// newDialector builds the gorm dialector for the requested driver.
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
