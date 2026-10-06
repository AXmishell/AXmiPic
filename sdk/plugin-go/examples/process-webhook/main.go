// Command process-webhook 是一个 AXmiPic 进程外（process）通知插件示例。
//
// 它把通知以 JSON POST 转发到配置的 Webhook 地址，演示进程插件如何直接使用
// Go 标准库（net/http）——这类插件不受 WASM 沙箱限制，可承载官方云 SDK 等重型
// 依赖。
//
// 构建（宿主架构，非 wasm）：
//
//	go build -o plugin-bin .
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	plugin "github.com/AXmishell/axmipic/sdk/plugin-go"
	"github.com/AXmishell/axmipic/sdk/plugin-go/process"
)

type webhook struct {
	url     string
	token   string
	timeout time.Duration
	client  *http.Client
}

// Describe 返回插件自描述。
func (w *webhook) Describe() plugin.Descriptor {
	return plugin.Descriptor{
		Title: "Webhook 通知（进程插件）",
		Fields: []plugin.Field{
			{Key: "url", Label: "Webhook 地址", Required: true, Help: "接收 JSON POST 的地址"},
			{Key: "token", Label: "鉴权 Token", Type: plugin.FieldPassword, Secret: true, Help: "可选，作为 Authorization: Bearer 发送"},
			{Key: "timeout_sec", Label: "超时（秒）", Default: "10"},
		},
	}
}

// Configure 保存配置。
func (w *webhook) Configure(config map[string]string) error {
	w.url = strings.TrimSpace(config["url"])
	w.token = config["token"]
	if w.url == "" {
		return fmt.Errorf("webhook 地址不能为空")
	}
	seconds := 10
	if v := strings.TrimSpace(config["timeout_sec"]); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &seconds); err != nil || seconds <= 0 {
			seconds = 10
		}
	}
	w.timeout = time.Duration(seconds) * time.Second
	w.client = &http.Client{Timeout: w.timeout}
	return nil
}

// Invoke 处理 op == "send"。
func (w *webhook) Invoke(op string, input []byte) ([]byte, error) {
	if op != "send" {
		return nil, fmt.Errorf("webhook 插件不支持操作 %q", op)
	}
	var msg plugin.SMSMessage
	if err := json.Unmarshal(input, &msg); err != nil {
		return nil, fmt.Errorf("无效的消息载荷: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"to":        msg.To,
		"subject":   msg.Subject,
		"body":      msg.Body,
		"sign_name": msg.SignName,
		"params":    msg.Params,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if w.token != "" {
		req.Header.Set("Authorization", "Bearer "+w.token)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 webhook 失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("webhook 返回 %d: %s", resp.StatusCode, string(raw))
	}
	return json.Marshal(map[string]any{"ok": true, "provider": "process-webhook", "status": resp.StatusCode})
}

func main() { process.Main(&webhook{}) }
