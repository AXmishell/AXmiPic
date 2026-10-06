// Command smsbao 是一个 AXmiPic 短信渠道插件，对接短信宝 HTTP 网关。
//
// 构建为 WASM reactor 模块：
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .
package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	plugin "github.com/AXmishell/axmipic/sdk/plugin-go"
)

// defaultEndpoint 为短信宝默认发送接口，可在配置中覆盖。
const defaultEndpoint = "https://api.smsbao.com/sms"

type smsbao struct {
	username string
	password string
	sign     string
	endpoint string
}

// Describe 返回插件自描述。
func (s *smsbao) Describe() plugin.Descriptor {
	return plugin.Descriptor{
		Title: "短信宝",
		Fields: []plugin.Field{
			{Key: "username", Label: "账号", Required: true},
			{Key: "password", Label: "密码", Type: plugin.FieldPassword, Required: true, Secret: true},
			{Key: "sign", Label: "短信签名", Help: "可选，将追加在短信内容前，例如 【AXmiPic】"},
			{Key: "endpoint", Label: "接口地址", Default: defaultEndpoint, Help: "一般无需修改，可指向自建网关"},
		},
	}
}

// Configure 保存账号信息。
func (s *smsbao) Configure(config map[string]string) error {
	s.username = strings.TrimSpace(config["username"])
	s.password = config["password"]
	s.sign = strings.TrimSpace(config["sign"])
	s.endpoint = strings.TrimSpace(config["endpoint"])
	if s.endpoint == "" {
		s.endpoint = defaultEndpoint
	}
	if s.username == "" || s.password == "" {
		return fmt.Errorf("短信宝账号与密码不能为空")
	}
	return nil
}

// Invoke 处理 op == "send"。
func (s *smsbao) Invoke(op string, input []byte) ([]byte, error) {
	if op != "send" {
		return nil, fmt.Errorf("短信宝插件不支持操作 %q", op)
	}
	var msg plugin.SMSMessage
	if err := json.Unmarshal(input, &msg); err != nil {
		return nil, fmt.Errorf("无效的消息载荷: %w", err)
	}
	if strings.TrimSpace(msg.To) == "" {
		return nil, fmt.Errorf("接收手机号不能为空")
	}

	content := s.renderContent(msg)
	sum := md5.Sum([]byte(s.password))
	query := url.Values{}
	query.Set("u", s.username)
	query.Set("p", hex.EncodeToString(sum[:]))
	query.Set("m", msg.To)
	query.Set("c", content)

	resp, err := plugin.HTTPDo(plugin.HTTPRequest{
		Method: "GET",
		URL:    s.endpoint + "?" + query.Encode(),
	})
	if err != nil {
		return nil, fmt.Errorf("调用短信宝失败: %w", err)
	}
	body := strings.TrimSpace(string(resp.Body))
	if body != "0" {
		return nil, fmt.Errorf("短信宝返回错误码 %q", body)
	}
	return json.Marshal(map[string]any{"ok": true, "provider": "smsbao"})
}

// renderContent 依据消息生成短信正文，并在需要时追加签名。
func (s *smsbao) renderContent(msg plugin.SMSMessage) string {
	content := strings.TrimSpace(msg.Body)
	if content == "" && msg.Template != "" {
		content = msg.Template
		for key, value := range msg.Params {
			content = strings.ReplaceAll(content, "{{"+key+"}}", value)
		}
	}
	if s.sign != "" && !strings.Contains(content, s.sign) {
		content = s.sign + content
	}
	return content
}

func init() { plugin.Register(&smsbao{}) }

func main() {}
