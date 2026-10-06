package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// 进程插件的 stdio JSON 协议方法名。
const (
	procMethodDescribe  = "describe"
	procMethodConfigure = "configure"
	procMethodInvoke    = "invoke"
	procMethodClose     = "close"
)

// procCloseGrace 是 Close 时等待子进程退出的宽限期。
const procCloseGrace = 3 * time.Second

// procRequest 是宿主发往插件进程的请求。Params 为方法参数（可为空）。
type procRequest struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// procResponse 是插件进程返回给宿主的响应。
type procResponse struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// procConfigureParams 是 configure 方法的参数。
type procConfigureParams struct {
	Config map[string]string `json:"config"`
}

// procInvokeParams 是 invoke 方法的参数。Input 以 base64 编码（[]byte 的默认 JSON 形式）。
type procInvokeParams struct {
	Op    string `json:"op"`
	Input []byte `json:"input,omitempty"`
}

// procInvokeResult 是 invoke 方法的结果。
type procInvokeResult struct {
	Output []byte `json:"output,omitempty"`
}

// processProvider 是一个以独立子进程运行、通过 stdio JSON 通信的插件实例。
//
// 注意：进程插件是受信任的代码，运行在宿主同一权限下，不受 WASM 沙箱与 HTTP
// 能力白名单约束。
type processProvider struct {
	name    string
	desc    Descriptor
	logger  *slog.Logger
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	encoder *json.Encoder

	// callMu 串行化请求/响应，保证一次只有一条在途请求。
	callMu sync.Mutex
	// stateMu 保护 closed 与 readErr。
	stateMu sync.RWMutex
	closed  bool
	readErr error

	nextID uint64
	respCh chan procResponse
}

// loadProcess 启动一个进程插件并完成 describe。
func loadProcess(ctx context.Context, man Manifest, entry string, opts RuntimeOptions) (*processProvider, error) {
	opts = opts.withDefaults()
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	info, err := os.Stat(entry)
	if err != nil {
		return nil, fmt.Errorf("plugin %q: stat entry: %w", man.Name, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("plugin %q: entry is a directory", man.Name)
	}
	if info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("plugin %q: entry %s is not executable", man.Name, filepath.Base(entry))
	}
	// 使用绝对路径启动，避免相对入口在子进程工作目录下被二次拼接。
	absEntry, err := filepath.Abs(entry)
	if err != nil {
		return nil, fmt.Errorf("plugin %q: resolve entry: %w", man.Name, err)
	}

	logger.Warn("loading trusted process plugin; it runs with host privileges and is not sandboxed",
		"plugin", man.Name)

	cmd := exec.Command(absEntry)
	cmd.Dir = filepath.Dir(absEntry)
	cmd.Env = processEnv()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin %q: stdin pipe: %w", man.Name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin %q: stdout pipe: %w", man.Name, err)
	}
	cmd.Stderr = newLogWriter(logger, man.Name)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("plugin %q: start process: %w", man.Name, err)
	}

	p := &processProvider{
		name:    man.Name,
		logger:  logger,
		cmd:     cmd,
		stdin:   stdin,
		encoder: json.NewEncoder(stdin),
		respCh:  make(chan procResponse),
	}
	go p.readLoop(stdout)

	p.desc, err = p.describe(ctx)
	if err != nil {
		_ = p.Close(context.Background())
		return nil, err
	}
	p.desc.Name = man.Name
	p.desc.Category = man.Category
	if p.desc.Version == "" {
		p.desc.Version = man.Version
	}
	if err := validateDescriptor(p.desc); err != nil {
		_ = p.Close(context.Background())
		return nil, fmt.Errorf("plugin %q: %w", man.Name, err)
	}
	logger.Info("plugin loaded",
		"plugin", man.Name,
		"category", p.desc.Category,
		"runtime", RuntimeProcess,
		"fields", len(p.desc.Fields),
	)
	return p, nil
}

