// Command tencent-sms 是一个 AXmiPic 短信渠道插件，对接腾讯云短信服务。
//
// 使用 TC3-HMAC-SHA256 签名 + JSON POST，全部在插件内用 Go 标准库完成。
//
// 构建为 WASM reactor 模块：
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	plugin "github.com/AXmishell/axmipic/sdk/plugin-go"
)

const (
	defaultEndpoint = "https://sms.tencentcloudapi.com"
	apiVersion      = "2021-01-11"
	apiAction       = "SendSms"
	apiService      = "sms"
	contentType     = "application/json; charset=utf-8"
)

// nowFunc 可在测试中替换以获得确定性时间戳。
var nowFunc = func() time.Time { return time.Now() }

type tencent struct {
	secretID    string
	secretKey   string
	smsAppID    string
	signName    string
	templateID  string
	region      string
	countryCode string
	endpoint    string
}

// Describe 返回插件自描述。
func (t *tencent) Describe() plugin.Descriptor {
	return plugin.Descriptor{
		Title: "腾讯云短信",
		Fields: []plugin.Field{
			{Key: "secret_id", Label: "SecretId", Required: true, Secret: true},
			{Key: "secret_key", Label: "SecretKey", Type: plugin.FieldPassword, Required: true, Secret: true},
			{Key: "sms_sdk_app_id", Label: "短信应用 SdkAppId", Required: true, Help: "如 1400000000"},
			{Key: "sign_name", Label: "短信签名", Required: true, Help: "如：AXmiPic"},
			{Key: "template_id", Label: "模板 ID", Required: true, Help: "如 1234567"},
			{Key: "region", Label: "地域", Default: "ap-guangzhou", Help: "如 ap-guangzhou"},
			{Key: "country_code", Label: "国家码", Default: "86", Help: "不含 + 号；已带 + 的号码将原样使用"},
			{Key: "endpoint", Label: "接口地址", Default: defaultEndpoint, Help: "一般无需修改"},
		},
	}
}

// Configure 保存并校验配置。
func (t *tencent) Configure(config map[string]string) error {
	t.secretID = strings.TrimSpace(config["secret_id"])
	t.secretKey = config["secret_key"]
	t.smsAppID = strings.TrimSpace(config["sms_sdk_app_id"])
	t.signName = strings.TrimSpace(config["sign_name"])
	t.templateID = strings.TrimSpace(config["template_id"])
	t.region = strings.TrimSpace(config["region"])
	t.countryCode = strings.TrimSpace(config["country_code"])
	t.endpoint = strings.TrimSpace(config["endpoint"])
	if t.region == "" {
		t.region = "ap-guangzhou"
	}
	if t.countryCode == "" {
		t.countryCode = "86"
	}
	if t.endpoint == "" {
		t.endpoint = defaultEndpoint
	}
	if t.secretID == "" || t.secretKey == "" || t.smsAppID == "" || t.signName == "" || t.templateID == "" {
		return fmt.Errorf("腾讯云短信的 SecretId、SecretKey、SdkAppId、签名与模板 ID 均不能为空")
	}
	return nil
}

