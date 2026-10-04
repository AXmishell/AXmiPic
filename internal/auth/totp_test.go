package auth

import (
	"encoding/base32"
	"testing"
	"time"
)

// TestTOTPCodeRFC6238 使用 RFC 6238 附录 B 的 SHA1 测试向量校验实现。
func TestTOTPCodeRFC6238(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).
		EncodeToString([]byte("12345678901234567890"))

	cases := []struct {
		unix int64
		want string // 8 位向量的后 6 位
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}
	for _, tc := range cases {
		got, err := TOTPCode(secret, time.Unix(tc.unix, 0))
		if err != nil {
			t.Fatalf("TOTPCode(%d): %v", tc.unix, err)
		}
		if got != tc.want {
			t.Fatalf("TOTPCode(%d) = %s, want %s", tc.unix, got, tc.want)
		}
	}
}

func TestVerifyTOTPSkew(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	now := time.Now()
	code, err := TOTPCode(secret, now)
	if err != nil {
		t.Fatalf("TOTPCode: %v", err)
	}
	if !VerifyTOTP(secret, code, now) {
		t.Fatalf("VerifyTOTP rejected current code %s", code)
	}
	// 前一个时间步的验证码也应被接受（时钟漂移容忍）。
	prev, _ := TOTPCode(secret, now.Add(-totpPeriod))
	if !VerifyTOTP(secret, prev, now) {
		t.Fatalf("VerifyTOTP rejected previous-step code %s", prev)
	}
	if VerifyTOTP(secret, "000000", now) && code != "000000" {
		t.Fatalf("VerifyTOTP accepted a wrong code")
	}
	if VerifyTOTP(secret, "abc", now) {
		t.Fatalf("VerifyTOTP accepted a malformed code")
	}
}
