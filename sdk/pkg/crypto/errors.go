// Package crypto 提供 AES-256-GCM 加密/解密服务
package crypto

import "errors"

var (
	// ErrInvalidKeyLength 密钥长度无效（必须是32字节）
	ErrInvalidKeyLength = errors.New("encryption key must be 32 bytes")

	// ErrInvalidCiphertext 密文格式无效
	ErrInvalidCiphertext = errors.New("invalid ciphertext format")

	// ErrDecryptionFailed 解密失败（认证失败，可能是密钥错误或密文被篡改）
	ErrDecryptionFailed = errors.New("decryption failed: authentication failed")
)
