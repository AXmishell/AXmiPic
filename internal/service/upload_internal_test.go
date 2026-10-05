package service

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AXmishell/axmipic/internal/auth"
)

func TestSanitizeOriginalNameTruncatesOnRuneBoundary(t *testing.T) {
	name := strings.Repeat("图", 100) // 300 字节
	got := sanitizeOriginalName(name)
	if len(got) != maxOriginalNameBytes {
		t.Fatalf("len = %d, want %d", len(got), maxOriginalNameBytes)
	}
	if !utf8.ValidString(got) {
		t.Fatal("truncation produced invalid UTF-8")
	}
}

func TestSanitizeOriginalNameStripsPathsAndControls(t *testing.T) {
	if got := sanitizeOriginalName(`C:\tmp\a/b.png`); got != "b.png" {
		t.Fatalf("sanitized = %q, want %q", got, "b.png")
	}
	if got := sanitizeOriginalName("evil\x00name.png"); strings.ContainsRune(got, 0) {
		t.Fatalf("control character survived: %q", got)
	}
}

func TestOwnerIDOfAttributesGuests(t *testing.T) {
	guest := ownerIDOf(&auth.Principal{UserID: "g1", Guest: true})
	if guest == nil || *guest != "g1" {
		t.Fatalf("guest owner = %v, want g1", guest)
	}
	if ownerIDOf(nil) != nil {
		t.Fatal("nil principal should have no owner")
	}
	if ownerIDOf(&auth.Principal{}) != nil {
		t.Fatal("principal without user id should have no owner")
	}
}
