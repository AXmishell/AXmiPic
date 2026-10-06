package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// hostEnv 汇总一次插件加载的宿主侧能力与限额，作为宿主函数的接收者。
type hostEnv struct {
	name       string
	logger     *slog.Logger
	httpClient *http.Client
	allowHosts []string
	maxBody    int64
}

// instantiate 注册宿主导入模块。
func (h *hostEnv) instantiate(ctx context.Context, rt wazero.Runtime) error {
	_, err := rt.NewHostModuleBuilder(HostModule).
		NewFunctionBuilder().WithFunc(h.httpRequest).Export(HostFnHTTP).
		NewFunctionBuilder().WithFunc(h.log).Export(HostFnLog).
		NewFunctionBuilder().WithFunc(h.nowMillis).Export(HostFnNow).
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("plugin %q: instantiate host module: %w", h.name, err)
	}
	return nil
}

// httpRequest 是暴露给插件的唯一网络出口，强制执行主机白名单与体积限制。
func (h *hostEnv) httpRequest(ctx context.Context, mod api.Module, reqPtr, reqLen uint32) uint64 {
	resp := h.doHTTP(ctx, mod, reqPtr, reqLen)
	data, err := json.Marshal(resp)
	if err != nil {
		data, _ = json.Marshal(HTTPResponse{Error: "host: marshal response: " + err.Error()})
	}
	ptr, err := writeGuest(ctx, mod, data)
	if err != nil {
		h.logger.Warn("plugin http response write failed",
			slog.String("plugin", h.name),
			slog.Any("error", err))
		return 0
	}
	return pack(ptr, uint32(len(data)))
}

// doHTTP 解析请求、校验能力并执行。
func (h *hostEnv) doHTTP(ctx context.Context, mod api.Module, reqPtr, reqLen uint32) HTTPResponse {
	raw, ok := mod.Memory().Read(reqPtr, reqLen)
	if !ok {
		return HTTPResponse{Error: "host: request out of range"}
	}
	var req HTTPRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return HTTPResponse{Error: "host: invalid request JSON: " + err.Error()}
	}
	parsed, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil {
		return HTTPResponse{Error: "host: invalid url: " + err.Error()}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return HTTPResponse{Error: "host: only http/https is allowed"}
	}
	if !hostAllowed(parsed.Hostname(), h.allowHosts) {
		return HTTPResponse{Error: "host: host not allowed: " + parsed.Hostname()}
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodPost
	}
	if !methodAllowed(method) {
		return HTTPResponse{Error: "host: method not allowed: " + method}
	}
	if int64(len(req.Body)) > h.maxBody {
		return HTTPResponse{Error: "host: request body too large"}
	}

	var body io.Reader
	if len(req.Body) > 0 {
		body = strings.NewReader(string(req.Body))
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
	if err != nil {
		return HTTPResponse{Error: "host: build request: " + err.Error()}
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	res, err := h.httpClient.Do(httpReq)
	if err != nil {
		return HTTPResponse{Error: "host: request failed: " + err.Error()}
	}
	defer func() { _ = res.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(res.Body, h.maxBody+1))
	if err != nil {
		return HTTPResponse{Error: "host: read response: " + err.Error()}
	}
	if int64(len(data)) > h.maxBody {
		return HTTPResponse{Error: "host: response body too large"}
	}
	headers := make(map[string]string, len(res.Header))
	for k := range res.Header {
		headers[k] = res.Header.Get(k)
	}
	return HTTPResponse{Status: res.StatusCode, Headers: headers, Body: data}
}

// log 记录插件写入的调试日志。
func (h *hostEnv) log(_ context.Context, mod api.Module, level, ptr, length uint32) {
	buf, ok := mod.Memory().Read(ptr, length)
	if !ok {
		return
	}
	msg := string(buf)
	lvl := slog.LevelDebug
	switch level {
	case 1:
		lvl = slog.LevelInfo
	case 2:
		lvl = slog.LevelWarn
	case 3:
		lvl = slog.LevelError
	}
	h.logger.Log(context.Background(), lvl, msg, slog.String("plugin", h.name))
}

// nowMillis 返回当前 Unix 毫秒时间戳。
func (h *hostEnv) nowMillis(context.Context) uint64 {
	return uint64(time.Now().UnixMilli())
}

// writeGuest 在 guest 内存中分配并写入数据，返回分配指针。
func writeGuest(ctx context.Context, mod api.Module, data []byte) (uint32, error) {
	if len(data) == 0 {
		return 0, nil
	}
	alloc := mod.ExportedFunction(ExportAlloc)
	if alloc == nil {
		return 0, fmt.Errorf("guest has no %q export", ExportAlloc)
	}
	res, err := alloc.Call(ctx, uint64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("guest alloc: %w", err)
	}
	ptr := uint32(res[0])
	if ptr == 0 {
		return 0, fmt.Errorf("guest alloc returned null")
	}
	if !mod.Memory().Write(ptr, data) {
		return 0, fmt.Errorf("guest memory write out of range")
	}
	return ptr, nil
}

// hostAllowed 判断主机是否命中白名单，支持 *.example.com 通配子域与 "*"。
func hostAllowed(host string, patterns []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		switch {
		case p == "*":
			return true
		case strings.HasPrefix(p, "*."):
			suffix := p[1:] // ".example.com"
			if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
				return true
			}
		case host == p:
			return true
		}
	}
	return false
}

// methodAllowed 限制插件可使用的 HTTP 方法。
func methodAllowed(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
		return true
	default:
		return false
	}
}

// newHTTPClient 构造带超时的宿主 HTTP 客户端。
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// logWriter 把插件的 stderr 输出转发到结构化日志。
type logWriter struct {
	logger *slog.Logger
	name   string
}

// newLogWriter 构造一个转发 stderr 的 writer。
func newLogWriter(logger *slog.Logger, name string) io.Writer {
	if logger == nil {
		logger = slog.Default()
	}
	return &logWriter{logger: logger, name: name}
}

// Write 将写入内容按行记录到调试日志。
func (w *logWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	if msg != "" {
		w.logger.Debug(msg, slog.String("plugin", w.name), slog.String("stream", "stderr"))
	}
	return len(p), nil
}
