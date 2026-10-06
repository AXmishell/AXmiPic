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

// installedPlugin 是一个已安装的插件。
//
// 它区分「逻辑启用」（enabled）与「物理加载」（loaded/provider）：插件可以被
// 暂停（enabled=false）或空闲卸载（enabled=true 但 loader/provider=nil），
// 后者在下次调用时按需重新实例化，从而在不重启进程的前提下回收其内存。
type installedPlugin struct {
	manifest   Manifest
	dir        string
	provider   Provider
	descriptor Descriptor
	enabled    bool
	loaded     bool
	lastUsed   time.Time
	lastError  string
}

// InstalledInfo 是插件的安装/启用/加载状态快照。
type InstalledInfo struct {
	Manifest   Manifest
	Descriptor Descriptor
	// Enabled 为逻辑启用（未被暂停）。
	Enabled bool
	// Loaded 为当前是否已实例化（占用内存）。
	Loaded bool
	// MemoryBytes 为已加载插件报告的内存占用（WASM 线性内存）；未知时为 0。
	MemoryBytes uint64
	// LastUsed 为最近一次调用的时间；零值表示加载后尚未被调用。
	LastUsed time.Time
	// LastError 为最近一次激活失败的描述；成功后清空。
	LastError string
	Dir       string
}

// memoryReporter 是可选接口：已加载插件可报告其当前内存占用（字节）。
type memoryReporter interface {
	MemoryBytes() uint64
}

// providerMemory 返回插件报告的内存占用；不支持时返回 0。
func providerMemory(p Provider) uint64 {
	if p == nil {
		return 0
	}
	if r, ok := p.(memoryReporter); ok {
		return r.MemoryBytes()
	}
	return 0
}

// Manager 负责发现、加载、配置并管理插件，是插件系统的门面。
type Manager struct {
	opts     Options
	registry *Registry

	mu        sync.RWMutex
	installed map[string]*installedPlugin
	closed    bool
	// loadMu 串行化实例化，避免并发首次调用重复加载同一插件。
	loadMu sync.Mutex
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

// Registry 返回底层注册表，供只读查询已启用（且已加载）插件的描述。
func (m *Manager) Registry() *Registry { return m.registry }

// Load 加载 Options.Dir 下的全部插件并启用。
func (m *Manager) Load(ctx context.Context) error {
	return m.LoadFiltered(ctx, nil, nil)
}

// LoadFiltered 扫描 Options.Dir 下的插件。
//
// include 决定是否**实例化**：为 nil 时全部实例化；否则仅对 include(name) 为
// true 的插件实例化，其余登记为「已安装但未加载」，待首次使用时按需加载。
// enabled 决定**逻辑启用状态**（为 nil 时默认启用），对未实例化的插件同样生效，
// 因此可以在不加载的前提下保留其启用/暂停意图。
func (m *Manager) LoadFiltered(ctx context.Context, include, enabled func(name string) bool) error {
	if m.opts.Dir == "" {
		return nil
	}
	return m.loadDir(ctx, m.opts.Dir, include, enabled)
}

// LoadDir 扫描目录并加载其中的全部插件。单个插件加载失败不会阻止其它插件。
func (m *Manager) LoadDir(ctx context.Context, dir string) error {
	return m.loadDir(ctx, dir, nil, nil)
}

// loadDir 按 include/enabled 过滤扫描并加载插件目录。
func (m *Manager) loadDir(ctx context.Context, dir string, include, enabled func(name string) bool) error {
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
		man, entry, err := loadManifest(dir)
		if err != nil {
			return err
		}
		if include == nil || include(man.Name) {
			return m.loadResolved(ctx, man, dir, entry)
		}
		m.registerUnloaded(man, dir, enabled == nil || enabled(man.Name))
		return nil
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
		man, entry, err := loadManifest(sub)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if include == nil || include(man.Name) {
			if err := m.loadResolved(ctx, man, sub, entry); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		m.registerUnloaded(man, sub, enabled == nil || enabled(man.Name))
	}
	return errors.Join(errs...)
}

// loadOne 读取清单并加载单个插件目录（供安装/重载路径使用）。
func (m *Manager) loadOne(ctx context.Context, dir string) error {
	man, entry, err := loadManifest(dir)
	if err != nil {
		return err
	}
	return m.loadResolved(ctx, man, dir, entry)
}

// registerUnloaded 登记一个已安装但未实例化的插件，enabled 为其逻辑启用状态。
func (m *Manager) registerUnloaded(man Manifest, dir string, enabled bool) {
	m.mu.Lock()
	m.installed[man.Name] = &installedPlugin{
		manifest:   man,
		dir:        dir,
		descriptor: m.manifestDescriptor(man),
		enabled:    enabled,
	}
	m.mu.Unlock()
}

// loadResolved 实例化一个已解析清单的插件并注册。
func (m *Manager) loadResolved(ctx context.Context, man Manifest, dir, entry string) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return fmt.Errorf("plugin: manager is closed")
	}
	m.mu.Unlock()

	// 先关闭同名的旧实例（若存在）。
	if err := m.Disable(ctx, man.Name); err != nil {
		m.opts.Logger.Warn("failed to disable previous plugin",
			slog.String("plugin", man.Name), slog.Any("error", err))
	}

	provider, err := m.newProvider(ctx, man, entry)
	if err != nil {
		// 记录安装但保留未加载态，便于后台展示与排查。
		m.mu.Lock()
		m.installed[man.Name] = &installedPlugin{
			manifest:   man,
			dir:        dir,
			descriptor: m.manifestDescriptor(man),
			lastError:  err.Error(),
		}
		m.mu.Unlock()
		return err
	}

	desc := descriptorFromManifest(man, provider.Descriptor())
	m.mu.Lock()
	m.installed[man.Name] = &installedPlugin{
		manifest:   man,
		dir:        dir,
		provider:   provider,
		descriptor: desc,
		enabled:    true,
		loaded:     true,
		lastUsed:   time.Now(),
	}
	m.mu.Unlock()
	m.registry.Register(Entry{Manifest: man, Descriptor: desc, Provider: provider, Dir: dir})
	return nil
}

