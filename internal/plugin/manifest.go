package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Capabilities 描述插件向宿主申请的能力。未申请即不可用。
type Capabilities struct {
	// HTTP 为网络访问能力；为空表示禁止任何网络访问。
	HTTP *HTTPCapability `yaml:"http,omitempty" json:"http,omitempty"`
}

// HTTPCapability 声明插件允许访问的目标主机。
type HTTPCapability struct {
	// Hosts 为主机白名单，支持精确匹配（api.example.com）与通配子域（*.example.com）。
	// 使用 "*" 表示允许任意主机（不推荐）。
	Hosts []string `yaml:"hosts" json:"hosts"`
}

// Manifest 是插件的元数据，来自插件目录下的 plugin.yaml。
type Manifest struct {
	Name         string       `yaml:"name" json:"name"`
	Version      string       `yaml:"version" json:"version"`
	Category     string       `yaml:"category" json:"category"`
	Runtime      RuntimeKind  `yaml:"runtime" json:"runtime"`
	Entry        string       `yaml:"entry" json:"entry"`
	ABI          int          `yaml:"abi" json:"abi"`
	Capabilities Capabilities `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	// Checksum 为入口文件的 sha256 校验值，形如 "sha256:<hex>"，可选。
	Checksum string `yaml:"checksum,omitempty" json:"checksum,omitempty"`
	// Builtin 标识内置插件。该字段不从 plugin.yaml 读取，仅由宿主在注册
	// 内置 Provider 时设置，外置清单无法伪造。
	Builtin bool `yaml:"-" json:"-"`
}

// ManifestFileName 是插件清单的约定文件名。
const ManifestFileName = "plugin.yaml"

// loadManifest 从插件目录读取并校验清单。
func loadManifest(dir string) (Manifest, string, error) {
	path := filepath.Join(dir, ManifestFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("plugin: read manifest: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return Manifest{}, "", fmt.Errorf("plugin: parse manifest %s: %w", path, err)
	}
	entry, err := m.resolveEntry(dir)
	if err != nil {
		return Manifest{}, "", err
	}
	if err := verifyChecksum(entry, m.Checksum); err != nil {
		return Manifest{}, "", err
	}
	return m, entry, nil
}

// resolveEntry 校验清单并返回入口文件的绝对路径。
func (m Manifest) resolveEntry(dir string) (string, error) {
	if err := m.validate(); err != nil {
		return "", err
	}
	entry := filepath.Join(dir, filepath.FromSlash(m.Entry))
	// 防目录穿越：入口必须位于插件目录内。
	rel, err := filepath.Rel(dir, entry)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("plugin %q: entry escapes plugin directory", m.Name)
	}
	info, err := os.Stat(entry)
	if err != nil {
		return "", fmt.Errorf("plugin %q: entry not found: %w", m.Name, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("plugin %q: entry is a directory", m.Name)
	}
	return entry, nil
}

// validate 检查清单的基本字段。外置插件只允许 WASM 沙箱运行时；内置插件
// 由宿主直接注册，不经过此路径。
func (m Manifest) validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("plugin: manifest name is required")
	}
	if !validPluginName(m.Name) {
		return fmt.Errorf("plugin %q: name must match [a-z0-9][a-z0-9._-]* and must not be . or ..", m.Name)
	}
	if strings.HasPrefix(m.Name, "builtin-") {
		return fmt.Errorf("plugin %q: the builtin- prefix is reserved for built-in plugins", m.Name)
	}
	if strings.TrimSpace(m.Category) == "" {
		return fmt.Errorf("plugin %q: category is required", m.Name)
	}
	if m.Runtime != RuntimeWASM {
		return fmt.Errorf("plugin %q: external plugins must use the wasm sandbox runtime (got %q)", m.Name, m.Runtime)
	}
	if strings.TrimSpace(m.Entry) == "" {
		return fmt.Errorf("plugin %q: entry is required", m.Name)
	}
	if m.ABI != ABI {
		return fmt.Errorf("plugin %q: unsupported abi %d (host expects %d)", m.Name, m.ABI, ABI)
	}
	for _, host := range m.httpHosts() {
		if err := validateHostPattern(host); err != nil {
			return fmt.Errorf("plugin %q: %w", m.Name, err)
		}
	}
	return nil
}

// validPluginName 只允许安全的目录名，避免 . / .. 或路径分隔符进入安装路径。
func validPluginName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// httpHosts 返回声明的主机白名单（可能为空）。
func (m Manifest) httpHosts() []string {
	if m.Capabilities.HTTP == nil {
		return nil
	}
	return m.Capabilities.HTTP.Hosts
}

// validateHostPattern 校验主机白名单条目。
func validateHostPattern(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("empty host pattern in http capability")
	}
	if host == "*" {
		return nil
	}
	host = strings.TrimPrefix(host, "*.")
	if strings.ContainsAny(host, "/\\ \t:*") {
		return fmt.Errorf("invalid host pattern %q", host)
	}
	return nil
}

// verifyChecksum 在清单声明 checksum 时校验入口文件摘要。
func verifyChecksum(path, want string) error {
	want = strings.TrimSpace(want)
	if want == "" {
		return nil
	}
	const prefix = "sha256:"
	if !strings.HasPrefix(want, prefix) {
		return fmt.Errorf("plugin: unsupported checksum format %q", want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("plugin: read entry for checksum: %w", err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, strings.TrimPrefix(want, prefix)) {
		return fmt.Errorf("plugin: checksum mismatch (want %s, got sha256:%s)", want, got)
	}
	return nil
}
