package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// CryptoService AES-256-GCM 加密服务
type CryptoService struct {
	key []byte
}

// NewCryptoService 创建加密服务（密钥必须是32字节）
func NewCryptoService(key string) (*CryptoService, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKeyLength
	}
	return &CryptoService{key: []byte(key)}, nil
}

// String 返回服务的字符串表示（用于调试）
func (s *CryptoService) String() string {
	return "CryptoService{key: [REDACTED]}"
}

// GoString 返回服务的 Go 字符串表示
func (s *CryptoService) GoString() string {
	return fmt.Sprintf("&CryptoService{keyLen: %d}", len(s.key))
}

// Encrypt 加密明文，返回 Base64 编码的密文
func (s *CryptoService) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// 生成随机 nonce (12 bytes for GCM)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// 加密并附加认证标签
	// Seal: nonce + ciphertext + tag
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Base64 编码
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 Base64 编码的密文，返回明文
func (s *CryptoService) Decrypt(ciphertextBase64 string) (string, error) {
	// Base64 解码
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCiphertext, err)
	}

	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", ErrInvalidCiphertext
	}

	// 提取 nonce 和密文
	nonce, cipher := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// 解密并验证
	plaintext, err := gcm.Open(nil, nonce, cipher, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}