// activate 确保指定插件已实例化并返回其 Provider。它尊重「逻辑启用」状态，
// 但不修改它，因此可安全用于空闲卸载后的按需重载。
func (m *Manager) activate(ctx context.Context, name string) (Provider, error) {
	m.mu.RLock()
	lp, ok := m.installed[name]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("%w: plugin %q", ErrNotFound, name)
	}
	if lp.provider != nil {
		p := lp.provider
		m.mu.RUnlock()
		return p, nil
	}
	dir := lp.dir
	m.mu.RUnlock()
	if dir == "" {
		return nil, fmt.Errorf("%w: plugin %q is not loaded", ErrNotFound, name)
	}

	m.loadMu.Lock()
	defer m.loadMu.Unlock()

	// 双重检查：等待期间可能已被其它调用加载。
	m.mu.RLock()
	lp, ok = m.installed[name]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("%w: plugin %q", ErrNotFound, name)
	}
	if lp.provider != nil {
		p := lp.provider
		m.mu.RUnlock()
		return p, nil
	}
	man := lp.manifest
	m.mu.RUnlock()

	entry, err := man.resolveEntry(dir)
	if err != nil {
		m.setLastError(name, err.Error())
		return nil, err
	}
	provider, err := m.newProvider(ctx, man, entry)
	if err != nil {
		m.setLastError(name, err.Error())
		return nil, err
	}
	desc := descriptorFromManifest(man, provider.Descriptor())

	m.mu.Lock()
	lp, ok = m.installed[name]
	if !ok {
		m.mu.Unlock()
		_ = provider.Close(ctx)
		return nil, fmt.Errorf("%w: plugin %q", ErrNotFound, name)
	}
	lp.provider = provider
	lp.descriptor = desc
	lp.loaded = true
	lp.lastUsed = time.Now()
	lp.lastError = ""
	m.mu.Unlock()

	m.registry.Register(Entry{Manifest: man, Descriptor: desc, Provider: provider, Dir: dir})
	return provider, nil
}

// touch 更新插件的最近使用时间，用于空闲自动卸载。
func (m *Manager) touch(name string) {
	m.mu.Lock()
	if lp, ok := m.installed[name]; ok {
		lp.lastUsed = time.Now()
	}
	m.mu.Unlock()
}

