package payment

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// generateKeyPair 生成一对 PEM 编码的 RSA 密钥，用于测试签名与验签。
func generateKeyPair(t *testing.T) (privatePEM, publicPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	privDER := x509.MarshalPKCS1PrivateKey(key)
	privatePEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER}))
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	publicPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	return privatePEM, publicPEM
}

func TestAlipaySignAndVerifyCallback(t *testing.T) {
	privatePEM, publicPEM := generateKeyPair(t)
	gateway, err := NewAlipayGateway(AlipayOptions{
		AppID:      "2021000000000000",
		PrivateKey: privatePEM,
		PublicKey:  publicPEM,
	})
	if err != nil {
		t.Fatalf("NewAlipayGateway: %v", err)
	}
	if gateway.Name() != "alipay" {
		t.Fatalf("name = %q", gateway.Name())
	}

	// 用私钥对回调参数签名，验证 VerifyCallback 能通过公钥校验。
	params := url.Values{}
	params.Set("app_id", "2021000000000000")
	params.Set("out_trade_no", "order-1")
	params.Set("trade_no", "trade-1")
	params.Set("trade_status", "TRADE_SUCCESS")
	params.Set("total_amount", "10.00")
	params.Set("gmt_payment", "2026-01-02 15:04:05")
	signature, err := signAlipay(params, gateway.privateKey)
	if err != nil {
		t.Fatalf("signAlipay: %v", err)
	}
	params.Set("sign", signature)
	params.Set("sign_type", "RSA2")

	callback, err := gateway.VerifyCallback(context.Background(), http.Header{}, []byte(params.Encode()))
	if err != nil {
		t.Fatalf("VerifyCallback: %v", err)
	}
	if callback.OrderID != "order-1" || callback.TradeNo != "trade-1" || !callback.Success {
		t.Fatalf("callback = %+v", callback)
	}
	if !callback.HasAmount || callback.AmountCents != 1000 {
		t.Fatalf("callback amount = %d (has=%v), want 1000", callback.AmountCents, callback.HasAmount)
	}

	// 篡改金额后签名应失效。
	params.Set("out_trade_no", "order-tampered")
	if _, err := gateway.VerifyCallback(context.Background(), http.Header{}, []byte(params.Encode())); err == nil {
		t.Fatalf("tampered callback unexpectedly verified")
	}
}

