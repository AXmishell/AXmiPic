// Package plugin 提供 AXmiPic 的运行时插件框架。
//
// 插件以独立模块的形式加载：WASM 插件通过 wazero 运行在沙箱内，使用
// -buildmode=c-shared 构建为 reactor（导出 _initialize 与 ABI 函数）；宿主
// 只向插件暴露受控的能力（目前为 HTTP 与日志）。所有插件共享同一套
// category + descriptor 契约，因此通知、支付、存储等领域可以复用同一框架。
//
// 本包不依赖具体业务；调用方（如 notify 适配器）负责把领域接口映射到
// Provider.Invoke。
package plugin

import (
	"context"
	"fmt"
)

// ABI 版本。宿主与插件必须一致。
const ABI = 1

// Category 是插件的领域命名空间，避免不同领域的名称冲突。
type Category = string

const (
	// CategoryNotifySMS 为短信通知渠道。
	CategoryNotifySMS Category = "notify.sms"
	// CategoryNotifyEmail 为邮件通知渠道。
	CategoryNotifyEmail Category = "notify.email"
	// CategoryPayment 为支付渠道。
	CategoryPayment Category = "payment"
	// CategoryStorage 为存储后端。
	CategoryStorage Category = "storage"
	// CategoryModeration 为内容审核。
	CategoryModeration Category = "moderation"
	// CategoryImportX 为 X（Twitter）链接导入的解析插件类别。
	CategoryImportX Category = "import.x"
)

// RuntimeKind 是插件的运行时类型。
type RuntimeKind = string

const (
	// RuntimeWASM 表示外置插件以 WASM 模块运行（wazero 沙箱）。
	RuntimeWASM RuntimeKind = "wasm"
	// RuntimeBuiltin 表示内置插件直接编译进宿主，以进程内 Provider 运行。
	// 内置插件由宿主签名/发布，默认受信，不经过沙箱，也不从磁盘加载。
	RuntimeBuiltin RuntimeKind = "builtin"
)

// Origin 表示插件的来源与信任级别。
type Origin = string

const (
	// OriginBuiltin 为随宿主编译发布的内置插件：默认信任、进程内运行、无沙箱。
	OriginBuiltin Origin = "builtin"
	// OriginExternal 为从 plugins.dir 安装的外置插件：必须运行在 WASM 沙箱中。
	OriginExternal Origin = "external"
)

// FieldType 是插件配置字段的输入类型。
type FieldType = string

const (
	FieldString   FieldType = "string"
	FieldPassword FieldType = "password"
	FieldInt      FieldType = "int"
	FieldBool     FieldType = "bool"
	FieldSelect   FieldType = "select"
)

// FieldVisibility 描述字段的显示条件：当 Key 对应字段的值属于 Values 时显示。
// 为空表示始终显示。前端仅用它控制表单可见性，不影响配置存储。
type FieldVisibility struct {
	Key    string   `json:"key" yaml:"key"`
	Values []string `json:"values,omitempty" yaml:"values,omitempty"`
}

// Field 描述一个插件配置字段，用于驱动后台动态表单。
type Field struct {
	Key      string    `json:"key" yaml:"key"`
	Label    string    `json:"label" yaml:"label"`
	Type     FieldType `json:"type,omitempty" yaml:"type,omitempty"`
	Required bool      `json:"required,omitempty" yaml:"required,omitempty"`
	// Secret 为 true 时，字段值在后台以密文存储且不回显。
	Secret  bool     `json:"secret,omitempty" yaml:"secret,omitempty"`
	Default string   `json:"default,omitempty" yaml:"default,omitempty"`
	Options []string `json:"options,omitempty" yaml:"options,omitempty"`
	Help    string   `json:"help,omitempty" yaml:"help,omitempty"`
	// VisibleWhen 可选，用于按另一个字段的值控制该字段是否显示。
	VisibleWhen *FieldVisibility `json:"visible_when,omitempty" yaml:"visible_when,omitempty"`
}

// Descriptor 是插件对自身的描述，由插件在加载时返回。
type Descriptor struct {
	Category string  `json:"category"`
	Name     string  `json:"name"`
	Title    string  `json:"title,omitempty"`
	Version  string  `json:"version,omitempty"`
	Fields   []Field `json:"fields,omitempty"`
	// Origin 由宿主填充，插件自描述中的值会被忽略。
	Origin Origin `json:"origin,omitempty"`
	// Sandboxed 为 true 表示运行在 WASM 沙箱中；内置插件为 false。
	Sandboxed bool `json:"sandboxed,omitempty"`
}

// Provider 是一个已加载并可调用的插件实例。
type Provider interface {
	// Descriptor 返回插件自描述。
	Descriptor() Descriptor
	// Configure 注入解密后的配置值。宿主负责在静态存储时加密 Secret 字段。
	Configure(ctx context.Context, config map[string]string) error
	// Invoke 执行一次操作，op 由各领域约定（如通知渠道使用 "send"）。
	Invoke(ctx context.Context, op string, input []byte) ([]byte, error)
	// Close 释放插件占用的资源。
	Close(ctx context.Context) error
}

// ErrNotFound 在按名称查找插件失败时返回。
var ErrNotFound = fmt.Errorf("plugin: not found")

// ErrCapabilityDenied 在插件请求了未声明或被拒绝的能力时返回。
var ErrCapabilityDenied = fmt.Errorf("plugin: capability denied")
