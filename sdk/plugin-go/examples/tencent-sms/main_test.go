package main

import (
	"strings"
	"testing"
	"time"

	plugin "github.com/AXmishell/axmipic/sdk/plugin-go"
)

func TestTemplateParams(t *testing.T) {
	tr := &tencent{}
	if got := tr.templateParams(plugin.SMSMessage{Body: "1234"}); len(got) != 1 || got[0] != "1234" {
		t.Fatalf("body fallback = %v", got)
	}
	if got := tr.templateParams(plugin.SMSMessage{Params: map[string]string{"b": "2", "a": "1"}}); len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("params = %v", got)
	}
	if got := tr.templateParams(plugin.SMSMessage{}); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestFormatPhone(t *testing.T) {
	tr := &tencent{countryCode: "86"}
	if got := tr.formatPhone("13800000000"); got != "+8613800000000" {
		t.Errorf("formatPhone = %q", got)
	}
	if got := tr.formatPhone("+14155550100"); got != "+14155550100" {
		t.Errorf("should keep E.164: %q", got)
	}
	tr.countryCode = ""
	if got := tr.formatPhone("13800000000"); got != "+8613800000000" {
		t.Errorf("default cc = %q", got)
	}
}

func TestSortedValues(t *testing.T) {
	vals := sortedValues(map[string]string{"b": "2", "a": "1", "c": "3"})
	want := []string{"1", "2", "3"}
	if len(vals) != 3 {
		t.Fatalf("vals = %v", vals)
	}
	for i := range want {
		if vals[i] != want[i] {
			t.Fatalf("vals = %v, want %v", vals, want)
		}
	}
	if sortedValues(nil) != nil {
		t.Error("nil params should yield nil")
	}
}

func TestSignHeaders(t *testing.T) {
	old := nowFunc
	nowFunc = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	t.Cleanup(func() { nowFunc = old })

	tr := &tencent{
		secretID:   "AKID",
		secretKey:  "SECRET",
		smsAppID:   "1400",
		signName:   "AXmiPic",
		templateID: "123",
		region:     "ap-guangzhou",
		endpoint:   defaultEndpoint,
	}
	payload := []byte(`{"PhoneNumberSet":["+8613800000000"]}`)
	headers, err := tr.signHeaders(payload)
	if err != nil {
		t.Fatalf("signHeaders: %v", err)
	}

	auth := headers["Authorization"]
	for _, want := range []string{
		"TC3-HMAC-SHA256 Credential=AKID/2023-11-14/sms/tc3_request",
		"SignedHeaders=content-type;host;x-tc-action",
		"Signature=",
	} {
		if !strings.Contains(auth, want) {
			t.Errorf("authorization missing %q:\n%s", want, auth)
		}
	}
	if headers["Host"] != "sms.tencentcloudapi.com" {
		t.Errorf("host = %q", headers["Host"])
	}
	if headers["X-TC-Timestamp"] != "1700000000" {
		t.Errorf("timestamp = %q", headers["X-TC-Timestamp"])
	}

	// 不同载荷应产生不同签名。
	other, err := tr.signHeaders([]byte(`{"PhoneNumberSet":["+8613900000000"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if other["Authorization"] == auth {
		t.Error("signature should change with payload")
	}
}

func TestConfigureValidation(t *testing.T) {
	tr := &tencent{}
	if err := tr.Configure(map[string]string{"secret_id": "x"}); err == nil {
		t.Fatal("expected validation error")
	}
	if err := tr.Configure(map[string]string{
		"secret_id": "id", "secret_key": "key", "sms_sdk_app_id": "1400",
		"sign_name": "AXmiPic", "template_id": "123",
	}); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if tr.region != "ap-guangzhou" || tr.countryCode != "86" || tr.endpoint != defaultEndpoint {
		t.Fatalf("defaults not applied: %+v", tr)
	}
}
