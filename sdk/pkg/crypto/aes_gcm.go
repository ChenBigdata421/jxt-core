package crypto

import (
	"fmt"
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
