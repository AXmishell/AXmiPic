// Command aliyun-sms 是一个 AXmiPic 短信渠道插件，对接阿里云短信服务（dysmsapi）。
//
// 使用 RPC 风格 GET 请求 + HMAC-SHA1 签名，全部在插件内用 Go 标准库完成。
//
// 构建为 WASM reactor 模块：
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .
package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	plugin "github.com/AXmishell/axmipic/sdk/plugin-go"
)

const (
	defaultEndpoint = "https://dysmsapi.aliyuncs.com"
	apiVersion      = "2017-05-25"
	apiAction       = "SendSms"
	apiFormat       = "JSON"
	signMethod      = "HMAC-SHA1"
	signVersion     = "1.0"
)

// nowFunc 与 nonceFunc 可在测试中替换以获得确定性。
var (
	nowFunc   = func() time.Time { return time.Now() }
	nonceFunc = randomNonce
)

type aliyun struct {
	accessKeyID     string
	accessKeySecret string
	signName        string
	templateCode    string
	region          string
	endpoint        string
	// paramKey 是仅提供正文时使用的模板参数名（常见为 "code"）。
	paramKey string
}

// Describe 返回插件自描述。
func (a *aliyun) Describe() plugin.Descriptor {
	return plugin.Descriptor{
		Title: "阿里云短信",
		Fields: []plugin.Field{
			{Key: "access_key_id", Label: "AccessKeyId", Required: true, Secret: true},
			{Key: "access_key_secret", Label: "AccessKeySecret", Type: plugin.FieldPassword, Required: true, Secret: true},
			{Key: "sign_name", Label: "短信签名", Required: true, Help: "如：AXmiPic"},
			{Key: "template_code", Label: "模板 Code", Required: true, Help: "如：SMS_123456789"},
			{Key: "param_key", Label: "验证码参数名", Default: "code", Help: "仅提供正文时使用的模板参数名"},
			{Key: "region", Label: "地域", Default: "cn-hangzhou", Help: "如 cn-hangzhou"},
			{Key: "endpoint", Label: "接口地址", Default: defaultEndpoint, Help: "一般无需修改"},
		},
	}
}

// Configure 保存并校验配置。
func (a *aliyun) Configure(config map[string]string) error {
	a.accessKeyID = strings.TrimSpace(config["access_key_id"])
	a.accessKeySecret = config["access_key_secret"]
	a.signName = strings.TrimSpace(config["sign_name"])
	a.templateCode = strings.TrimSpace(config["template_code"])
	a.region = strings.TrimSpace(config["region"])
	a.endpoint = strings.TrimSpace(config["endpoint"])
	a.paramKey = strings.TrimSpace(config["param_key"])
	if a.region == "" {
		a.region = "cn-hangzhou"
	}
	if a.paramKey == "" {
		a.paramKey = "code"
	}
	if a.endpoint == "" {
		a.endpoint = defaultEndpoint
	}
	if a.accessKeyID == "" || a.accessKeySecret == "" || a.signName == "" || a.templateCode == "" {
		return fmt.Errorf("阿里云短信的 AccessKey、签名与模板 Code 均不能为空")
	}
	return nil
}

// templateParam 返回模板参数 JSON：优先使用显式 params，否则用正文作为单一参数。
func (a *aliyun) templateParam(msg plugin.SMSMessage) (string, error) {
	if len(msg.Params) > 0 {
		data, err := json.Marshal(msg.Params)
		if err != nil {
			return "", fmt.Errorf("模板参数序列化失败: %w", err)
		}
		return string(data), nil
	}
	if strings.TrimSpace(msg.Body) != "" {
		data, err := json.Marshal(map[string]string{a.paramKey: msg.Body})
		if err != nil {
			return "", fmt.Errorf("模板参数序列化失败: %w", err)
		}
		return string(data), nil
	}
	return "", fmt.Errorf("阿里云短信需要模板参数 params 或正文 body")
}

// Invoke 处理 op == "send"。
func (a *aliyun) Invoke(op string, input []byte) ([]byte, error) {
	if op != "send" {
		return nil, fmt.Errorf("阿里云短信插件不支持操作 %q", op)
	}
	var msg plugin.SMSMessage
	if err := json.Unmarshal(input, &msg); err != nil {
		return nil, fmt.Errorf("无效的消息载荷: %w", err)
	}
	if strings.TrimSpace(msg.To) == "" {
		return nil, fmt.Errorf("接收手机号不能为空")
	}
	if len(msg.Params) == 0 && strings.TrimSpace(msg.Body) == "" {
		return nil, fmt.Errorf("阿里云短信需要模板参数 params 或正文 body")
	}
	templateParam, err := a.templateParam(msg)
	if err != nil {
		return nil, err
	}

	requestURL, err := a.signedURL(msg.To, templateParam)
	if err != nil {
		return nil, err
	}
	resp, err := plugin.HTTPDo(plugin.HTTPRequest{Method: "GET", URL: requestURL})
	if err != nil {
		return nil, fmt.Errorf("调用阿里云短信失败: %w", err)
	}
	var parsed struct {
		Code      string `json:"Code"`
		Message   string `json:"Message"`
		RequestID string `json:"RequestId"`
	}
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, fmt.Errorf("解析阿里云响应失败: %w (原始: %s)", err, string(resp.Body))
	}
	if parsed.Code != "OK" {
		return nil, fmt.Errorf("阿里云返回错误 %s: %s", parsed.Code, parsed.Message)
	}
	return json.Marshal(map[string]any{"ok": true, "provider": "aliyun", "request_id": parsed.RequestID})
}

// signedURL 构造带签名的 GET 请求地址。
func (a *aliyun) signedURL(phone, templateParam string) (string, error) {
	nonce, err := nonceFunc()
	if err != nil {
		return "", err
	}
	params := map[string]string{
		"AccessKeyId":      a.accessKeyID,
		"Action":           apiAction,
		"Format":           apiFormat,
		"PhoneNumbers":     phone,
		"RegionId":         a.region,
		"SignName":         a.signName,
		"SignatureMethod":  signMethod,
		"SignatureNonce":   nonce,
		"SignatureVersion": signVersion,
		"TemplateCode":     a.templateCode,
		"TemplateParam":    templateParam,
		"Timestamp":        nowFunc().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          apiVersion,
	}
	canonical := canonicalQuery(params)
	stringToSign := "GET&" + percentEncode("/") + "&" + percentEncode(canonical)

	mac := hmac.New(sha1.New, []byte(a.accessKeySecret+"&"))
	mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return strings.TrimRight(a.endpoint, "/") + "/?" + canonical + "&Signature=" + percentEncode(signature), nil
}

// canonicalQuery 按键排序并 encode 后拼接查询串。
func canonicalQuery(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, percentEncode(k)+"="+percentEncode(params[k]))
	}
	return strings.Join(parts, "&")
}

// percentEncode 按 RFC3986 对非保留字符进行百分号编码（阿里云要求）。
func percentEncode(s string) string {
	const upperhex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(upperhex[c>>4])
			b.WriteByte(upperhex[c&0x0F])
		}
	}
	return b.String()
}

// randomNonce 生成随机签名随机数。
func randomNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func init() { plugin.Register(&aliyun{}) }

func main() {}