// setLastError 记录最近一次激活/加载失败的原因。
func (m *Manager) setLastError(name, msg string) {
	m.mu.Lock()
	if lp, ok := m.installed[name]; ok {
		lp.lastError = msg
	}
	m.mu.Unlock()
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

// Enable 将插件标记为「已启用」。它**不会实例化**插件：插件进入待激活
// （standby）状态，直到首次被调用时才真正加载。若插件已加载（例如进程内
// Provider 或空闲卸载前的实例），则重新注册到注册表（不改变启用状态）。
func (m *Manager) Enable(_ context.Context, name string) (Descriptor, error) {
	m.mu.Lock()
	lp, ok := m.installed[name]
	if !ok {
		m.mu.Unlock()
		return Descriptor{}, fmt.Errorf("%w: plugin %q", ErrNotFound, name)
	}
	lp.enabled = true
	d := lp.descriptor
	if d.Name == "" {
		d = m.manifestDescriptor(lp.manifest)
	}
	man, dir, provider := lp.manifest, lp.dir, lp.provider
	m.mu.Unlock()

	if provider != nil {
		m.registry.Register(Entry{Manifest: man, Descriptor: d, Provider: provider, Dir: dir})
	}
	return d, nil
}

// Disable 暂停一个插件：注销并（磁盘插件）关闭其实例，但保留文件与配置。
// 暂停会释放插件占用的内存，内存中不再保留其实例。
func (m *Manager) Disable(ctx context.Context, name string) error {
	m.mu.Lock()
	lp, ok := m.installed[name]
	if !ok || !lp.enabled {
		m.mu.Unlock()
		return nil
	}
	lp.enabled = false
	man := lp.manifest
	var provider Provider
	if lp.dir != "" {
		provider = lp.provider
		lp.provider = nil
		lp.loaded = false
	}
	m.mu.Unlock()

	m.registry.Unregister(man.Category, name)
	if provider != nil {
		return provider.Close(ctx)
	}
	return nil
}

// Suspend 卸载一个已加载插件的实例以释放内存，但保持其逻辑启用状态：下次
// 调用时会按需重新加载。进程内注册的 Provider 不会被卸载。
func (m *Manager) Suspend(ctx context.Context, name string) error {
	m.mu.Lock()
	lp, ok := m.installed[name]
	if !ok || lp.dir == "" || !lp.loaded || lp.provider == nil {
		m.mu.Unlock()
		return nil
	}
	provider := lp.provider
	lp.provider = nil
	lp.loaded = false
	man := lp.manifest
	m.mu.Unlock()

	m.registry.Unregister(man.Category, name)
	return provider.Close(ctx)
}

// SuspendIdle 卸载已启用、已加载且空闲超过 idle 的磁盘插件，返回被卸载的名称。
// idle<=0 时不执行任何操作。
func (m *Manager) SuspendIdle(ctx context.Context, idle time.Duration) []string {
	if idle <= 0 {
		return nil
	}
	cutoff := time.Now().Add(-idle)
	m.mu.RLock()
	var candidates []string
	for name, lp := range m.installed {
		if lp.enabled && lp.loaded && lp.dir != "" && !lp.lastUsed.After(cutoff) {
			candidates = append(candidates, name)
		}
	}
	m.mu.RUnlock()

	var suspended []string
	for _, name := range candidates {
		if err := m.Suspend(ctx, name); err != nil {
			m.opts.Logger.Warn("failed to suspend idle plugin",
				slog.String("plugin", name), slog.Any("error", err))
			continue
		}
		suspended = append(suspended, name)
	}
	return suspended
}

// SetDescriptor 为未加载的插件注入缓存的描述（供后台表单与渠道列表使用）。
func (m *Manager) SetDescriptor(name string, desc Descriptor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lp, ok := m.installed[name]
	if !ok || lp.loaded {
		return
	}
	if desc.Name == "" {
		desc.Name = lp.manifest.Name
	}
	if desc.Category == "" {
		desc.Category = lp.manifest.Category
	}
	if desc.Version == "" {
		desc.Version = lp.manifest.Version
	}
	lp.descriptor = desc
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

// Configure 向指定插件注入配置。未加载但已启用的插件会先按需加载。
func (m *Manager) Configure(ctx context.Context, name string, config map[string]string) error {
	p, err := m.Acquire(ctx, name)
	if err != nil {
		return err
	}
	if err := p.Configure(ctx, config); err != nil {
		return err
	}
	m.touch(name)
	return nil
}

// Invoke 调用指定类别与名称的插件。未加载但已启用的插件会先按需加载。
func (m *Manager) Invoke(ctx context.Context, category, name, op string, input []byte) ([]byte, error) {
	p, err := m.Acquire(ctx, name)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	lp, ok := m.installed[name]
	cat := ""
	if ok {
		cat = lp.manifest.Category
	}
	m.mu.RUnlock()
	if !ok || cat != category {
		return nil, fmt.Errorf("%w: %s/%s", ErrNotFound, category, name)
	}
	out, err := p.Invoke(ctx, op, input)
	m.touch(name)
	return out, err
}

// Acquire 返回一个已启用插件的 Provider，必要时按需加载。
func (m *Manager) Acquire(ctx context.Context, name string) (Provider, error) {
	m.mu.RLock()
	lp, ok := m.installed[name]
	enabled := ok && lp.enabled
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: plugin %q", ErrNotFound, name)
	}
	if !enabled {
		return nil, fmt.Errorf("%w: plugin %q is not enabled", ErrNotFound, name)
	}
	return m.activate(ctx, name)
}

// Has 报告指定类别下是否存在已启用的插件。
func (m *Manager) Has(category, name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lp, ok := m.installed[name]
	return ok && lp.enabled && lp.manifest.Category == category
}

// List 返回指定类别已启用插件的描述；category 为空返回全部。未加载但已启用
// 的插件使用其缓存或清单描述，因此后台在惰性加载下仍能看到它们。
func (m *Manager) List(category string) []Descriptor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Descriptor, 0, len(m.installed))
	for _, lp := range m.installed {
		if !lp.enabled {
			continue
		}
		d := lp.descriptor
		if d.Name == "" {
			d = m.manifestDescriptor(lp.manifest)
		}
		if category != "" && d.Category != category {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Describe 返回指定插件的描述（含未加载插件，来自缓存或清单）。
func (m *Manager) Describe(name string) (Descriptor, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lp, ok := m.installed[name]
	if !ok {
		return Descriptor{}, false
	}
	d := lp.descriptor
	if d.Name == "" {
		d = m.manifestDescriptor(lp.manifest)
	}
	return d, true
}

// manifestDescriptor 返回仅依据清单的描述（不含插件自报字段）。
func (m *Manager) manifestDescriptor(man Manifest) Descriptor {
	return Descriptor{Name: man.Name, Category: man.Category, Version: man.Version}
}

// descriptorFromManifest 以清单信息补全插件自描述。
func descriptorFromManifest(man Manifest, desc Descriptor) Descriptor {
	desc.Name = man.Name
	desc.Category = man.Category
	if desc.Version == "" {
		desc.Version = man.Version
	}
	return desc
}

// Provider 返回指定插件已加载的实例（且已启用）；暂停或空闲卸载后返回 false。
func (m *Manager) Provider(name string) (Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lp, ok := m.installed[name]
	if !ok || !lp.enabled || lp.provider == nil {
		return nil, false
	}
	return lp.provider, true
}

// Enabled 报告插件当前是否已启用（逻辑状态，与是否已加载无关）。
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
		d := lp.descriptor
		if d.Name == "" {
			d = m.manifestDescriptor(lp.manifest)
		}
		out = append(out, InstalledInfo{
			Manifest:    lp.manifest,
			Descriptor:  d,
			Enabled:     lp.enabled,
			Loaded:      lp.loaded,
			MemoryBytes: providerMemory(lp.provider),
			LastUsed:    lp.lastUsed,
			LastError:   lp.lastError,
			Dir:         lp.dir,
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

// RegisterProvider 直接注册一个进程内 Provider（供内嵌插件与测试使用）。
// 进程内 Provider 常驻内存，不会因空闲或暂停而卸载。
func (m *Manager) RegisterProvider(man Manifest, provider Provider) {
	desc := descriptorFromManifest(man, provider.Descriptor())
	m.mu.Lock()
	m.installed[man.Name] = &installedPlugin{
		manifest:   man,
		provider:   provider,
		descriptor: desc,
		enabled:    true,
		loaded:     true,
		lastUsed:   time.Now(),
	}
	m.mu.Unlock()
	m.registry.Register(Entry{Manifest: man, Descriptor: desc, Provider: provider})
}

// removeEntry 从安装表与注册表中删除一个插件（不关闭实例）；供 Remove 使用。
func (m *Manager) removeEntry(name string) {
	m.mu.Lock()
	lp, ok := m.installed[name]
	delete(m.installed, name)
	m.mu.Unlock()
	if ok {
		m.registry.Unregister(lp.manifest.Category, name)
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
		m.registry.Unregister(lp.manifest.Category, name)
		if lp.provider != nil {
			if err := lp.provider.Close(ctx); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
