package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 安装相关的错误。
var (
	// ErrSignatureRequired 表示需要签名但未提供（或未配置可信公钥）。
	ErrSignatureRequired = errors.New("plugin: signature required")
	// ErrSignatureInvalid 表示签名校验失败。
	ErrSignatureInvalid = errors.New("plugin: signature verification failed")
	// ErrChecksumMismatch 表示归档 sha256 不匹配。
	ErrChecksumMismatch = errors.New("plugin: checksum mismatch")
	// ErrArchiveInvalid 表示归档格式非法。
	ErrArchiveInvalid = errors.New("plugin: invalid archive")
)

// DefaultMaxArchiveBytes 是插件归档解压后的默认最大字节数。
const DefaultMaxArchiveBytes int64 = 64 << 20

// InstallSpec 描述一次安装请求。URL 为空且 Name 非空时，会从插件索引解析。
type InstallSpec struct {
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
	Name      string `json:"name"`
	Version   string `json:"version"`
}

// MarketEntry 是插件索引中的一条市场条目。
type MarketEntry struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Category    string `json:"category"`
	Runtime     string `json:"runtime"`
	Description string `json:"description"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	Signature   string `json:"signature"`
}

// marketIndex 是插件索引文件的结构。
type marketIndex struct {
	Plugins []MarketEntry `json:"plugins"`
}

// InstallFromArchive 从内存中的归档安装插件，并立即加载。
func (m *Manager) InstallFromArchive(ctx context.Context, data []byte, sha256Hex, signature string) (Manifest, error) {
	if err := m.verifyArtifact(data, sha256Hex, signature); err != nil {
		return Manifest{}, err
	}
	return m.installArchive(ctx, data)
}

// InstallFromURL 下载并安装插件。若 URL 为空但 Name 非空，则从索引解析。
func (m *Manager) InstallFromURL(ctx context.Context, spec InstallSpec) (Manifest, error) {
	if strings.TrimSpace(spec.URL) == "" {
		entry, err := m.resolveFromIndex(ctx, spec.Name, spec.Version)
		if err != nil {
			return Manifest{}, err
		}
		spec.URL = entry.URL
		spec.SHA256 = entry.SHA256
		spec.Signature = entry.Signature
	}
	data, err := m.download(ctx, spec.URL)
	if err != nil {
		return Manifest{}, err
	}
	return m.InstallFromArchive(ctx, data, spec.SHA256, spec.Signature)
}

// FetchRegistry 拉取插件索引。未配置索引地址时返回空列表（视为无市场）。
func (m *Manager) FetchRegistry(ctx context.Context) ([]MarketEntry, error) {
	if strings.TrimSpace(m.opts.IndexURL) == "" {
		return []MarketEntry{}, nil
	}
	data, err := m.downloadLimit(ctx, m.opts.IndexURL, 4<<20)
	if err != nil {
		return nil, err
	}
	var index marketIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("plugin: parse index: %w", err)
	}
	if index.Plugins == nil {
		index.Plugins = []MarketEntry{}
	}
	return index.Plugins, nil
}

// RegistryConfigured 报告是否配置了插件市场索引地址。
func (m *Manager) RegistryConfigured() bool {
	return strings.TrimSpace(m.opts.IndexURL) != ""
}

// Remove 卸载并删除指定插件目录。
func (m *Manager) Remove(ctx context.Context, name string) error {
	m.mu.RLock()
	lp, ok := m.installed[name]
	m.mu.RUnlock()
	dir := ""
	if ok {
		dir = lp.dir
	}
	if err := m.Disable(ctx, name); err != nil {
		return err
	}
	m.removeEntry(name)
	if dir == "" {
		if strings.TrimSpace(m.opts.Dir) == "" {
			return nil
		}
		dir = filepath.Join(m.opts.Dir, name)
	}
	// 安全约束：只允许删除插件目录内的路径。
	root, err := filepath.Abs(m.opts.Dir)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return fmt.Errorf("plugin %q: refusing to remove path outside plugins dir", name)
	}
	if err := os.RemoveAll(abs); err != nil {
		return fmt.Errorf("plugin %q: remove: %w", name, err)
	}
	return nil
}

// resolveFromIndex 从索引解析条目。
func (m *Manager) resolveFromIndex(ctx context.Context, name, version string) (MarketEntry, error) {
	entries, err := m.FetchRegistry(ctx)
	if err != nil {
		return MarketEntry{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return MarketEntry{}, fmt.Errorf("%w: plugin name is required", ErrNotFound)
	}
	for _, e := range entries {
		if e.Name != name {
			continue
		}
		if version == "" || e.Version == version {
			return e, nil
		}
	}
	return MarketEntry{}, fmt.Errorf("%w: %s@%s not found in index", ErrNotFound, name, version)
}

// verifyArtifact 校验归档的 sha256 与签名。
func (m *Manager) verifyArtifact(data []byte, sha256Hex, signature string) error {
	if strings.TrimSpace(sha256Hex) != "" {
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimPrefix(strings.TrimSpace(sha256Hex), "sha256:")) {
			return ErrChecksumMismatch
		}
	}
	return m.verifySignature(data, signature)
}

// verifySignature 依据可信公钥校验 Ed25519 签名。
func (m *Manager) verifySignature(data []byte, signature string) error {
	keys := m.opts.TrustedKeys
	if len(keys) == 0 {
		if m.opts.RequireSignature {
			return fmt.Errorf("%w: no trusted keys configured", ErrSignatureRequired)
		}
		if m.opts.Logger != nil {
			m.opts.Logger.Warn("installing plugin without signature verification; configure plugins.trusted_keys to enforce")
		}
		return nil
	}
	if strings.TrimSpace(signature) == "" {
		return ErrSignatureRequired
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrSignatureInvalid
	}
	for _, key := range keys {
		pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
		if err != nil || len(pub) != ed25519.PublicKeySize {
			continue
		}
		if ed25519.Verify(ed25519.PublicKey(pub), data, sig) {
			return nil
		}
	}
	return ErrSignatureInvalid
}

// download 下载归档，大小受 MaxArchiveBytes 限制。
func (m *Manager) download(ctx context.Context, url string) ([]byte, error) {
	return m.downloadLimit(ctx, url, m.maxArchiveBytes())
}

// downloadLimit 下载 URL 内容，最多 limit 字节。
func (m *Manager) downloadLimit(ctx context.Context, url string, limit int64) ([]byte, error) {
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("plugin: url is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("plugin: build download request: %w", err)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plugin: download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("plugin: download %s: status %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("plugin: read %s: %w", url, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("plugin: download %s exceeds %d bytes", url, limit)
	}
	return data, nil
}

// maxArchiveBytes 返回解压后的最大字节数。
func (m *Manager) maxArchiveBytes() int64 {
	if m.opts.MaxArchiveBytes > 0 {
		return m.opts.MaxArchiveBytes
	}
	return DefaultMaxArchiveBytes
}

// installArchive 解压、校验清单、落盘并加载插件。
func (m *Manager) installArchive(ctx context.Context, data []byte) (Manifest, error) {
	if strings.TrimSpace(m.opts.Dir) == "" {
		return Manifest{}, fmt.Errorf("plugin: plugins dir is not configured")
	}
	m.installMu.Lock()
	defer m.installMu.Unlock()

	if err := os.MkdirAll(m.opts.Dir, 0o755); err != nil {
		return Manifest{}, fmt.Errorf("plugin: create plugins dir: %w", err)
	}
	tmp, err := os.MkdirTemp(m.opts.Dir, ".install-*")
	if err != nil {
		return Manifest{}, fmt.Errorf("plugin: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := extractZip(data, tmp, m.maxArchiveBytes()); err != nil {
		return Manifest{}, err
	}

	man, entry, err := loadManifest(tmp)
	if err != nil {
		return Manifest{}, err
	}
	// 进程插件需要可执行位（zip 通常不保留 Unix 权限）。
	if man.Runtime == RuntimeProcess {
		if err := os.Chmod(entry, 0o755); err != nil {
			return Manifest{}, fmt.Errorf("plugin %q: chmod entry: %w", man.Name, err)
		}
	}

	final := filepath.Join(m.opts.Dir, man.Name)
	// 若同名插件在运行，先卸载再替换。
	if err := m.Unload(ctx, man.Name); err != nil {
		if m.opts.Logger != nil {
			m.opts.Logger.Warn("failed to unload previous plugin",
				slog.String("plugin", man.Name), slog.Any("error", err))
		}
	}
	if err := os.RemoveAll(final); err != nil {
		return Manifest{}, fmt.Errorf("plugin %q: remove existing: %w", man.Name, err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return Manifest{}, fmt.Errorf("plugin %q: install: %w", man.Name, err)
	}
	// MkdirTemp 默认 0700，安装目录改为可读。
	if err := os.Chmod(final, 0o755); err != nil {
		return Manifest{}, fmt.Errorf("plugin %q: chmod dir: %w", man.Name, err)
	}
	if err := m.loadOne(ctx, final); err != nil {
		return Manifest{}, err
	}
	return man, nil
}

// extractZip 安全地把 zip 解压到 dest。拒绝路径穿越、符号链接与超限内容。
func extractZip(data []byte, dest string, maxBytes int64) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrArchiveInvalid, err)
	}
	root := filepath.Clean(dest)
	var total int64
	for _, f := range zr.File {
		name := filepath.Clean(filepath.FromSlash(f.Name))
		if name == "." || name == "" {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("%w: entry %q escapes archive root", ErrArchiveInvalid, f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink %q is not allowed", ErrArchiveInvalid, f.Name)
		}
		total += int64(f.UncompressedSize64)
		if total > maxBytes {
			return fmt.Errorf("%w: extracted size exceeds %d bytes", ErrArchiveInvalid, maxBytes)
		}
		target := filepath.Join(root, name)
		if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return fmt.Errorf("%w: entry %q escapes archive root", ErrArchiveInvalid, f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("plugin: create dir: %w", err)
		}
		if err := writeZipFile(f, target, maxBytes); err != nil {
			return err
		}
	}
	return nil
}

// writeZipFile 写出单个 zip 条目。
func writeZipFile(f *zip.File, target string, maxBytes int64) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("%w: open %q: %v", ErrArchiveInvalid, f.Name, err)
	}
	defer func() { _ = rc.Close() }()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("plugin: create %q: %w", target, err)
	}
	if _, err := io.Copy(out, io.LimitReader(rc, maxBytes+1)); err != nil {
		_ = out.Close()
		return fmt.Errorf("plugin: write %q: %w", target, err)
	}
	return out.Close()
}
