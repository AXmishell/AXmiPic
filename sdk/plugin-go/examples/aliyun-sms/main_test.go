package main

import (
	"strings"
	"testing"
	"time"

	plugin "github.com/AXmishell/axmipic/sdk/plugin-go"
)

func TestTemplateParam(t *testing.T) {
	a := &aliyun{paramKey: "code"}
	got, err := a.templateParam(plugin.SMSMessage{Body: "1234"})
	if err != nil || got != `{"code":"1234"}` {
		t.Fatalf("body fallback = %q, err %v", got, err)
	}
	got, err = a.templateParam(plugin.SMSMessage{Params: map[string]string{"b": "2", "a": "1"}})
	if err != nil || got != `{"a":"1","b":"2"}` {
		t.Fatalf("params = %q, err %v", got, err)
	}
	if _, err := a.templateParam(plugin.SMSMessage{}); err == nil {
		t.Fatal("expected error when neither params nor body present")
	}
}

func TestPercentEncode(t *testing.T) {
	cases := map[string]string{
		"abcXYZ019-_.~":   "abcXYZ019-_.~",
		"a b":             "a%20b",
		"a+b":             "a%2Bb",
		"a*b":             "a%2Ab",
		"a/b":             "a%2Fb",
		"a=b":             "a%3Db",
		"中文":              "%E4%B8%AD%E6%96%87",
		`{"code":"1234"}`: "%7B%22code%22%3A%221234%22%7D",
	}
	for in, want := range cases {
		if got := percentEncode(in); got != want {
			t.Errorf("percentEncode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalQuerySorted(t *testing.T) {
	q := canonicalQuery(map[string]string{"b": "2", "a": "1", "A": "0"})
	// 字节序：A < a < b
	if q != "A=0&a=1&b=2" {
		t.Fatalf("canonicalQuery = %q", q)
	}
}

func TestSignedURLCanonicalAndSignature(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	oldNow, oldNonce := nowFunc, nonceFunc
	nowFunc = func() time.Time { return fixed }
	nonceFunc = func() (string, error) { return "nonce123", nil }
	t.Cleanup(func() { nowFunc, nonceFunc = oldNow, oldNonce })

	a := &aliyun{
		accessKeyID:     "key",
		accessKeySecret: "secret",
		signName:        "AXmiPic",
		templateCode:    "SMS_1",
		region:          "cn-hangzhou",
		endpoint:        defaultEndpoint,
	}
	got, err := a.signedURL("13800000000", `{"code":"1234"}`)
	if err != nil {
		t.Fatalf("signedURL: %v", err)
	}
	for _, want := range []string{
		"AccessKeyId=key",
		"Action=SendSms",
		"SignatureMethod=HMAC-SHA1",
		"SignatureNonce=nonce123",
		"SignatureVersion=1.0",
		"TemplateCode=SMS_1",
		"Timestamp=2026-01-02T03%3A04%3A05Z",
		"Version=2017-05-25",
		"&Signature=",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("signed URL missing %q:\n%s", want, got)
		}
	}
	if !strings.HasPrefix(got, defaultEndpoint+"/?") {
		t.Errorf("unexpected base: %s", got)
	}
}

func TestConfigureValidation(t *testing.T) {
	a := &aliyun{}
	if err := a.Configure(map[string]string{"access_key_id": "x"}); err == nil {
		t.Fatal("expected validation error")
	}
	if err := a.Configure(map[string]string{
		"access_key_id": "id", "access_key_secret": "s", "sign_name": "AXmiPic", "template_code": "SMS_1",
	}); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if a.region != "cn-hangzhou" || a.endpoint != defaultEndpoint {
		t.Fatalf("defaults not applied: %+v", a)
	}
}
