package plugin

import (
	"encoding/json"
	"errors"
)

// 宿主导入模块名与函数名。插件通过 //go:wasmimport axmipic <name> 引用。
const (
	// HostModule 是宿主向插件提供的 WASM 导入模块名。
	HostModule = "axmipic"
	// HostFnHTTP 发起一次受控的 HTTP 请求。
	HostFnHTTP = "http_request"
	// HostFnLog 记录一条调试日志。
	HostFnLog = "log"
	// HostFnNow 返回当前 Unix 毫秒时间戳。
	HostFnNow = "now_millis"
)

// 插件必须导出的函数名。
const (
	ExportAlloc     = "alloc"
	ExportDealloc   = "dealloc"
	ExportDescribe  = "describe"
	ExportConfigure = "configure"
	ExportInvoke    = "invoke"
)

// Envelope 是插件返回给宿主的结果信封。Result 为二进制安全的原始结果，
// 在 JSON 中以 base64 编码。
type Envelope struct {
	Result []byte `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// HTTPRequest 是插件向宿主发起的一次 HTTP 请求描述。
type HTTPRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
}

// HTTPResponse 是宿主返回给插件的 HTTP 响应描述。
type HTTPResponse struct {
	Status  int               `json:"status,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// pack 将一个 (指针, 长度) 对打包进 64 位整数：高 32 位为指针，低 32 位为长度。
func pack(ptr, length uint32) uint64 {
	return uint64(ptr)<<32 | uint64(length)
}

// unpack 拆解 pack 的结果。
func unpack(v uint64) (ptr, length uint32) {
	return uint32(v >> 32), uint32(v)
}

// decodeEnvelope 解析插件返回的结果信封。
func decodeEnvelope(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Error != "" {
		return nil, errors.New(env.Error)
	}
	return env.Result, nil
}
