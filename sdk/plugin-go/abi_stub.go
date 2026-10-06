//go:build !wasip1

package plugin

import "errors"

// ErrUnsupportedHost 表示这些函数只能在 wasip1 目标下调用。
var ErrUnsupportedHost = errors.New("plugin: host capabilities are only available in wasip1 builds")

// Log 在非 wasm 目标下为空操作，便于编辑器与静态检查。
func Log(string) {}

// NowMillis 在非 wasm 目标下返回 0。
func NowMillis() int64 { return 0 }

// HTTPDo 在非 wasm 目标下始终失败。
func HTTPDo(HTTPRequest) (*HTTPResponse, error) { return nil, ErrUnsupportedHost }
