package imaging

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Driver 是处理器的注册名。通过构建标签与运行时配置选择具体实现。
type Driver string

// 内置驱动名称。
const (
	// DriverPureGo 是纯 Go 处理器（默认，无外部依赖）。
	DriverPureGo Driver = "purego"
	// DriverLibVips 是 libvips/bimg 处理器（需 -tags libvips 与 CGO）。
	DriverLibVips Driver = "libvips"
	// DriverMagick 是基于 ImageMagick 命令行的处理器。
	DriverMagick Driver = "magick"
)

// factory 构造一个处理器。返回错误时该驱动不可用。
type factory func() (Processor, error)

var (
	registryMu sync.RWMutex
	registry   = map[Driver]factory{}
)

// register 注册一个处理器工厂，供 Resolve 按名称选择。
func register(driver Driver, f factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[driver] = f
}

// AvailableDrivers 返回当前二进制可用的驱动名称（已编译且能初始化）。
func AvailableDrivers() []Driver {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]Driver, 0, len(registry))
	for name, f := range registry {
		if _, err := f(); err == nil {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

// Resolve 按名称返回处理器。名称为空时返回默认处理器。
func Resolve(name string) (Processor, error) {
	trimmed := Driver(strings.ToLower(strings.TrimSpace(name)))
	if trimmed == "" {
		return Default(), nil
	}
	registryMu.RLock()
	f, ok := registry[trimmed]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("imaging: driver %q is not available in this build", name)
	}
	return f()
}

// ResolveWithFallback 尝试按名称解析处理器；不可用时回退到默认处理器，
// 并返回实际使用的处理器名称。
func ResolveWithFallback(name string) (Processor, string, error) {
	processor, err := Resolve(name)
	if err == nil {
		return processor, processor.Capabilities().Name, nil
	}
	fallback := Default()
	return fallback, fallback.Capabilities().Name, nil
}
