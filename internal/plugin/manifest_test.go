package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePluginDir 在临时目录下写入一个插件目录，返回其路径。
func writePluginDir(t *testing.T, manifest string, entryName string, entry []byte) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if entryName != "" {
		if err := os.WriteFile(filepath.Join(dir, entryName), entry, 0o600); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
	return dir
}

func TestLoadManifestValid(t *testing.T) {
	dir := writePluginDir(t, `
name: smsbao
version: 0.1.0
category: notify.sms
runtime: wasm
entry: plugin.wasm
abi: 1
capabilities:
  http:
    hosts: ["api.smsbao.com", "*.example.com"]
`, "plugin.wasm", []byte("wasm-bytes"))

	man, entry, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if man.Name != "smsbao" || man.Category != CategoryNotifySMS || man.Runtime != RuntimeWASM {
		t.Fatalf("unexpected manifest: %+v", man)
	}
	if entry != filepath.Join(dir, "plugin.wasm") {
		t.Fatalf("unexpected entry: %s", entry)
	}
	if got := man.httpHosts(); len(got) != 2 {
		t.Fatalf("unexpected hosts: %v", got)
	}
}

func TestLoadManifestErrors(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		entry    string
		wantSub  string
	}{
		{"missing name", "version: 1\ncategory: c\nruntime: wasm\nentry: plugin.wasm\nabi: 1\n", "plugin.wasm", "name is required"},
		{"missing category", "name: p\nversion: 1\nruntime: wasm\nentry: plugin.wasm\nabi: 1\n", "plugin.wasm", "category is required"},
		{"unknown runtime", "name: p\ncategory: c\nruntime: native\nentry: plugin.wasm\nabi: 1\n", "plugin.wasm", "unknown runtime"},
		{"bad abi", "name: p\ncategory: c\nruntime: wasm\nentry: plugin.wasm\nabi: 2\n", "plugin.wasm", "unsupported abi"},
		{"missing entry", "name: p\ncategory: c\nruntime: wasm\nentry: missing.wasm\nabi: 1\n", "", "entry not found"},
		{"escape", "name: p\ncategory: c\nruntime: wasm\nentry: ../escape.wasm\nabi: 1\n", "plugin.wasm", "escapes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writePluginDir(t, tc.manifest, tc.entry, []byte("x"))
			if _, _, err := loadManifest(dir); err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("want error containing %q, got %v", tc.wantSub, err)
			}
		})
	}
}

func TestLoadManifestChecksum(t *testing.T) {
	entry := []byte("the-wasm-binary")
	sum := sha256.Sum256(entry)
	checksum := "sha256:" + hex.EncodeToString(sum[:])

	dir := writePluginDir(t, "name: p\ncategory: c\nruntime: wasm\nentry: p.wasm\nabi: 1\nchecksum: "+checksum+"\n", "p.wasm", entry)
	if _, _, err := loadManifest(dir); err != nil {
		t.Fatalf("checksum should match: %v", err)
	}

	bad := writePluginDir(t, "name: p\ncategory: c\nruntime: wasm\nentry: p.wasm\nabi: 1\nchecksum: sha256:deadbeef\n", "p.wasm", entry)
	if _, _, err := loadManifest(bad); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
}

func TestHostAllowed(t *testing.T) {
	patterns := []string{"api.smsbao.com", "*.example.com"}
	cases := map[string]bool{
		"api.smsbao.com":   true,
		"API.SMSBAO.COM":   true,
		"evil.com":         false,
		"a.example.com":    true,
		"a.b.example.com":  true,
		"example.com":      false,
		"notexample.com":   false,
		"xapi.smsbao.com2": false,
	}
	for host, want := range cases {
		if got := hostAllowed(host, patterns); got != want {
			t.Errorf("hostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
	if !hostAllowed("anything", []string{"*"}) {
		t.Error("wildcard should allow anything")
	}
}

func TestMethodAllowed(t *testing.T) {
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"} {
		if !methodAllowed(m) {
			t.Errorf("method %s should be allowed", m)
		}
	}
	if methodAllowed("TRACE") {
		t.Error("TRACE should not be allowed")
	}
}
