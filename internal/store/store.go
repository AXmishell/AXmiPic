// Package store 为 AXmiPic 元数据提供持久化。
package store

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ErrNotFound 在请求的记录不存在时返回。
var ErrNotFound = errors.New("store: record not found")

// migrationModels 是需要由 AutoMigrate 维护的模型集合。
var migrationModels = []any{
	&Image{}, &PendingUpload{}, &Admin{}, &Customer{}, &APIToken{},
	&StorageBackend{}, &Album{}, &RoleGroup{}, &Policy{}, &RoleGroupPolicy{},
	&Share{}, &Announcement{}, &Report{}, &Page{}, &Setting{},
	&Plan{}, &Order{}, &Coupon{}, &CouponRedemption{}, &Ticket{}, &TicketMessage{},
	&EmailCode{}, &EmailCodeStat{}, &NotifyLog{}, &GuestIPUsage{},
	&UserPreference{},
}

// mysqlTableOptions 强制新表使用 utf8mb4。MySQL 5.7 的服务端默认字符集是
// latin1，AutoMigrate 建表时会继承数据库默认字符集，导致中文等多字节字符
// 写入失败（ERROR 1366）。前置空格是必需的：GORM 直接把它拼在
// "CREATE TABLE ... )" 之后。
const mysqlTableOptions = " DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"

// isMySQLDriver 判断 driver 是否使用 MySQL 方言（MariaDB 共用同一驱动）。
func isMySQLDriver(driver string) bool {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "mysql", "mariadb":
		return true
	default:
		return false
	}
}

// Repository 提供元数据的持久化操作。
type Repository struct {
	db *gorm.DB
}

// Open 打开由 driver 选择的数据库，运行迁移，并返回一个即用型的
// repository。支持的驱动有 "sqlite"（默认）、"postgres" 和 "mysql"
// （含 MariaDB）。dsn 对于 sqlite 是文件路径，对于 postgres 是 libpq 连接
// 字符串（或 URL），对于 mysql 是 go-sql-driver 的 DSN。
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
	// MySQL：显式指定建表字符集，避免 5.7 默认 latin1 带来的写入失败。
	if isMySQLDriver(driver) {
		db = db.Set("gorm:table_options", mysqlTableOptions)
	}
	if err := db.AutoMigrate(migrationModels...); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	if isMySQLDriver(driver) {
		warnNonUTF8MB4Tables(db)
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
	case "mysql", "mariadb":
		return mysql.Open(normalizeMySQLDSN(dsn)), nil
	default:
		return nil, fmt.Errorf("store: unsupported database driver %q", driver)
	}
}

// normalizeMySQLDSN 补齐 MySQL 连接串的关键参数：parseTime=true（扫描
// time.Time 所必需）与 utf8mb4 字符集。用户已显式指定的参数保持不变。
func normalizeMySQLDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return dsn
	}
	base, query, _ := strings.Cut(dsn, "?")
	params := map[string]string{}
	if query != "" {
		for _, pair := range strings.Split(query, "&") {
			key, value, _ := strings.Cut(pair, "=")
			if key != "" {
				params[key] = value
			}
		}
	}
	if _, ok := params["parseTime"]; !ok {
		params["parseTime"] = "true"
	}
	if _, ok := params["charset"]; !ok {
		params["charset"] = "utf8mb4"
	}
	if _, ok := params["loc"]; !ok {
		params["loc"] = "Local"
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	return base + "?" + strings.Join(parts, "&")
}

// warnNonUTF8MB4Tables 检查既有表是否使用 utf8mb4，并在发现非 utf8mb4 表时
// 记录告警。MySQL 5.7 服务端默认字符集是 latin1：升级前由旧版本建出的表会
// 继承 latin1，写入中文等多字节字符时返回 ERROR 1366。这里不自动执行
// CONVERT（会重写整表），由运维选择合适的时机处理。
func warnNonUTF8MB4Tables(db *gorm.DB) {
	tables := make([]string, 0, len(migrationModels))
	for _, model := range migrationModels {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(model); err != nil || stmt.Schema == nil {
			continue
		}
		tables = append(tables, stmt.Schema.Table)
	}
	if len(tables) == 0 {
		return
	}
	var rows []struct {
		TableName      string
		TableCollation string
	}
	if err := db.Raw(
		"SELECT TABLE_NAME AS table_name, TABLE_COLLATION AS table_collation "+
			"FROM information_schema.TABLES "+
			"WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ? AND TABLE_COLLATION NOT LIKE 'utf8mb4%' "+
			"ORDER BY TABLE_NAME",
		tables,
	).Scan(&rows).Error; err != nil || len(rows) == 0 {
		return
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.TableName+" ("+row.TableCollation+")")
	}
	slog.Warn("mysql: tables are not utf8mb4; non-ASCII text may fail with ERROR 1366",
		slog.Any("tables", names),
		slog.String("fix", "ALTER TABLE <table> CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"),
	)
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

// SetPool 配置底层 database/sql 连接池，避免高并发下无界地打开连接与缓冲。
// maxOpen<=0 表示不限制；maxIdle<=0 表示不保留空闲连接（设为 0）；lifetime<=0
// 表示不限制连接存活时间。
func (r *Repository) SetPool(maxOpen, maxIdle int, lifetime time.Duration) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return fmt.Errorf("store: connection: %w", err)
	}
	if maxOpen > 0 {
		sqlDB.SetMaxOpenConns(maxOpen)
	}
	if maxIdle >= 0 {
		sqlDB.SetMaxIdleConns(maxIdle)
	}
	if lifetime > 0 {
		sqlDB.SetConnMaxLifetime(lifetime)
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
