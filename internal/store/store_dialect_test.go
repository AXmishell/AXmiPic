package store

import (
	"strings"
	"testing"
)

func TestNormalizeMySQLDSNAddsDefaults(t *testing.T) {
	got := normalizeMySQLDSN("user:pass@tcp(127.0.0.1:3306)/axmipic")
	if !strings.HasPrefix(got, "user:pass@tcp(127.0.0.1:3306)/axmipic?") {
		t.Fatalf("unexpected base: %q", got)
	}
	for _, want := range []string{"parseTime=true", "charset=utf8mb4", "loc=Local"} {
		if !strings.Contains(got, want) {
			t.Fatalf("normalizeMySQLDSN() = %q, missing %q", got, want)
		}
	}
}

func TestNormalizeMySQLDSNPreservesOverrides(t *testing.T) {
	got := normalizeMySQLDSN("u:p@tcp(h:3306)/db?parseTime=false&charset=latin1&loc=UTC")
	if strings.Contains(got, "parseTime=true") {
		t.Fatalf("should preserve parseTime=false: %q", got)
	}
	if !strings.Contains(got, "charset=latin1") || !strings.Contains(got, "loc=UTC") {
		t.Fatalf("should preserve overrides: %q", got)
	}
}

func TestNormalizeMySQLDSNEmpty(t *testing.T) {
	if got := normalizeMySQLDSN(""); got != "" {
		t.Fatalf("empty dsn should stay empty, got %q", got)
	}
}

func TestIsMySQLDriver(t *testing.T) {
	for _, driver := range []string{"mysql", "MySQL", " mariadb "} {
		if !isMySQLDriver(driver) {
			t.Fatalf("isMySQLDriver(%q) = false, want true", driver)
		}
	}
	for _, driver := range []string{"", "sqlite", "postgres", "postgresql", "pgx"} {
		if isMySQLDriver(driver) {
			t.Fatalf("isMySQLDriver(%q) = true, want false", driver)
		}
	}
}

// TestMySQLTableOptions 确保建表选项包含 utf8mb4：MySQL 5.7 的服务端默认
// 字符集为 latin1，缺省建表会让中文写入失败（ERROR 1366）。
func TestMySQLTableOptions(t *testing.T) {
	if !strings.HasPrefix(mysqlTableOptions, " ") {
		t.Fatalf("mysqlTableOptions must start with a space (GORM appends it directly after \")\"): %q", mysqlTableOptions)
	}
	for _, want := range []string{"DEFAULT CHARSET=utf8mb4", "COLLATE=utf8mb4_unicode_ci"} {
		if !strings.Contains(mysqlTableOptions, want) {
			t.Fatalf("mysqlTableOptions = %q, missing %q", mysqlTableOptions, want)
		}
	}
}
