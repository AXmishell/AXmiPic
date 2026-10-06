package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Options 配置插件管理器。
type Options struct {
	// Dir 为插件根目录；每个子目录为一个插件。
	Dir string
	// Logger 为日志器，默认使用 slog.Default。
	Logger *slog.Logger
	// HTTPTimeout 为插件发起 HTTP 请求的超时。
	HTTPTimeout time.Duration
	// MaxHTTPBody 为单次 HTTP 请求/响应体的最大字节数。
	MaxHTTPBody int64
	// MemoryLimitPages 为 WASM 插件线性内存上限（页）。
	MemoryLimitPages uint32
	// TrustedKeys 为 Ed25519 可信公钥（base64），用于校验在线安装的插件签名。
	TrustedKeys []string
	// RequireSignature 为 true 时，安装插件必须提供有效签名。
	RequireSignature bool
	// IndexURL 为插件市场索引（JSON）地址，供在线浏览与按名安装。
	IndexURL string
	// MaxArchiveBytes 为插件归档解压后的最大字节数；0 表示默认值。
	MaxArchiveBytes int64
}

// installedPlugin 是一个已安装的插件。enabled 为 false 表示已暂停（未注册，磁盘插件
// 的实例已关闭但保留文件与配置）。
type installedPlugin struct {
	manifest   Manifest
	dir        string
	provider   Provider
	descriptor Descriptor
	enabled    bool
}

// InstalledInfo 是插件的安装/启用状态快照。
type InstalledInfo struct {
	Manifest   Manifest
	Descriptor Descriptor
	Enabled    bool
	Dir        string
}

// Manager 负责发现、加载、配置并管理插件，是插件系统的门面。
type Manager struct {
	opts     Options
	registry *Registry

	mu        sync.RWMutex
	installed map[string]*installedPlugin
	closed    bool
	// installMu 串行化安装/卸载，避免并发写插件目录。
	installMu sync.Mutex
}

// NewManager 创建一个插件管理器。
func NewManager(opts Options) *Manager {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Manager{
		opts:      opts,
		registry:  NewRegistry(),
		installed: make(map[string]*installedPlugin),
	}
}

// Registry 返回底层注册表，供只读查询已启用插件的描述。
func (m *Manager) Registry() *Registry { return m.registry }

// Load 加载 Options.Dir 下的全部插件。
func (m *Manager) Load(ctx context.Context) error {
	if m.opts.Dir == "" {
		return nil
	}
	return m.LoadDir(ctx, m.opts.Dir)
}

// LoadDir 扫描目录并加载其中的插件。单个插件加载失败不会阻止其它插件，
// 所有错误会被聚合返回。
func (m *Manager) LoadDir(ctx context.Context, dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m.opts.Logger.Debug("plugin directory does not exist", slog.String("dir", dir))
			return nil
		}
		return fmt.Errorf("plugin: stat dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("plugin: %s is not a directory", dir)
	}

	// 目录本身即插件（包含 plugin.yaml）时只加载它。
	if _, err := os.Stat(filepath.Join(dir, ManifestFileName)); err == nil {
		return m.loadOne(ctx, dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("plugin: read dir %s: %w", dir, err)
	}
	var errs []error
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		sub := filepath.Join(dir, ent.Name())
		if _, err := os.Stat(filepath.Join(sub, ManifestFileName)); err != nil {
			continue // 非插件目录，跳过。
		}
		if err := m.loadOne(ctx, sub); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// loadOne 加载单个插件目录并注册。
func (m *Manager) loadOne(ctx context.Context, dir string) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return fmt.Errorf("plugin: manager is closed")
	}
	m.mu.Unlock()

	man, entry, err := loadManifest(dir)
	if err != nil {
		return err
	}

	// 先关闭同名的旧实例（若存在）。
	if err := m.Disable(ctx, man.Name); err != nil {
		m.opts.Logger.Warn("failed to disable previous plugin",
			slog.String("plugin", man.Name), slog.Any("error", err))
	}

	provider, err := m.newProvider(ctx, man, entry)
	if err != nil {
		// 记录安装但保留暂停态，便于后台展示与排查。
		m.mu.Lock()
		m.installed[man.Name] = &installedPlugin{manifest: man, dir: dir}
		m.mu.Unlock()
		return err
	}
	m.activate(man, dir, provider)
	return nil
}

