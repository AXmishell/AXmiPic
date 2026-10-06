package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// DefaultMemoryLimitPages 是 WASM 插件默认的最大线性内存（以 64 KiB 页计）。
// 1024 页 = 64 MiB，足以容纳 Go 运行时并限制失控插件。
const DefaultMemoryLimitPages = 1024

// RuntimeOptions 是加载插件时的运行参数。
type RuntimeOptions struct {
	// Logger 为插件与宿主日志器。
	Logger *slog.Logger
	// HTTPTimeout 为单次宿主 HTTP 请求超时。
	HTTPTimeout time.Duration
	// MaxHTTPBody 为请求/响应体的最大字节数。
	MaxHTTPBody int64
	// MemoryLimitPages 为 WASM 线性内存上限（页）。为 0 时使用默认值。
	MemoryLimitPages uint32
}

func (o RuntimeOptions) withDefaults() RuntimeOptions {
	if o.HTTPTimeout <= 0 {
		o.HTTPTimeout = 10 * time.Second
	}
	if o.MaxHTTPBody <= 0 {
		o.MaxHTTPBody = 1 << 20 // 1 MiB
	}
	if o.MemoryLimitPages == 0 {
		o.MemoryLimitPages = DefaultMemoryLimitPages
	}
	return o
}

// wasmProvider 是一个运行在 wazero 沙箱内的 WASM 插件实例。
type wasmProvider struct {
	name    string
	runtime wazero.Runtime
	module  api.Module
	desc    Descriptor
	logger  *slog.Logger
	// mu 串行化对同一 guest 模块的调用；wazero 的模块调用不是并发安全的。
	mu     sync.Mutex
	closed bool
}

// loadWASM 加载一个 WASM 插件模块并返回可调用的 Provider。
func loadWASM(ctx context.Context, man Manifest, entry string, opts RuntimeOptions) (*wasmProvider, error) {
	opts = opts.withDefaults()
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	bin, err := readFile(entry)
	if err != nil {
		return nil, err
	}

	rtCfg := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(opts.MemoryLimitPages)
	rt := wazero.NewRuntimeWithConfig(ctx, rtCfg)

	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	env := &hostEnv{
		name:       man.Name,
		logger:     logger,
		allowHosts: man.httpHosts(),
		maxBody:    opts.MaxHTTPBody,
		httpClient: newHTTPClient(opts.HTTPTimeout),
	}
	if err := env.instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}

	cfg := wazero.NewModuleConfig().
		WithName(man.Name).
		WithStartFunctions("_initialize").
		WithStdout(io.Discard).
		WithStderr(newLogWriter(logger, man.Name))

	mod, err := rt.InstantiateWithConfig(ctx, bin, cfg)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("plugin %q: instantiate wasm: %w", man.Name, err)
	}

	p := &wasmProvider{name: man.Name, runtime: rt, module: mod, logger: logger}
	if err := p.verifyExports(); err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}

	desc, err := p.describe(ctx)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}
	// 清单是注册信息的权威来源；插件只需提供字段与展示信息。
	desc.Name = man.Name
	desc.Category = man.Category
	if desc.Version == "" {
		desc.Version = man.Version
	}
	if err := validateDescriptor(desc); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("plugin %q: %w", man.Name, err)
	}
	p.desc = desc
	logger.Info("plugin loaded",
		slog.String("plugin", man.Name),
		slog.String("category", desc.Category),
		slog.String("runtime", RuntimeWASM),
		slog.Int("fields", len(desc.Fields)),
	)
	return p, nil
}

// verifyExports 确认插件导出了 ABI 要求的全部函数。
func (p *wasmProvider) verifyExports() error {
	required := []string{ExportAlloc, ExportDealloc, ExportDescribe, ExportConfigure, ExportInvoke}
	for _, name := range required {
		if p.module.ExportedFunction(name) == nil {
			return fmt.Errorf("plugin %q: missing required export %q", p.name, name)
		}
	}
	return nil
}

// Descriptor 返回插件自描述。
func (p *wasmProvider) Descriptor() Descriptor { return p.desc }

// Configure 将配置注入插件。
func (p *wasmProvider) Configure(ctx context.Context, config map[string]string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if config == nil {
		config = map[string]string{}
	}
	payload, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("plugin %q: encode config: %w", p.name, err)
	}
	result, err := p.callWithBytes(ctx, ExportConfigure, payload)
	if err != nil {
		return fmt.Errorf("plugin %q: configure: %w", p.name, err)
	}
	if _, err := decodeEnvelope(result); err != nil {
		return fmt.Errorf("plugin %q: configure: %w", p.name, err)
	}
	return nil
}

// Invoke 执行一次插件操作。
func (p *wasmProvider) Invoke(ctx context.Context, op string, input []byte) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	result, err := p.callWithOpBytes(ctx, op, input)
	if err != nil {
		return nil, fmt.Errorf("plugin %q: invoke %s: %w", p.name, op, err)
	}
	out, err := decodeEnvelope(result)
	if err != nil {
		return nil, fmt.Errorf("plugin %q: invoke %s: %w", p.name, op, err)
	}
	return out, nil
}