func TestWechatDecryptResource(t *testing.T) {
	privatePEM, _ := generateKeyPair(t)
	gateway, err := NewWechatGateway(WechatOptions{
		AppID:      "wxapp",
		MchID:      "1900000001",
		SerialNo:   "serial-1",
		PrivateKey: privatePEM,
		APIv3Key:   "0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("NewWechatGateway: %v", err)
	}

	plaintext := []byte(`{"out_trade_no":"order-2","transaction_id":"tx-2","trade_state":"SUCCESS","success_time":"2026-01-02T15:04:05+08:00"}`)
	nonce := "abcdefghijkl"
	associatedData := "transaction"
	ciphertext, err := encryptResource(gateway.apiV3Key, plaintext, nonce, associatedData)
	if err != nil {
		t.Fatalf("encryptResource: %v", err)
	}
	decrypted, err := gateway.decryptResource(ciphertext, nonce, associatedData)
	if err != nil {
		t.Fatalf("decryptResource: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted = %s", decrypted)
	}
}

func TestWechatRejectsBadKey(t *testing.T) {
	privatePEM, _ := generateKeyPair(t)
	if _, err := NewWechatGateway(WechatOptions{
		AppID: "wxapp", MchID: "1", SerialNo: "s", PrivateKey: privatePEM, APIv3Key: "short",
	}); err == nil {
		t.Fatalf("expected error for short api_v3_key")
	}
}

func TestFormatYuan(t *testing.T) {
	cases := map[int64]string{0: "0.00", 1000: "10.00", 1: "0.01", 12345: "123.45"}
	for cents, want := range cases {
		if got := formatYuan(cents); got != want {
			t.Fatalf("formatYuan(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestParseYuan(t *testing.T) {
	cases := map[string]int64{"0": 0, "10.00": 1000, "0.01": 1, "123.45": 12345, " 9.9 ": 990}
	for raw, want := range cases {
		got, err := parseYuan(raw)
		if err != nil {
			t.Fatalf("parseYuan(%q): %v", raw, err)
		}
		if got != want {
			t.Fatalf("parseYuan(%q) = %d, want %d", raw, got, want)
		}
	}
	if _, err := parseYuan("abc"); err == nil {
		t.Fatal("expected error for invalid amount")
	}
}

func TestEpayCreateAndVerifyCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if got := r.Form.Get("sign"); got != epaySign(r.Form, "secret-key") {
			_, _ = w.Write([]byte(`{"code":0,"msg":"bad sign"}`))
			return
		}
		if r.Form.Get("pid") != "1001" || r.Form.Get("out_trade_no") != "order-9" {
			_, _ = w.Write([]byte(`{"code":0,"msg":"bad params"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":1,"trade_no":"ep-1","payurl":"https://pay.example.com/qr/1"}`))
	}))
	defer server.Close()

	gateway, err := NewEpayGateway(EpayOptions{PID: "1001", Key: "secret-key", GatewayURL: server.URL, PayType: "alipay"})
	if err != nil {
		t.Fatalf("NewEpayGateway: %v", err)
	}
	result, err := gateway.Create(context.Background(), Order{
		ID: "order-9", Subject: "套餐", AmountCents: 1234, NotifyURL: "https://x/notify",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if result.PayURL != "https://pay.example.com/qr/1" || result.TradeNo != "ep-1" {
		t.Fatalf("create result = %+v", result)
	}

	notify := url.Values{}
	notify.Set("pid", "1001")
	notify.Set("trade_no", "ep-1")
	notify.Set("out_trade_no", "order-9")
	notify.Set("type", "alipay")
	notify.Set("money", "12.34")
	notify.Set("trade_status", "TRADE_SUCCESS")
	notify.Set("sign", epaySign(notify, "secret-key"))
	notify.Set("sign_type", "MD5")

	cb, err := gateway.VerifyCallback(context.Background(), http.Header{}, []byte(notify.Encode()))
	if err != nil {
		t.Fatalf("VerifyCallback: %v", err)
	}
	if cb.OrderID != "order-9" || cb.TradeNo != "ep-1" || !cb.Success || !cb.HasAmount || cb.AmountCents != 1234 {
		t.Fatalf("callback = %+v", cb)
	}

	notify.Set("money", "0.01")
	if _, err := gateway.VerifyCallback(context.Background(), http.Header{}, []byte(notify.Encode())); err == nil {
		t.Fatal("tampered notify unexpectedly verified")
	}
}

func TestWechatVerifiesPlatformSignature(t *testing.T) {
	privatePEM, publicPEM := generateKeyPair(t)
	gateway, err := NewWechatGateway(WechatOptions{
		AppID: "wxapp", MchID: "1900000001", SerialNo: "serial-1",
		PrivateKey: privatePEM, APIv3Key: "0123456789abcdef0123456789abcdef",
		PlatformPublicKey: publicPEM, PlatformSerialNo: "PLAT-1",
	})
	if err != nil {
		t.Fatalf("NewWechatGateway: %v", err)
	}
	plaintext := []byte(`{"out_trade_no":"order-2","transaction_id":"tx-2","trade_state":"SUCCESS","success_time":"2026-01-02T15:04:05+08:00","amount":{"total":1234}}`)
	nonce := "abcdefghijkl"
	associatedData := "transaction"
	ciphertext, err := encryptResource(gateway.apiV3Key, plaintext, nonce, associatedData)
	if err != nil {
		t.Fatalf("encryptResource: %v", err)
	}
	body := []byte(fmt.Sprintf(
		`{"event_type":"TRANSACTION.SUCCESS","resource":{"algorithm":"AEAD_AES_256_GCM","ciphertext":%q,"nonce":%q,"associated_data":%q}}`,
		ciphertext, nonce, associatedData,
	))
	timestamp, headerNonce := "1700000000", "headernonce"
	signature, err := signSHA256RSA(timestamp+"\n"+headerNonce+"\n"+string(body)+"\n", gateway.privateKey)
	if err != nil {
		t.Fatalf("signSHA256RSA: %v", err)
	}
	header := http.Header{}
	header.Set("Wechatpay-Signature", signature)
	header.Set("Wechatpay-Timestamp", timestamp)
	header.Set("Wechatpay-Nonce", headerNonce)
	header.Set("Wechatpay-Serial", "PLAT-1")

	cb, err := gateway.VerifyCallback(context.Background(), header, body)
	if err != nil {
		t.Fatalf("VerifyCallback: %v", err)
	}
	if cb.OrderID != "order-2" || !cb.Success || !cb.HasAmount || cb.AmountCents != 1234 {
		t.Fatalf("callback = %+v", cb)
	}

	header.Set("Wechatpay-Signature", "AAAA")
	if _, err := gateway.VerifyCallback(context.Background(), header, body); err == nil {
		t.Fatal("tampered signature unexpectedly verified")
	}
}

// encryptResource 用 AES-256-GCM 加密测试资源（与解密的对称实现）。
func encryptResource(key, plaintext []byte, nonce, associatedData string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, []byte(nonce), plaintext, []byte(associatedData))
	return base64.StdEncoding.EncodeToString(sealed), nil
}
