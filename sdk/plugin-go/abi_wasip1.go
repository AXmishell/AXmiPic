//go:build wasip1

package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"unsafe"
)

// ---- 宿主导入 ----

//go:wasmimport axmipic http_request
func hostHTTPRequest(ptr, length uint32) uint64

//go:wasmimport axmipic log
func hostLog(level, ptr, length uint32)

//go:wasmimport axmipic now_millis
func hostNowMillis() uint64

// ---- ABI 辅助 ----

// allocations 持有通过 alloc 暴露给宿主的缓冲区，防止被 GC 回收。
var allocations = map[uint32][]byte{}

// result 持有最近一次返回给宿主的结果缓冲区。
var result []byte

//go:wasmexport alloc
func wasmAlloc(size uint32) uint32 {
	if size == 0 {
		return 0
	}
	b := make([]byte, size)
	p := ptrOf(b)
	allocations[p] = b
	return p
}

//go:wasmexport dealloc
func wasmDealloc(ptr, size uint32) {
	delete(allocations, ptr)
}

//go:wasmexport describe
func wasmDescribe() uint64 {
	if impl == nil {
		return packBytes([]byte(`{"category":"","name":"","title":"","error":"plugin: no provider registered"}`))
	}
	data, err := json.Marshal(impl.Describe())
	if err != nil {
		return packBytes([]byte(fmt.Sprintf(`{"category":"","name":"","error":%q}`, err.Error())))
	}
	return packBytes(data)
}

//go:wasmexport configure
func wasmConfigure(ptr, length uint32) uint64 {
	if impl == nil {
		return packEnvelope(nil, errors.New("plugin: no provider registered"))
	}
	var cfg map[string]string
	if err := json.Unmarshal(readGuest(ptr, length), &cfg); err != nil {
		return packEnvelope(nil, fmt.Errorf("plugin: invalid config: %w", err))
	}
	return packEnvelope(nil, impl.Configure(cfg))
}

//go:wasmexport invoke
func wasmInvoke(opPtr, opLen, inPtr, inLen uint32) uint64 {
	if impl == nil {
		return packEnvelope(nil, errors.New("plugin: no provider registered"))
	}
	op := string(readGuest(opPtr, opLen))
	out, err := impl.Invoke(op, readGuest(inPtr, inLen))
	return packEnvelope(out, err)
}

// ---- 宿主能力封装 ----

// Log 通过宿主记录一条调试日志。
func Log(message string) {
	b := []byte(message)
	if len(b) == 0 {
		return
	}
	hostLog(1, ptrOf(b), uint32(len(b)))
	runtime.KeepAlive(b)
}

// NowMillis 返回当前 Unix 毫秒时间戳。
func NowMillis() int64 { return int64(hostNowMillis()) }

// HTTPDo 通过宿主发起一次受白名单约束的 HTTP 请求。
func HTTPDo(req HTTPRequest) (*HTTPResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	ptr := wasmAlloc(uint32(len(payload)))
	defer wasmDealloc(ptr, uint32(len(payload)))
	copy(unsafeSlice(ptr, uint32(len(payload))), payload)

	packed := hostHTTPRequest(ptr, uint32(len(payload)))
	rptr, rlen := unpack(packed)
	if rlen == 0 {
		return nil, errors.New("plugin: empty http response")
	}
	raw := make([]byte, rlen)
	copy(raw, unsafeSlice(rptr, rlen))
	wasmDealloc(rptr, rlen)

	var resp HTTPResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("plugin: invalid http response: %w", err)
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return &resp, nil
}

// ---- 内部工具 ----

type envelope struct {
	Result []byte `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// packEnvelope 将结果或错误编码为信封并返回给宿主。
func packEnvelope(res []byte, err error) uint64 {
	env := envelope{Result: res}
	if err != nil {
		env = envelope{Error: err.Error()}
	}
	data, mErr := json.Marshal(env)
	if mErr != nil {
		data = []byte(`{"error":"plugin: marshal envelope"}`)
	}
	return packBytes(data)
}

// packBytes 保存结果缓冲区并返回 (指针, 长度) 打包值。
func packBytes(data []byte) uint64 {
	if len(data) == 0 {
		return 0
	}
	result = data
	return uint64(ptrOf(data))<<32 | uint64(uint32(len(data)))
}

// readGuest 从 guest 线性内存复制一段数据。
func readGuest(ptr, length uint32) []byte {
	if ptr == 0 || length == 0 {
		return nil
	}
	out := make([]byte, length)
	copy(out, unsafeSlice(ptr, length))
	return out
}

// ptrOf 返回切片首字节的指针（作为 uint32）。
func ptrOf(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

// unsafeSlice 将 (指针, 长度) 映射为字节切片。
func unsafeSlice(ptr, length uint32) []byte {
	if ptr == 0 || length == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
}

// unpack 拆解 (指针, 长度) 打包值。
func unpack(v uint64) (ptr, length uint32) {
	return uint32(v >> 32), uint32(v)
}
