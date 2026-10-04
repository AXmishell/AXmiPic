package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP 参数（RFC 6238 默认值）。
const (
	totpSecretBytes = 20
	totpPeriod      = 30 * time.Second
	totpDigits      = 6
	totpSkewSteps   = 1
)

// totpEncoding 使用不带填充的大写 Base32，符合 Authenticator 约定。
var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret 生成一个新的、随机 Base32 编码的 TOTP 密钥。
func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, totpSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: generate totp secret: %w", err)
	}
	return totpEncoding.EncodeToString(buf), nil
}

// TOTPCode 计算 secret 在给定时刻的 6 位动态码。
func TOTPCode(secret string, at time.Time) (string, error) {
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		return "", err
	}
	counter := uint64(at.Unix() / int64(totpPeriod.Seconds()))
	return hotp(key, counter), nil
}

// VerifyTOTP 校验动态码，允许前后各一个时间步的时钟漂移。
func VerifyTOTP(secret, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		return false
	}
	counter := uint64(at.Unix() / int64(totpPeriod.Seconds()))
	for delta := -totpSkewSteps; delta <= totpSkewSteps; delta++ {
		candidate := uint64(int64(counter) + int64(delta))
		if subtle.ConstantTimeCompare([]byte(hotp(key, candidate)), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// OTPAuthURI 构造可供 Authenticator 扫描的 otpauth:// URI。
func OTPAuthURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", issuer)
	params.Set("algorithm", "SHA1")
	params.Set("digits", fmt.Sprintf("%d", totpDigits))
	params.Set("period", fmt.Sprintf("%d", int(totpPeriod.Seconds())))
	return "otpauth://totp/" + label + "?" + params.Encode()
}

// hotp 按 RFC 4226 计算动态截断后的十进制码。
func hotp(key []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, value%mod)
}

// decodeTOTPSecret 规范化并解码 Base32 密钥（容忍小写、空格与缺失填充）。
func decodeTOTPSecret(secret string) ([]byte, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	normalized = strings.TrimRight(normalized, "=")
	key, err := totpEncoding.DecodeString(normalized)
	if err != nil {
		return nil, fmt.Errorf("auth: invalid totp secret: %w", err)
	}
	return key, nil
}
