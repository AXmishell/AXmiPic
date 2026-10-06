package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const processManifestArchive = `
name: process-webhook
version: 0.1.0
category: notify.sms
runtime: process
entry: plugin-bin
abi: 1
`

// makeZip 构造一个内存 zip。
func makeZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
		hdr.SetMode(0o644)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// processArchive 用已编译的示例进程插件与清单构造归档。
func processArchive(t *testing.T) []byte {
	t.Helper()
	requireBinary(t, "process-webhook")
	bin, err := os.ReadFile(exampleBin["process-webhook"])
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	return makeZip(t, map[string][]byte{
		ManifestFileName: []byte(processManifestArchive),
		"plugin-bin":     bin,
	})
}

// signArtifact 生成 sha256 与 Ed25519 签名，返回归档、公钥、签名、sha256。
func signArtifact(t *testing.T, data []byte) (pubB64, sigB64, shaHex string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sig := ed25519.Sign(priv, data)
	sum := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(sig), hex.EncodeToString(sum[:])
}

// newInstallManager 构造带安装选项的管理器。
func newInstallManager(t *testing.T, dir string, mutate func(*Options)) *Manager {
	t.Helper()
	opts := Options{Dir: dir, Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))}
	if mutate != nil {
		mutate(&opts)
	}
	mgr := NewManager(opts)
	t.Cleanup(func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = mgr.Close(ctx)
	})
	return mgr
}

func TestInstallFromArchiveWithSignature(t *testing.T) {
	archive := processArchive(t)
	pubB64, sigB64, shaHex := signArtifact(t, archive)

	var webhookHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookHit = true
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	mgr := newInstallManager(t, dir, func(o *Options) {
		o.TrustedKeys = []string{pubB64}
		o.RequireSignature = true
	})
	ctx := context.Background()

	man, err := mgr.InstallFromArchive(ctx, archive, shaHex, sigB64)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if man.Name != "process-webhook" || man.Runtime != RuntimeProcess {
		t.Fatalf("unexpected manifest: %+v", man)
	}
	if _, err := os.Stat(filepath.Join(dir, "process-webhook", "plugin-bin")); err != nil {
		t.Fatalf("installed entry missing: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "process-webhook"); !ok {
		t.Fatal("plugin not registered after install")
	}

	// 已安装的进程插件应可正常调用。
	if err := mgr.Configure(ctx, "process-webhook", map[string]string{"url": srv.URL}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	input, _ := json.Marshal(map[string]string{"to": "13800000000", "body": "hi"})
	if _, err := mgr.Invoke(ctx, CategoryNotifySMS, "process-webhook", "send", input); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !webhookHit {
		t.Fatal("installed plugin did not call the webhook")
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	archive := processArchive(t)
	_, _, shaHex := signArtifact(t, archive)
	dir := t.TempDir()
	mgr := newInstallManager(t, dir, nil)

	bad := strings.Repeat("0", len(shaHex))
	if _, err := mgr.InstallFromArchive(context.Background(), archive, bad, ""); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
}

func TestInstallSignatureInvalid(t *testing.T) {
	archive := processArchive(t)
	pubB64, _, shaHex := signArtifact(t, archive)
	// 用另一个密钥签名。
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	badSig := base64.StdEncoding.EncodeToString(ed25519.Sign(otherPriv, archive))

	dir := t.TempDir()
	mgr := newInstallManager(t, dir, func(o *Options) { o.TrustedKeys = []string{pubB64} })
	if _, err := mgr.InstallFromArchive(context.Background(), archive, shaHex, badSig); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("want signature invalid, got %v", err)
	}
}

func TestInstallRequireSignatureWithoutKeys(t *testing.T) {
	archive := processArchive(t)
	dir := t.TempDir()
	mgr := newInstallManager(t, dir, func(o *Options) { o.RequireSignature = true })
	if _, err := mgr.InstallFromArchive(context.Background(), archive, "", ""); !errors.Is(err, ErrSignatureRequired) {
		t.Fatalf("want signature required, got %v", err)
	}
}

func TestInstallRejectsZipSlip(t *testing.T) {
	requireBinary(t, "process-webhook")
	bin, _ := os.ReadFile(exampleBin["process-webhook"])
	evil := makeZip(t, map[string][]byte{
		ManifestFileName:    []byte(processManifestArchive),
		"plugin-bin":        bin,
		"../escaped-plugin": []byte("pwned"),
	})
	dir := t.TempDir()
	mgr := newInstallManager(t, dir, nil)
	if _, err := mgr.InstallFromArchive(context.Background(), evil, "", ""); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("want archive invalid, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escaped-plugin")); err == nil {
		t.Fatal("zip-slip escaped the install directory")
	}
}

func TestInstallFromRegistry(t *testing.T) {
	archive := processArchive(t)
	pubB64, sigB64, shaHex := signArtifact(t, archive)

	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/sms.zip", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		index := map[string]any{"plugins": []map[string]any{{
			"name":      "process-webhook",
			"version":   "0.1.0",
			"category":  CategoryNotifySMS,
			"runtime":   RuntimeProcess,
			"url":       base + "/sms.zip",
			"sha256":    shaHex,
			"signature": sigB64,
		}}}
		_ = json.NewEncoder(w).Encode(index)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL

	dir := t.TempDir()
	mgr := newInstallManager(t, dir, func(o *Options) {
		o.IndexURL = srv.URL + "/index.json"
		o.TrustedKeys = []string{pubB64}
	})
	ctx := context.Background()

	entries, err := mgr.FetchRegistry(ctx)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "process-webhook" {
		t.Fatalf("unexpected registry: %+v", entries)
	}

	man, err := mgr.InstallFromURL(ctx, InstallSpec{Name: "process-webhook"})
	if err != nil {
		t.Fatalf("install from registry: %v", err)
	}
	if man.Name != "process-webhook" {
		t.Fatalf("unexpected manifest: %+v", man)
	}
}

func TestFetchRegistryNotConfigured(t *testing.T) {
	mgr := newInstallManager(t, t.TempDir(), nil)
	entries, err := mgr.FetchRegistry(context.Background())
	if err != nil {
		t.Fatalf("FetchRegistry: %v", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Fatalf("expected empty registry, got %+v", entries)
	}
	if mgr.RegistryConfigured() {
		t.Fatal("registry should not be reported as configured")
	}
}

func TestRemovePlugin(t *testing.T) {
	archive := processArchive(t)
	pubB64, sigB64, shaHex := signArtifact(t, archive)
	dir := t.TempDir()
	mgr := newInstallManager(t, dir, func(o *Options) { o.TrustedKeys = []string{pubB64} })
	ctx := context.Background()

	if _, err := mgr.InstallFromArchive(ctx, archive, shaHex, sigB64); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := mgr.Remove(ctx, "process-webhook"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, ok := mgr.Registry().Lookup(CategoryNotifySMS, "process-webhook"); ok {
		t.Fatal("plugin still registered after remove")
	}
	if _, err := os.Stat(filepath.Join(dir, "process-webhook")); !os.IsNotExist(err) {
		t.Fatalf("plugin dir should be removed, stat err = %v", err)
	}
}
