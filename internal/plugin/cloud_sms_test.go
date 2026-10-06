package plugin

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const allowCloudLocalManifest = `
name: %s
version: 0.1.0
category: notify.sms
runtime: wasm
entry: plugin.wasm
abi: 1
capabilities:
  http:
    hosts: ["127.0.0.1"]
`

// ---- 阿里云 ----

func TestAliyunSMSEndToEnd(t *testing.T) {
	requireModule(t, "aliyun-sms")

	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		_, _ = w.Write([]byte(`{"Code":"OK","Message":"OK","RequestId":"req-1","BizId":"biz-1"}`))
	}))
	defer srv.Close()

	manifest := strings.Replace(allowCloudLocalManifest, "%s", "aliyun-sms", 1)
	dir := installModule(t, "aliyun-sms", manifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}

	const secret = "test-secret"
	if err := mgr.Configure(ctx, "aliyun-sms", map[string]string{
		"access_key_id":     "test-key",
		"access_key_secret": secret,
		"sign_name":         "AXmiPic",
		"template_code":     "SMS_1",
		"region":            "cn-hangzhou",
		"endpoint":          srv.URL,
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}

	input, _ := json.Marshal(map[string]any{"to": "13800000000", "params": map[string]string{"code": "1234"}})
	out, err := mgr.Invoke(ctx, CategoryNotifySMS, "aliyun-sms", "send", input)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(string(out), `"ok":true`) {
		t.Fatalf("unexpected result: %s", out)
	}
	if captured == nil {
		t.Fatal("server did not receive a request")
	}

	q := captured.URL.Query()
	for key, want := range map[string]string{
		"Action":        "SendSms",
		"PhoneNumbers":  "13800000000",
		"SignName":      "AXmiPic",
		"TemplateCode":  "SMS_1",
		"TemplateParam": `{"code":"1234"}`,
	} {
		if got := q.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	verifyAliyunSignature(t, captured, secret)
}

// verifyAliyunSignature 用独立实现复算签名，验证插件签名正确。
func verifyAliyunSignature(t *testing.T, r *http.Request, secret string) {
	t.Helper()
	q := r.URL.Query()
	got := q.Get("Signature")
	if got == "" {
		t.Fatal("missing Signature")
	}
	params := map[string]string{}
	for k := range q {
		if k == "Signature" {
			continue
		}
		params[k] = q.Get(k)
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, aliyunPE(k)+"="+aliyunPE(params[k]))
	}
	canonical := strings.Join(parts, "&")
	stringToSign := "GET&" + aliyunPE("/") + "&" + aliyunPE(canonical)
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("signature mismatch:\n got %s\nwant %s", got, want)
	}
}

// aliyunPE 是测试侧的独立百分号编码实现。
func aliyunPE(s string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexDigits[c>>4])
		b.WriteByte(hexDigits[c&0x0F])
	}
	return b.String()
}

// ---- 腾讯云 ----

func TestTencentSMSEndToEnd(t *testing.T) {
	requireModule(t, "tencent-sms")

	var body []byte
	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = readAll(r)
		headers = r.Header.Clone()
		_, _ = w.Write([]byte(`{"Response":{"SendStatusSet":[{"Code":"Ok","Message":"send success"}],"RequestId":"req-2"}}`))
	}))
	defer srv.Close()

	manifest := strings.Replace(allowCloudLocalManifest, "%s", "tencent-sms", 1)
	dir := installModule(t, "tencent-sms", manifest)
	mgr := newTestManager(t, dir)
	ctx := context.Background()
	if err := mgr.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}

	const secretKey = "test-secret-key"
	if err := mgr.Configure(ctx, "tencent-sms", map[string]string{
		"secret_id":      "AKIDtest",
		"secret_key":     secretKey,
		"sms_sdk_app_id": "1400000000",
		"sign_name":      "AXmiPic",
		"template_id":    "123456",
		"region":         "ap-guangzhou",
		"country_code":   "86",
		"endpoint":       srv.URL,
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}

	input, _ := json.Marshal(map[string]any{"to": "13800000000", "params": map[string]string{"code": "1234"}})
	if _, err := mgr.Invoke(ctx, CategoryNotifySMS, "tencent-sms", "send", input); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	var payload struct {
		PhoneNumberSet   []string `json:"PhoneNumberSet"`
		SmsSdkAppId      string   `json:"SmsSdkAppId"`
		SignName         string   `json:"SignName"`
		TemplateId       string   `json:"TemplateId"`
		TemplateParamSet []string `json:"TemplateParamSet"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("payload decode: %v (%s)", err, body)
	}
	if len(payload.PhoneNumberSet) != 1 || payload.PhoneNumberSet[0] != "+8613800000000" {
		t.Errorf("phone = %v", payload.PhoneNumberSet)
	}
	if payload.SmsSdkAppId != "1400000000" || payload.SignName != "AXmiPic" || payload.TemplateId != "123456" {
		t.Errorf("unexpected payload: %s", body)
	}
	if len(payload.TemplateParamSet) != 1 || payload.TemplateParamSet[0] != "1234" {
		t.Errorf("template params = %v", payload.TemplateParamSet)
	}
	verifyTencentSignature(t, headers, srv.URL, body, secretKey)
}

// verifyTencentSignature 用独立实现复算 TC3-HMAC-SHA256 签名。
func verifyTencentSignature(t *testing.T, headers http.Header, endpoint string, body []byte, secretKey string) {
	t.Helper()
	auth := headers.Get("Authorization")
	if !strings.HasPrefix(auth, "TC3-HMAC-SHA256 ") {
		t.Fatalf("bad authorization: %q", auth)
	}
	ts := headers.Get("X-TC-Timestamp")
	if ts == "" {
		t.Fatal("missing X-TC-Timestamp")
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		t.Fatalf("timestamp: %v", err)
	}
	date := time.Unix(sec, 0).UTC().Format("2006-01-02")
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	action := strings.ToLower(headers.Get("X-TC-Action"))
	ct := headers.Get("Content-Type")

	canonicalHeaders := "content-type:" + ct + "\n" + "host:" + host + "\n" + "x-tc-action:" + action + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + sha256HexBytes(body)
	credentialScope := date + "/sms/tc3_request"
	stringToSign := "TC3-HMAC-SHA256\n" + ts + "\n" + credentialScope + "\n" + sha256HexBytes([]byte(canonicalRequest))

	secretDate := hmacSHA256Bytes([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256Bytes(secretDate, "sms")
	secretSigning := hmacSHA256Bytes(secretService, "tc3_request")
	want := hex.EncodeToString(hmacSHA256Bytes(secretSigning, stringToSign))

	idx := strings.Index(auth, "Signature=")
	if idx < 0 {
		t.Fatalf("no signature in %q", auth)
	}
	got := auth[idx+len("Signature="):]
	if got != want {
		t.Fatalf("signature mismatch:\n got %s\nwant %s", got, want)
	}
}

func sha256HexBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256Bytes(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func readAll(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	return io.ReadAll(r.Body)
}