// readLoop 持续读取插件进程的响应并投递到 respCh；在出错或进程退出时关闭通道。
func (p *processProvider) readLoop(stdout io.Reader) {
	dec := json.NewDecoder(stdout)
	for {
		var resp procResponse
		if err := dec.Decode(&resp); err != nil {
			p.stateMu.Lock()
			if p.readErr == nil {
				p.readErr = err
			}
			p.stateMu.Unlock()
			close(p.respCh)
			return
		}
		p.respCh <- resp
	}
}

// Descriptor 返回插件自描述。
func (p *processProvider) Descriptor() Descriptor { return p.desc }

// MemoryBytes 报告插件进程的内存占用。进程插件的内存由操作系统管理，宿主
// 无法直接读取，返回 0 表示未知。
func (p *processProvider) MemoryBytes() uint64 { return 0 }

// describe 调用插件的 describe 方法。
func (p *processProvider) describe(ctx context.Context) (Descriptor, error) {
	var desc Descriptor
	if err := p.call(ctx, procMethodDescribe, nil, &desc); err != nil {
		return Descriptor{}, fmt.Errorf("plugin %q: describe: %w", p.name, err)
	}
	return desc, nil
}

// Configure 将配置注入插件。
func (p *processProvider) Configure(ctx context.Context, config map[string]string) error {
	if config == nil {
		config = map[string]string{}
	}
	return p.call(ctx, procMethodConfigure, procConfigureParams{Config: config}, nil)
}

// Invoke 执行一次插件操作。
func (p *processProvider) Invoke(ctx context.Context, op string, input []byte) ([]byte, error) {
	var result procInvokeResult
	if err := p.call(ctx, procMethodInvoke, procInvokeParams{Op: op, Input: input}, &result); err != nil {
		return nil, err
	}
	return result.Output, nil
}

// Close 关闭插件进程。
func (p *processProvider) Close(_ context.Context) error {
	p.callMu.Lock()
	defer p.callMu.Unlock()

	p.stateMu.Lock()
	if p.closed {
		p.stateMu.Unlock()
		return nil
	}
	p.closed = true
	p.stateMu.Unlock()

	// 尽力通知插件退出，然后关闭 stdin 促使进程结束。
	_ = p.encoder.Encode(procRequest{Method: procMethodClose})
	_ = p.stdin.Close()

	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(procCloseGrace):
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		<-done
	}
	return nil
}

// call 发送一次请求并等待对应响应，遵守 ctx 取消。
func (p *processProvider) call(ctx context.Context, method string, params any, out any) error {
	p.callMu.Lock()
	defer p.callMu.Unlock()

	p.stateMu.RLock()
	closed := p.closed
	p.stateMu.RUnlock()
	if closed {
		return fmt.Errorf("plugin %q: process is closed", p.name)
	}

	p.nextID++
	req := procRequest{ID: p.nextID, Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("plugin %q: encode params: %w", p.name, err)
		}
		req.Params = raw
	}
	if err := p.encoder.Encode(req); err != nil {
		return fmt.Errorf("plugin %q: write request: %w", p.name, err)
	}

	select {
	case resp, ok := <-p.respCh:
		if !ok {
			return fmt.Errorf("plugin %q: process ended: %w", p.name, p.readError())
		}
		if resp.ID != req.ID {
			return fmt.Errorf("plugin %q: response id mismatch", p.name)
		}
		if resp.Error != "" {
			return errors.New(resp.Error)
		}
		if out != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, out); err != nil {
				return fmt.Errorf("plugin %q: decode result: %w", p.name, err)
			}
		}
		return nil
	case <-ctx.Done():
		// 超时后流可能失步；标记进程不可用，等待其退出。
		go func() { _ = p.Close(context.Background()) }()
		return fmt.Errorf("plugin %q: %w", p.name, ctx.Err())
	}
}

// readError 返回读取错误（若有）。
func (p *processProvider) readError() error {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	if p.readErr != nil {
		return p.readErr
	}
	return io.EOF
}

// processEnv 返回给插件进程的最小环境变量集合。
func processEnv() []string {
	env := []string{"AXMIPIC_PLUGIN=1"}
	for _, key := range []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}
	return env
}