// newProvider 依据运行时构建插件实例。
func (m *Manager) newProvider(ctx context.Context, man Manifest, entry string) (Provider, error) {
	switch man.Runtime {
	case RuntimeWASM:
		return loadWASM(ctx, man, entry, RuntimeOptions{
			Logger:           m.opts.Logger,
			HTTPTimeout:      m.opts.HTTPTimeout,
			MaxHTTPBody:      m.opts.MaxHTTPBody,
			MemoryLimitPages: m.opts.MemoryLimitPages,
		})
	case RuntimeProcess:
		return loadProcess(ctx, man, entry, RuntimeOptions{
			Logger:      m.opts.Logger,
			HTTPTimeout: m.opts.HTTPTimeout,
			MaxHTTPBody: m.opts.MaxHTTPBody,
		})
	default:
		return nil, fmt.Errorf("plugin %q: runtime %q is not supported", man.Name, man.Runtime)
	}
}

// activate 记录插件实例并注册到注册表。
func (m *Manager) activate(man Manifest, dir string, provider Provider) {
	desc := provider.Descriptor()
	m.mu.Lock()
	m.installed[man.Name] = &installedPlugin{manifest: man, dir: dir, provider: provider, descriptor: desc, enabled: true}
	m.mu.Unlock()
	m.registry.Register(Entry{Manifest: man, Descriptor: desc, Provider: provider, Dir: dir})
}

// RegisterProvider 直接注册一个进程内 Provider（供内嵌插件与测试使用）。
func (m *Manager) RegisterProvider(man Manifest, provider Provider) {
	m.activate(man, "", provider)
}

// Enable 启用（或重新加载）一个已安装的插件，返回其描述。
func (m *Manager) Enable(ctx context.Context, name string) (Descriptor, error) {
	m.mu.Lock()
	lp, ok := m.installed[name]
	if !ok {
		m.mu.Unlock()
		return Descriptor{}, fmt.Errorf("%w: plugin %q", ErrNotFound, name)
	}
	if lp.enabled {
		desc := lp.descriptor
		m.mu.Unlock()
		return desc, nil
	}
	provider := lp.provider
	m.mu.Unlock()

	if provider == nil {
		man, entry, err := loadManifest(lp.dir)
		if err != nil {
			return Descriptor{}, err
		}
		p, err := m.newProvider(ctx, man, entry)
		if err != nil {
			return Descriptor{}, err
		}
		provider = p
	}

	desc := provider.Descriptor()
	m.mu.Lock()
	lp.provider = provider
	lp.descriptor = desc
	lp.enabled = true
	m.mu.Unlock()
	m.registry.Register(Entry{Manifest: lp.manifest, Descriptor: desc, Provider: provider, Dir: lp.dir})
	return desc, nil
}

// Disable 暂停一个插件：注销并（磁盘插件）关闭实例，但保留文件与配置。
func (m *Manager) Disable(ctx context.Context, name string) error {
	m.mu.Lock()
	lp, ok := m.installed[name]
	if !ok || !lp.enabled {
		m.mu.Unlock()
		return nil
	}
	lp.enabled = false
	provider := lp.provider
	// 磁盘插件暂停时关闭实例；进程内注册的 Provider 保留以便再次启用。
	if lp.dir != "" {
		lp.provider = nil
	}
	category := lp.descriptor.Category
	m.mu.Unlock()

	m.registry.Unregister(category, name)
	if lp.dir != "" && provider != nil {
		return provider.Close(ctx)
	}
	return nil
}

// Unload 是 Disable 的别名，保留以兼容既有调用。
func (m *Manager) Unload(ctx context.Context, name string) error {
	return m.Disable(ctx, name)
}

