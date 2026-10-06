// Package process 是 AXmiPic 进程外插件的 Go 编写 SDK。
//
// 插件实现 plugin.Provider，并在 main 中调用 process.Main：
//
//	func main() { process.Main(&myProvider{}) }
//
// 宿主以子进程方式启动插件，双方通过 stdin/stdout 上的换行分隔 JSON 请求/响应
// 通信；插件日志请写入 stderr。相比 WASM 插件，进程插件不受沙箱约束，可使用任意
// Go 库（含云厂商官方 SDK）与网络。
package process

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	plugin "github.com/AXmishell/axmipic/sdk/plugin-go"
)

// 协议方法名，需与宿主 internal/plugin 保持一致。
const (
	methodDescribe  = "describe"
	methodConfigure = "configure"
	methodInvoke    = "invoke"
	methodClose     = "close"
)

// request 是宿主发来的请求。
type request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// response 是插件返回的响应。
type response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type configureParams struct {
	Config map[string]string `json:"config"`
}

type invokeParams struct {
	Op    string `json:"op"`
	Input []byte `json:"input,omitempty"`
}

type invokeResult struct {
	Output []byte `json:"output,omitempty"`
}

// Main 运行插件主循环，直到 stdio 关闭或收到 close 请求。
func Main(p plugin.Provider) {
	if err := Serve(p); err != nil {
		fmt.Fprintln(os.Stderr, "axmipic process plugin:", err)
		os.Exit(1)
	}
}

// Serve 运行插件主循环。
func Serve(p plugin.Provider) error {
	if p == nil {
		return errors.New("process: provider must not be nil")
	}
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for {
		var req request
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("decode request: %w", err)
		}
		resp, done := dispatch(p, req)
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("encode response: %w", err)
		}
		if done {
			return nil
		}
	}
}

// dispatch 处理单个请求，返回响应以及是否结束主循环。
func dispatch(p plugin.Provider, req request) (resp response, done bool) {
	resp.ID = req.ID
	defer func() {
		if r := recover(); r != nil {
			resp = response{ID: req.ID, Error: fmt.Sprintf("panic: %v", r)}
			done = false
		}
	}()

	switch req.Method {
	case methodDescribe:
		data, err := json.Marshal(p.Describe())
		if err != nil {
			resp.Error = err.Error()
			return resp, false
		}
		resp.Result = data
		return resp, false

	case methodConfigure:
		var params configureParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = "invalid configure params: " + err.Error()
			return resp, false
		}
		if err := p.Configure(params.Config); err != nil {
			resp.Error = err.Error()
		}
		return resp, false

	case methodInvoke:
		var params invokeParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = "invalid invoke params: " + err.Error()
			return resp, false
		}
		out, err := p.Invoke(params.Op, params.Input)
		if err != nil {
			resp.Error = err.Error()
			return resp, false
		}
		data, err := json.Marshal(invokeResult{Output: out})
		if err != nil {
			resp.Error = err.Error()
			return resp, false
		}
		resp.Result = data
		return resp, false

	case methodClose:
		return resp, true

	default:
		resp.Error = "unknown method: " + req.Method
		return resp, false
	}
}