// MemoryBytes 返回插件当前占用的 WASM 线性内存字节数。
func (p *wasmProvider) MemoryBytes() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.module == nil {
		return 0
	}
	return uint64(p.module.Memory().Size())
}

// Close 关闭插件运行时。它持有 mu，确保不会与在途的 Invoke/Configure 竞争。
func (p *wasmProvider) Close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return p.runtime.Close(ctx)
}

// describe 调用插件的 describe 导出并解析其自描述。
func (p *wasmProvider) describe(ctx context.Context) (Descriptor, error) {
	res, err := p.module.ExportedFunction(ExportDescribe).Call(ctx)
	if err != nil {
		return Descriptor{}, fmt.Errorf("plugin %q: describe: %w", p.name, err)
	}
	data, err := p.readPacked(res[0])
	if err != nil {
		return Descriptor{}, fmt.Errorf("plugin %q: describe: %w", p.name, err)
	}
	var desc Descriptor
	if err := json.Unmarshal(data, &desc); err != nil {
		return Descriptor{}, fmt.Errorf("plugin %q: describe: invalid JSON: %w", p.name, err)
	}
	return desc, nil
}

// callWithBytes 以单段字节参数调用导出函数（用于 configure）。
func (p *wasmProvider) callWithBytes(ctx context.Context, fn string, payload []byte) ([]byte, error) {
	ptr, err := p.writeGuest(ctx, payload)
	if err != nil {
		return nil, err
	}
	defer p.freeGuest(ctx, ptr, uint32(len(payload)))
	res, err := p.module.ExportedFunction(fn).Call(ctx, uint64(ptr), uint64(len(payload)))
	if err != nil {
		return nil, err
	}
	return p.readPacked(res[0])
}

// callWithOpBytes 以操作名与输入两段字节调用导出函数（用于 invoke）。
func (p *wasmProvider) callWithOpBytes(ctx context.Context, op string, input []byte) ([]byte, error) {
	opPtr, err := p.writeGuest(ctx, []byte(op))
	if err != nil {
		return nil, err
	}
	defer p.freeGuest(ctx, opPtr, uint32(len(op)))
	inPtr, err := p.writeGuest(ctx, input)
	if err != nil {
		return nil, err
	}
	defer p.freeGuest(ctx, inPtr, uint32(len(input)))
	res, err := p.module.ExportedFunction(ExportInvoke).Call(ctx,
		uint64(opPtr), uint64(len(op)), uint64(inPtr), uint64(len(input)))
	if err != nil {
		return nil, err
	}
	return p.readPacked(res[0])
}

// writeGuest 在插件内存中分配并写入数据，返回指针。
func (p *wasmProvider) writeGuest(ctx context.Context, data []byte) (uint32, error) {
	if len(data) == 0 {
		return 0, nil
	}
	res, err := p.module.ExportedFunction(ExportAlloc).Call(ctx, uint64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("plugin %q: alloc: %w", p.name, err)
	}
	ptr := uint32(res[0])
	if ptr == 0 {
		return 0, fmt.Errorf("plugin %q: alloc returned null", p.name)
	}
	if !p.module.Memory().Write(ptr, data) {
		return 0, fmt.Errorf("plugin %q: write guest memory out of range", p.name)
	}
	return ptr, nil
}

// freeGuest 释放之前由 writeGuest 分配的内存。
func (p *wasmProvider) freeGuest(ctx context.Context, ptr, size uint32) {
	if ptr == 0 || size == 0 {
		return
	}
	if _, err := p.module.ExportedFunction(ExportDealloc).Call(ctx, uint64(ptr), uint64(size)); err != nil {
		p.logger.Debug("plugin dealloc failed",
			slog.String("plugin", p.name),
			slog.Any("error", err))
	}
}

// readPacked 读取由 pack 编码的 guest 内存段。
func (p *wasmProvider) readPacked(packed uint64) ([]byte, error) {
	ptr, length := unpack(packed)
	if length == 0 {
		return nil, nil
	}
	data, ok := p.module.Memory().Read(ptr, length)
	if !ok {
		return nil, fmt.Errorf("plugin %q: read guest memory out of range", p.name)
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

// validateDescriptor 校验插件自描述的类别与字段。
func validateDescriptor(d Descriptor) error {
	if d.Category == "" {
		return fmt.Errorf("descriptor category is required")
	}
	if d.Name == "" {
		return fmt.Errorf("descriptor name is required")
	}
	seen := make(map[string]struct{}, len(d.Fields))
	for _, f := range d.Fields {
		if f.Key == "" {
			return fmt.Errorf("descriptor field key is required")
		}
		if _, ok := seen[f.Key]; ok {
			return fmt.Errorf("duplicate field key %q", f.Key)
		}
		seen[f.Key] = struct{}{}
		switch f.Type {
		case "", FieldString, FieldPassword, FieldInt, FieldBool, FieldSelect:
		default:
			return fmt.Errorf("field %q: unknown type %q", f.Key, f.Type)
		}
	}
	return nil
}