// Reload 重新加载指定名称的插件，并保持其启用状态。
func (m *Manager) Reload(ctx context.Context, name string) error {
	m.mu.RLock()
	lp, ok := m.installed[name]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: plugin %q", ErrNotFound, name)
	}
	wasEnabled := lp.enabled
	if lp.dir == "" {
		return nil // 进程内注册的 Provider 无需重新加载。
	}
	if err := m.Disable(ctx, name); err != nil {
		return err
	}
	if err := m.loadOne(ctx, lp.dir); err != nil {
		return err
	}
	if !wasEnabled {
		return m.Disable(ctx, name)
	}
	return nil
}

// Configure 向指定插件注入配置。已暂停的插件无法配置。
func (m *Manager) Configure(ctx context.Context, name string, config map[string]string) error {
	p, ok := m.Provider(name)
	if !ok {
		return fmt.Errorf("%w: plugin %q", ErrNotFound, name)
	}
	return p.Configure(ctx, config)
}

// Invoke 调用指定类别与名称的插件。
func (m *Manager) Invoke(ctx context.Context, category, name, op string, input []byte) ([]byte, error) {
	entry, ok := m.registry.Lookup(category, name)
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, category, name)
	}
	return entry.Provider.Invoke(ctx, op, input)
}

// List 返回指定类别已启用插件的描述；category 为空返回全部。
func (m *Manager) List(category string) []Descriptor {
	return m.registry.List(category)
}

// Describe 返回指定插件的描述（含已暂停插件，来自缓存或清单）。
func (m *Manager) Describe(name string) (Descriptor, bool) {
	m.mu.RLock()
	lp, ok := m.installed[name]
	m.mu.RUnlock()
	if !ok {
		return Descriptor{}, false
	}
	desc := lp.descriptor
	if desc.Name == "" {
		desc = Descriptor{
			Name:     lp.manifest.Name,
			Category: lp.manifest.Category,
			Version:  lp.manifest.Version,
		}
	}
	return desc, true
}

// Provider 返回指定插件已加载的实例；已暂停时返回 false。
func (m *Manager) Provider(name string) (Provider, bool) {
	m.mu.RLock()
	lp, ok := m.installed[name]
	m.mu.RUnlock()
	if !ok || !lp.enabled {
		return nil, false
	}
	return lp.provider, true
}

// Enabled 报告插件当前是否已启用。
func (m *Manager) Enabled(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lp, ok := m.installed[name]
	return ok && lp.enabled
}

// Installed 返回全部已安装插件的状态快照，按名称排序。
func (m *Manager) Installed() []InstalledInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]InstalledInfo, 0, len(m.installed))
	for _, lp := range m.installed {
		desc := lp.descriptor
		if desc.Name == "" {
			desc = Descriptor{Name: lp.manifest.Name, Category: lp.manifest.Category, Version: lp.manifest.Version}
		}
		out = append(out, InstalledInfo{
			Manifest:   lp.manifest,
			Descriptor: desc,
			Enabled:    lp.enabled,
			Dir:        lp.dir,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.Name < out[j].Manifest.Name })
	return out
}

// Loaded 返回已启用插件的清单快照，按名称排序。
func (m *Manager) Loaded() []Manifest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Manifest, 0, len(m.installed))
	for _, lp := range m.installed {
		if lp.enabled {
			out = append(out, lp.manifest)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// removeEntry 从安装表与注册表中删除一个插件（不关闭实例）；供 Remove 使用。
func (m *Manager) removeEntry(name string) {
	m.mu.Lock()
	lp, ok := m.installed[name]
	delete(m.installed, name)
	m.mu.Unlock()
	if ok && lp.descriptor.Name != "" {
		m.registry.Unregister(lp.descriptor.Category, name)
	}
}

// Close 关闭所有插件实例并关闭管理器。
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	installed := m.installed
	m.installed = make(map[string]*installedPlugin)
	m.mu.Unlock()

	var errs []error
	for name, lp := range installed {
		if lp.descriptor.Name != "" {
			m.registry.Unregister(lp.descriptor.Category, name)
		}
		if lp.provider != nil {
			if err := lp.provider.Close(ctx); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
