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
	params.Set("gmt_payment", "2026-01-02 15:04:05")
	signature, err := signAlipay(params, gateway.privateKey)
	if err != nil {
		t.Fatalf("signAlipay: %v", err)
	}
	params.Set("sign", signature)
	params.Set("sign_type", "RSA2")

	callback, err := gateway.VerifyCallback(context.Background(), []byte(params.Encode()))
	if err != nil {
		t.Fatalf("VerifyCallback: %v", err)
	}
	if callback.OrderID != "order-1" || callback.TradeNo != "trade-1" || !callback.Success {
		t.Fatalf("callback = %+v", callback)
	}

	// 篡改金额后签名应失效。
	params.Set("out_trade_no", "order-tampered")
	if _, err := gateway.VerifyCallback(context.Background(), []byte(params.Encode())); err == nil {
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
