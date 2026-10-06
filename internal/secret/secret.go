// Package secret 提供对称加密工具，用于加密存储的敏感配置（如对象存储密钥）。
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrInvalidCiphertext 表示密文格式错误或无法解密。
var ErrInvalidCiphertext = errors.New("secret: invalid ciphertext")

// versionPrefix 标记带版本的新格式密文；旧格式无前缀。base64 URL 字母表不含
// ':'，因此该前缀不会与历史密文冲突。
const versionPrefix = "v1:"

// MinMasterKeyLen 是建议的主密钥最小字节数；短于此值会在启动时告警。
const MinMasterKeyLen = 16

// Cipher 使用 AES-256-GCM 加解密字符串。
type Cipher struct {
	aead cipher.AEAD
}

// New 由主密钥派生出一个 Cipher。主密钥长度不限，内部会做 sha256 归一化。
func New(masterKey []byte) (*Cipher, error) {
	if len(masterKey) == 0 {
		return nil, fmt.Errorf("secret: master key must not be empty")
	}
	key := sha256.Sum256(masterKey)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("secret: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret: new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// WeakKey 报告主密钥是否短于建议长度。
func WeakKey(masterKey []byte) bool {
	return len(masterKey) < MinMasterKeyLen
}

// Encrypt 加密明文（不绑定 AAD），返回带版本前缀的 base64 编码密文。
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	return c.EncryptWithAAD(plaintext, "")
}

// EncryptWithAAD 加密明文，并把 aad 作为附加认证数据绑定到密文。aad 应为稳定的
// 字段标识（如 "settings.smtp.password"），用于防止密文在不同字段之间被搬运。
func (c *Cipher) EncryptWithAAD(plaintext, aad string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secret: nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), aadBytes(aad))
	return versionPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密由 Encrypt/EncryptWithAAD 产生的字符串。
func (c *Cipher) Decrypt(encoded string) (string, error) {
	return c.DecryptWithAAD(encoded, "")
}

// DecryptWithAAD 解密字符串，兼容三种情况：带版本前缀且绑定 aad 的新密文、
// 带前缀但历史未绑定 aad 的密文、以及完全无前缀的旧格式密文。
func (c *Cipher) DecryptWithAAD(encoded, aad string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	versioned := strings.HasPrefix(encoded, versionPrefix)
	payload := strings.TrimPrefix(encoded, versionPrefix)
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("%w: base64", ErrInvalidCiphertext)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", ErrInvalidCiphertext
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]

	if !versioned {
		// 旧格式：无 AAD。
		plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return "", ErrInvalidCiphertext
		}
		return string(plaintext), nil
	}
	if plaintext, err := c.aead.Open(nil, nonce, ciphertext, aadBytes(aad)); err == nil {
		return string(plaintext), nil
	}
	// 迁移期：历史 v1 密文可能未绑定 AAD，回退再试一次。
	if aad != "" {
		if plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil); err == nil {
			return string(plaintext), nil
		}
	}
	return "", ErrInvalidCiphertext
}

// aadBytes 把 AAD 字符串转换为字节；空字符串表示不使用 AAD。
func aadBytes(aad string) []byte {
	if aad == "" {
		return nil
	}
	return []byte(aad)
}