// Invoke 处理 op == "send"。
func (t *tencent) Invoke(op string, input []byte) ([]byte, error) {
	if op != "send" {
		return nil, fmt.Errorf("腾讯云短信插件不支持操作 %q", op)
	}
	var msg plugin.SMSMessage
	if err := json.Unmarshal(input, &msg); err != nil {
		return nil, fmt.Errorf("无效的消息载荷: %w", err)
	}
	if strings.TrimSpace(msg.To) == "" {
		return nil, fmt.Errorf("接收手机号不能为空")
	}

	params := t.templateParams(msg)
	payload, err := json.Marshal(map[string]any{
		"PhoneNumberSet":   []string{t.formatPhone(msg.To)},
		"SmsSdkAppId":      t.smsAppID,
		"SignName":         t.signName,
		"TemplateId":       t.templateID,
		"TemplateParamSet": params,
	})
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}

	headers, err := t.signHeaders(payload)
	if err != nil {
		return nil, err
	}
	resp, err := plugin.HTTPDo(plugin.HTTPRequest{
		Method:  "POST",
		URL:     strings.TrimRight(t.endpoint, "/"),
		Headers: headers,
		Body:    payload,
	})
	if err != nil {
		return nil, fmt.Errorf("调用腾讯云短信失败: %w", err)
	}

	var parsed struct {
		Response struct {
			SendStatusSet []struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"SendStatusSet"`
			RequestID string `json:"RequestId"`
			Error     *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, fmt.Errorf("解析腾讯云响应失败: %w (原始: %s)", err, string(resp.Body))
	}
	if parsed.Response.Error != nil {
		return nil, fmt.Errorf("腾讯云返回错误 %s: %s", parsed.Response.Error.Code, parsed.Response.Error.Message)
	}
	if len(parsed.Response.SendStatusSet) == 0 || parsed.Response.SendStatusSet[0].Code != "Ok" {
		return nil, fmt.Errorf("腾讯云短信未成功: %s", string(resp.Body))
	}
	return json.Marshal(map[string]any{"ok": true, "provider": "tencent", "request_id": parsed.Response.RequestID})
}

// signHeaders 依据 TC3-HMAC-SHA256 构造请求头。
func (t *tencent) signHeaders(payload []byte) (map[string]string, error) {
	u, err := url.Parse(t.endpoint)
	if err != nil {
		return nil, fmt.Errorf("无效的接口地址: %w", err)
	}
	host := strings.ToLower(u.Host)

	ts := nowFunc().Unix()
	date := time.Unix(ts, 0).UTC().Format("2006-01-02")

	canonicalHeaders := "content-type:" + contentType + "\n" +
		"host:" + host + "\n" +
		"x-tc-action:" + strings.ToLower(apiAction) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + sha256Hex(payload)

	credentialScope := date + "/" + apiService + "/tc3_request"
	stringToSign := "TC3-HMAC-SHA256\n" +
		fmt.Sprintf("%d", ts) + "\n" +
		credentialScope + "\n" +
		sha256Hex([]byte(canonicalRequest))

	secretDate := hmacSHA256([]byte("TC3"+t.secretKey), date)
	secretService := hmacSHA256(secretDate, apiService)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	authorization := "TC3-HMAC-SHA256 Credential=" + t.secretID + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature

	return map[string]string{
		"Authorization":  authorization,
		"Content-Type":   contentType,
		"Host":           u.Host,
		"X-TC-Action":    apiAction,
		"X-TC-Timestamp": fmt.Sprintf("%d", ts),
		"X-TC-Version":   apiVersion,
		"X-TC-Region":    t.region,
	}, nil
}

// formatPhone 将号码格式化为 E.164（默认加国家码前缀 +）。
func (t *tencent) formatPhone(to string) string {
	to = strings.TrimSpace(to)
	if strings.HasPrefix(to, "+") {
		return to
	}
	cc := strings.TrimPrefix(strings.TrimSpace(t.countryCode), "+")
	if cc == "" {
		cc = "86"
	}
	return "+" + cc + to
}

// templateParams 返回模板参数：显式 params 按 key 排序；否则用正文作为单一参数。
func (t *tencent) templateParams(msg plugin.SMSMessage) []string {
	params := sortedValues(msg.Params)
	if len(params) == 0 && strings.TrimSpace(msg.Body) != "" {
		params = []string{msg.Body}
	}
	return params
}

// sortedValues 按 key 排序后返回模板参数值，保证顺序稳定。
func sortedValues(params map[string]string) []string {
	if len(params) == 0 {
		return nil
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, params[k])
	}
	return values
}

// sha256Hex 返回字符串的 sha256 十六进制摘要。
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// hmacSHA256 返回 HMAC-SHA256 摘要（原始字节）。
func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func init() { plugin.Register(&tencent{}) }

func main() {}
