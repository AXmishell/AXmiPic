// Package plugin 是 AXmiPic WASM 插件的 Go 编写 SDK。
//
// 用法：在插件中实现 Provider，并在 init 中调用 Register；随后使用
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .
//
// 构建为 reactor 模块。宿主 AXmiPic 通过其 internal/plugin 包加载该模块。
//
// 注意：使用本 SDK 的插件必须定义 package main 与空的 func main()，且必须从
// main 包（直接或间接）导入本包，以保证 ABI 导出函数被链接。
package plugin

// FieldType 是配置字段的输入类型。
type FieldType = string

const (
	FieldString   FieldType = "string"
	FieldPassword FieldType = "password"
	FieldInt      FieldType = "int"
	FieldBool     FieldType = "bool"
	FieldSelect   FieldType = "select"
)

// Field 描述一个插件配置字段。
type Field struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type,omitempty"`
	Required bool      `json:"required,omitempty"`
	// Secret 为 true 时宿主会加密存储该字段且不回显。
	Secret  bool     `json:"secret,omitempty"`
	Default string   `json:"default,omitempty"`
	Options []string `json:"options,omitempty"`
	Help    string   `json:"help,omitempty"`
}

// Descriptor 是插件对自身的描述。category 与 name 由宿主清单指定，插件可留空。
type Descriptor struct {
	Category string  `json:"category"`
	Name     string  `json:"name"`
	Title    string  `json:"title,omitempty"`
	Version  string  `json:"version,omitempty"`
	Fields   []Field `json:"fields,omitempty"`
}

// Provider 由插件实现。
type Provider interface {
	// Describe 返回插件自描述（通常只需填写 Title 与 Fields）。
	Describe() Descriptor
	// Configure 接收宿主注入的配置值。
	Configure(config map[string]string) error
	// Invoke 执行一次操作。约定：通知渠道使用 op == "send"。
	Invoke(op string, input []byte) ([]byte, error)
}

// HTTPRequest 是插件发起的一次宿主代理 HTTP 请求。
type HTTPRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
}

// HTTPResponse 是宿主返回的 HTTP 响应。
type HTTPResponse struct {
	Status  int               `json:"status,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// SMSMessage 是通知短信渠道的 Invoke("send", ...) 约定载荷。
type SMSMessage struct {
	To       string            `json:"to"`
	Subject  string            `json:"subject,omitempty"`
	Body     string            `json:"body,omitempty"`
	Template string            `json:"template,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
	SignName string            `json:"sign_name,omitempty"`
}

var impl Provider

// Register 注册插件实现。应在 init 中调用。
func Register(p Provider) { impl = p }
