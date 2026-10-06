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
