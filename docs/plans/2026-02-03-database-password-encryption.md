# 数据库密码加密支持 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 jxt-core 添加 AES-256-GCM 加密/解密服务，支持 tenant-service 发布到 etcd 的加密数据库密码

**Architecture:** 新建 `sdk/pkg/crypto/` 模块提供独立的加密服务，`ServiceDatabaseConfig` 添加 `Password` 和 `PasswordEncrypted` 字段及 `DecryptPassword()` 方法

**Tech Stack:** Go 1.23+, crypto/aes, crypto/cipher (标准库), Ginkgo/Gomega (测试)

---

## Task 1: 创建 crypto 模块目录结构

**Files:**
- Create: `sdk/pkg/crypto/`

**Step 1: 创建 crypto 目录**

```bash
mkdir -p sdk/pkg/crypto
```

**Step 2: 创建 go.mod 文件（如果需要）**

检查 `sdk/pkg/crypto/` 是否需要独立的 go.mod（通常不需要，因为它是 jxt-core 的一部分）

```bash
# 验证父目录的 go.mod 存在
test -f sdk/go.mod && echo "go.mod exists" || echo "go.mod not found"
```

**Step 3: Commit**

```bash
git add sdk/pkg/crypto/
git commit -m "feat: create crypto module directory"
```

---

## Task 2: 实现 crypto 错误定义

**Files:**
- Create: `sdk/pkg/crypto/errors.go`
- Test: N/A（这是常量定义）

**Step 1: 写入错误定义**

```go
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
```

**Step 2: 运行 go fmt 验证格式**

```bash
go fmt sdk/pkg/crypto/errors.go
```

Expected: 无输出或仅显示文件名

**Step 3: Commit**

```bash
git add sdk/pkg/crypto/errors.go
git commit -m "feat(crypto): add error definitions"
```

---

## Task 3: 实现 AES-256-GCM 加密服务

**Files:**
- Create: `sdk/pkg/crypto/aes_gcm.go`
- Test: `sdk/pkg/crypto/crypto_test.go`

**Step 1: 先写测试 - TestNewCryptoService_ValidKey**

创建测试文件 `sdk/pkg/crypto/crypto_test.go`:

```go
package crypto

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("CryptoService", func() {
    var (
        validKey = "12345678901234567890123456789012" // 32 bytes
        shortKey = "short"
    )

    Describe("NewCryptoService", func() {
        Context("with valid 32-byte key", func() {
            It("should create service successfully", func() {
                service, err := NewCryptoService(validKey)
                Expect(err).ToNot(HaveOccurred())
                Expect(service).ToNot(BeNil())
            })
        })

        Context("with invalid key length", func() {
            It("should return ErrInvalidKeyLength", func() {
                _, err := NewCryptoService(shortKey)
                Expect(err).To(MatchError(ErrInvalidKeyLength))
            })
        })
    })
})
```

**Step 2: 运行测试验证失败**

```bash
cd sdk/pkg/crypto
go test -v -run TestNewCryptoService
```

Expected: FAIL with "undefined: NewCryptoService"

**Step 3: 实现最小代码使测试通过**

创建 `sdk/pkg/crypto/aes_gcm.go`:

```go
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
```

**Step 4: 运行测试验证通过**

```bash
cd sdk/pkg/crypto
go test -v -run TestNewCryptoService
```

Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/crypto/aes_gcm.go sdk/pkg/crypto/crypto_test.go
git commit -m "feat(crypto): add NewCryptoService with key validation"
```

---

## Task 4: 实现加密方法

**Files:**
- Modify: `sdk/pkg/crypto/aes_gcm.go`
- Modify: `sdk/pkg/crypto/crypto_test.go`

**Step 1: 写加密测试 - TestEncrypt**

在 `crypto_test.go` 中添加:

```go
    Describe("Encrypt", func() {
        var service *CryptoService

        BeforeEach(func() {
            service, _ = NewCryptoService(validKey)
        })

        Context("with valid plaintext", func() {
            It("should return base64-encoded ciphertext", func() {
                plaintext := "my-secret-password"
                ciphertext, err := service.Encrypt(plaintext)

                Expect(err).ToNot(HaveOccurred())
                Expect(ciphertext).ToNot(BeEmpty())
                Expect(ciphertext).ToNot(Equal(plaintext)) // 密文应该与原文不同

                // 验证是有效的 Base64
                decoded, err := base64.StdEncoding.DecodeString(ciphertext)
                Expect(err).ToNot(HaveOccurred())
                Expect(len(decoded)).To(BeNumerically(">", 12)) // 至少包含 nonce(12) + 密文 + tag(16)
            })

            It("should produce different ciphertext each time", func() {
                plaintext := "my-secret-password"

                ciphertext1, _ := service.Encrypt(plaintext)
                ciphertext2, _ := service.Encrypt(plaintext)

                // 因为每次加密都使用随机 nonce，结果应该不同
                Expect(ciphertext1).ToNot(Equal(ciphertext2))
            })

            It("should handle empty string", func() {
                ciphertext, err := service.Encrypt("")

                Expect(err).ToNot(HaveOccurred())
                Expect(ciphertext).ToNot(BeEmpty())
            })
        })
    })
```

需要添加 import:
```go
import (
    "encoding/base64"
    // ... 其他 imports
)
```

**Step 2: 运行测试验证失败**

```bash
cd sdk/pkg/crypto
go test -v -run TestCryptoService/Encrypt
```

Expected: FAIL with "service.Encrypt undefined"

**Step 3: 实现加密方法**

在 `aes_gcm.go` 中添加:

```go
import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "io"
)

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
```

**Step 4: 运行测试验证通过**

```bash
cd sdk/pkg/crypto
go test -v -run TestCryptoService/Encrypt
```

Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/crypto/aes_gcm.go sdk/pkg/crypto/crypto_test.go
git commit -m "feat(crypto): add Encrypt method with AES-256-GCM"
```

---

## Task 5: 实现解密方法

**Files:**
- Modify: `sdk/pkg/crypto/aes_gcm.go`
- Modify: `sdk/pkg/crypto/crypto_test.go`

**Step 1: 写解密测试 - TestDecrypt**

在 `crypto_test.go` 中添加:

```go
    Describe("Decrypt", func() {
        var service *CryptoService

        BeforeEach(func() {
            service, _ = NewCryptoService(validKey)
        })

        Context("with valid ciphertext", func() {
            It("should decrypt successfully", func() {
                plaintext := "my-secret-password"
                ciphertext, _ := service.Encrypt(plaintext)

                decrypted, err := service.Decrypt(ciphertext)

                Expect(err).ToNot(HaveOccurred())
                Expect(decrypted).To(Equal(plaintext))
            })

            It("should handle empty string", func() {
                ciphertext, _ := service.Encrypt("")

                decrypted, err := service.Decrypt(ciphertext)

                Expect(err).ToNot(HaveOccurred())
                Expect(decrypted).To(Equal(""))
            })
        })

        Context("with invalid ciphertext", func() {
            It("should fail with invalid base64", func() {
                _, err := service.Decrypt("not-valid-base64!!!")

                Expect(err).To(MatchError(ErrInvalidCiphertext))
            })

            It("should fail with wrong key", func() {
                wrongKey := "00000000000000000000000000000000"
                wrongService, _ := NewCryptoService(wrongKey)

                plaintext := "my-secret-password"
                ciphertext, _ := service.Encrypt(plaintext)

                _, err := wrongService.Decrypt(ciphertext)
                Expect(err).To(MatchError(ErrDecryptionFailed))
            })

            It("should fail with truncated ciphertext", func() {
                truncated := base64.StdEncoding.EncodeToString([]byte("too-short"))

                _, err := service.Decrypt(truncated)
                Expect(err).To(MatchError(ErrInvalidCiphertext))
            })
        })
    })
```

**Step 2: 运行测试验证失败**

```bash
cd sdk/pkg/crypto
go test -v -run TestCryptoService/Decrypt
```

Expected: FAIL with "service.Decrypt undefined"

**Step 3: 实现解密方法**

在 `aes_gcm.go` 中添加:

```go
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
```

**Step 4: 运行测试验证通过**

```bash
cd sdk/pkg/crypto
go test -v -run TestCryptoService/Decrypt
```

Expected: PASS

**Step 5: 运行所有 crypto 测试**

```bash
cd sdk/pkg/crypto
go test -v
```

Expected: 所有测试通过

**Step 6: Commit**

```bash
git add sdk/pkg/crypto/aes_gcm.go sdk/pkg/crypto/crypto_test.go
git commit -m "feat(crypto): add Decrypt method with validation"
```

---

## Task 6: 更新 ServiceDatabaseConfig 模型

**Files:**
- Modify: `sdk/pkg/tenant/provider/models.go`
- Test: `sdk/pkg/tenant/provider/models_test.go`

**Step 1: 写模型测试 - TestServiceDatabaseConfig_PasswordFields**

创建 `sdk/pkg/tenant/provider/models_test.go`:

```go
package provider

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("ServiceDatabaseConfig", func() {
    Describe("Password fields", func() {
        It("should have Password and PasswordEncrypted fields", func() {
            config := &ServiceDatabaseConfig{
                TenantID:          1,
                ServiceCode:       "evidence-command",
                Password:          "encrypted-password-here",
                PasswordEncrypted: true,
            }

            Expect(config.TenantID).To(Equal(1))
            Expect(config.Password).To(Equal("encrypted-password-here"))
            Expect(config.PasswordEncrypted).To(BeTrue())
        })
    })
})
```

**Step 2: 运行测试验证失败**

```bash
cd sdk/pkg/tenant/provider
go test -v -run TestServiceDatabaseConfig/Password
```

Expected: FAIL (如果字段不存在) 或 PASS (如果已有字段)

**Step 3: 添加字段到 ServiceDatabaseConfig**

编辑 `models.go`，更新 ServiceDatabaseConfig 结构体:

```go
// ServiceDatabaseConfig 服务级数据库配置
type ServiceDatabaseConfig struct {
    TenantID          int    `json:"tenantId"`
    ServiceCode       string `json:"serviceCode"` // evidence-command, evidence-query, file-storage, security-management
    Driver            string `json:"driver"`
    Host              string `json:"host"`
    Port              int    `json:"port"`
    Database          string `json:"database"`
    Username          string `json:"username"`
    Password          string `json:"password"`           // 加密后的密码（Base64编码）
    SSLMode           string `json:"sslMode"`
    MaxOpenConns      int    `json:"maxOpenConns"`
    MaxIdleConns      int    `json:"maxIdleConns"`
    PasswordEncrypted bool   `json:"passwordEncrypted"` // 密码加密标识
}
```

**Step 4: 运行测试验证通过**

```bash
cd sdk/pkg/tenant/provider
go test -v -run TestServiceDatabaseConfig/Password
```

Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/models.go sdk/pkg/tenant/provider/models_test.go
git commit -m "feat(tenant): add Password and PasswordEncrypted fields to ServiceDatabaseConfig"
```

---

## Task 7: 实现 HasEncryptedPassword 方法

**Files:**
- Modify: `sdk/pkg/tenant/provider/models.go`
- Modify: `sdk/pkg/tenant/provider/models_test.go`

**Step 1: 写测试 - TestHasEncryptedPassword**

在 `models_test.go` 中添加:

```go
    Describe("HasEncryptedPassword", func() {
        Context("when password is set and encrypted", func() {
            It("should return true", func() {
                config := &ServiceDatabaseConfig{
                    Password:          "some-encrypted-value",
                    PasswordEncrypted: true,
                }
                Expect(config.HasEncryptedPassword()).To(BeTrue())
            })
        })

        Context("when password is empty", func() {
            It("should return false", func() {
                config := &ServiceDatabaseConfig{
                    PasswordEncrypted: true,
                }
                Expect(config.HasEncryptedPassword()).To(BeFalse())
            })
        })

        Context("when PasswordEncrypted is false", func() {
            It("should return false", func() {
                config := &ServiceDatabaseConfig{
                    Password: "some-value",
                }
                Expect(config.HasEncryptedPassword()).To(BeFalse())
            })
        })
    })
```

**Step 2: 运行测试验证失败**

```bash
cd sdk/pkg/tenant/provider
go test -v -run TestServiceDatabaseConfig/HasEncryptedPassword
```

Expected: FAIL with "undefined: HasEncryptedPassword"

**Step 3: 实现 HasEncryptedPassword 方法**

在 `models.go` 中，ServiceDatabaseConfig 定义后添加:

```go
// HasEncryptedPassword 检查是否有加密的密码
func (c *ServiceDatabaseConfig) HasEncryptedPassword() bool {
    return c.PasswordEncrypted && c.Password != ""
}
```

**Step 4: 运行测试验证通过**

```bash
cd sdk/pkg/tenant/provider
go test -v -run TestServiceDatabaseConfig/HasEncryptedPassword
```

Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/models.go sdk/pkg/tenant/provider/models_test.go
git commit -m "feat(tenant): add HasEncryptedPassword method"
```

---

## Task 8: 实现 DecryptPassword 方法

**Files:**
- Modify: `sdk/pkg/tenant/provider/models.go`
- Modify: `sdk/pkg/tenant/provider/models_test.go`

**Step 1: 写测试 - TestDecryptPassword**

在 `models_test.go` 中添加:

```go
import (
    "jxt-evidence-system/jxt-core/sdk/pkg/crypto"
    // ... 其他 imports
)

    Describe("DecryptPassword", func() {
        var (
            encryptionKey = "12345678901234567890123456789012"
            config        *ServiceDatabaseConfig
        )

        BeforeEach(func() {
            config = &ServiceDatabaseConfig{
                TenantID:          1,
                ServiceCode:       "evidence-command",
                PasswordEncrypted: true,
            }
        })

        Context("with valid encrypted password", func() {
            It("should decrypt successfully", func() {
                // 先加密一个密码
                cryptoService, _ := crypto.NewCryptoService(encryptionKey)
                originalPassword := "my-db-password"
                encrypted, _ := cryptoService.Encrypt(originalPassword)

                config.Password = encrypted

                // 解密
                decrypted, err := config.DecryptPassword(encryptionKey)

                Expect(err).ToNot(HaveOccurred())
                Expect(decrypted).To(Equal(originalPassword))
            })
        })

        Context("when password is empty", func() {
            It("should return error", func() {
                config.Password = ""

                _, err := config.DecryptPassword(encryptionKey)

                Expect(err).To(MatchError(ContainSubstring("password is empty")))
            })
        })

        Context("when PasswordEncrypted is false", func() {
            It("should return error", func() {
                config.Password = "some-value"
                config.PasswordEncrypted = false

                _, err := config.DecryptPassword(encryptionKey)

                Expect(err).To(MatchError(ContainSubstring("password is not encrypted")))
            })
        })

        Context("with wrong key", func() {
            It("should fail to decrypt", func() {
                cryptoService, _ := crypto.NewCryptoService(encryptionKey)
                encrypted, _ := cryptoService.Encrypt("password")
                config.Password = encrypted

                wrongKey := "00000000000000000000000000000000"

                _, err := config.DecryptPassword(wrongKey)

                Expect(err).To(MatchError(ContainSubstring("failed to decrypt password")))
            })
        })
    })
```

需要添加 import:
```go
import (
    "fmt"
    "jxt-evidence-system/jxt-core/sdk/pkg/crypto"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/onsi/gomega/format"
)
```

同时添加匹配器:
```go
// 在测试文件开头添加
func init() {
    // 注册自定义匹配器
    format.RegisterCustomMatcher(func() interface{} {
        return &ContainSubstringMatcher{}
    })
}

type ContainSubstringMatcher struct {
    substring string
}

func (m *ContainSubstringMatcher) Match(actual interface{}) (bool, error) {
    actualString, ok := actual.(error)
    if !ok {
        return false, fmt.Errorf("ContainSubstring matcher expects an error")
    }
    return fmt.Sprintf("%v", actualString) != "" &&
           fmt.Sprintf("%v", actualString) != "nil", nil
}

func (m *ContainSubstringMatcher) FailureMessage(actual interface{}) string {
    return fmt.Sprintf("expected error to contain substring")
}
```

或者更简单的方式，直接使用标准匹配器:
```go
Expect(err).To(HaveOccurred())
Expect(err.Error()).To(ContainSubstring("password is empty"))
```

**Step 2: 运行测试验证失败**

```bash
cd sdk/pkg/tenant/provider
go test -v -run TestServiceDatabaseConfig/DecryptPassword
```

Expected: FAIL with "undefined: DecryptPassword"

**Step 3: 实现 DecryptPassword 方法**

在 `models.go` 中添加 import 和方法:

```go
import (
    "fmt"
    "jxt-evidence-system/jxt-core/sdk/pkg/crypto"
)

// DecryptPassword 解密数据库密码
// encryptionKey: 32字节的 AES-256 密钥（由调用者传入）
// 返回: 解密后的明文密码
func (c *ServiceDatabaseConfig) DecryptPassword(encryptionKey string) (string, error) {
    if c.Password == "" {
        return "", fmt.Errorf("password is empty")
    }
    if !c.PasswordEncrypted {
        return "", fmt.Errorf("password is not encrypted")
    }

    cryptoService, err := crypto.NewCryptoService(encryptionKey)
    if err != nil {
        return "", fmt.Errorf("failed to create crypto service: %w", err)
    }

    password, err := cryptoService.Decrypt(c.Password)
    if err != nil {
        return "", fmt.Errorf("failed to decrypt password: %w", err)
    }

    return password, nil
}
```

**Step 4: 运行测试验证通过**

```bash
cd sdk/pkg/tenant/provider
go test -v -run TestServiceDatabaseConfig/DecryptPassword
```

Expected: PASS

**Step 5: Commit**

```bash
git add sdk/pkg/tenant/provider/models.go sdk/pkg/tenant/provider/models_test.go
git commit -m "feat(tenant): add DecryptPassword method to ServiceDatabaseConfig"
```

---

## Task 9: 创建 crypto 模块 README

**Files:**
- Create: `sdk/pkg/crypto/README.md`

**Step 1: 创建 README**

```bash
cat > sdk/pkg/crypto/README.md << 'EOF'
# crypto 加密服务模块

提供 AES-256-GCM 加密/解密服务，用于保护敏感数据（如数据库密码）。

## 使用方法

### 创建加密服务

```go
import "jxt-evidence-system/jxt-core/sdk/pkg/crypto"

// 密钥必须是 32 字节
key := "12345678901234567890123456789012"
cryptoService, err := crypto.NewCryptoService(key)
if err != nil {
    log.Fatal(err)
}
```

### 加密

```go
plaintext := "my-secret-password"
ciphertext, err := cryptoService.Encrypt(plaintext)
if err != nil {
    log.Fatal(err)
}

// ciphertext 是 Base64 编码的密文字符串
// 格式: base64(nonce + ciphertext + tag)
fmt.Println(ciphertext)
```

### 解密

```go
plaintext, err := cryptoService.Decrypt(ciphertext)
if err != nil {
    log.Fatal(err)
}

fmt.Println(plaintext) // "my-secret-password"
```

## 在 ServiceDatabaseConfig 中使用

```go
import "jxt-evidence-system/jxt-core/sdk/pkg/tenant/provider"

// 从 Provider 获取配置
config, ok := provider.GetServiceDatabaseConfig(tenantID, serviceCode)
if !ok {
    log.Fatal("config not found")
}

// 解密密码（密钥从环境变量获取）
encryptionKey := os.Getenv("ENCRYPTION_KEY")
password, err := config.DecryptPassword(encryptionKey)
if err != nil {
    log.Fatal(err)
}

// 使用 password 建立数据库连接
dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
    config.Username, password, config.Host, config.Port, config.Database)
```

## 错误处理

模块定义了以下错误：

- `ErrInvalidKeyLength`: 密钥长度不是 32 字节
- `ErrInvalidCiphertext`: 密文格式无效（如 Base64 解码失败）
- `ErrDecryptionFailed`: 解密失败（通常是密钥错误或密文被篡改）

## 安全注意事项

1. **密钥管理**:
   - 密钥通过环境变量 `ENCRYPTION_KEY` 配置
   - 所有服务必须使用相同的密钥
   - 生产环境建议使用密钥管理服务（如 HashiCorp Vault）

2. **密钥轮换**:
   - 建议每 90-180 天轮换一次密钥
   - 轮换后需要重新发布所有租户配置

3. **加密算法**:
   - AES-256-GCM 提供认证加密
   - 每次加密使用随机 nonce（12 字节）
   - 输出格式: `base64(nonce + ciphertext + tag)`
EOF
```

**Step 2: 验证文件创建**

```bash
cat sdk/pkg/crypto/README.md
```

Expected: 显示 README 内容

**Step 3: Commit**

```bash
git add sdk/pkg/crypto/README.md
git commit -m "docs(crypto): add README with usage examples"
```

---

## Task 10: 运行所有测试

**Files:**
- Test: 所有测试文件

**Step 1: 运行 crypto 模块测试**

```bash
cd sdk/pkg/crypto
go test -v
```

Expected: 所有测试通过

**Step 2: 运行 tenant provider 测试**

```bash
cd sdk/pkg/tenant/provider
go test -v
```

Expected: 所有测试通过（包括新测试）

**Step 3: 运行 jxt-core 所有测试**

```bash
cd sdk
go test ./...
```

Expected: 所有测试通过

**Step 4: 如果有测试失败，修复并重新运行**

**Step 5: Commit（如果有任何修复）**

```bash
# 如果需要修复
git add ...
git commit -m "test: fix failing tests"
```

---

## Task 11: 更新 tenant provider README

**Files:**
- Modify: `sdk/pkg/tenant/README.md`（如果存在）

**Step 1: 检查 README 是否存在**

```bash
test -f sdk/pkg/tenant/README.md && echo "exists" || echo "not found"
```

**Step 2: 如果存在，更新 README**

如果文件存在，添加密码解密部分：

```bash
cat >> sdk/pkg/tenant/README.md << 'EOF'

## 数据库密码解密

从 jxt-core Provider 获取的 `ServiceDatabaseConfig` 包含加密后的数据库密码。要使用密码建立连接，需要先解密：

```go
import (
    "jxt-evidence-system/jxt-core/sdk/pkg/tenant/provider"
    "os"
)

// 从环境变量获取加密密钥
encryptionKey := os.Getenv("ENCRYPTION_KEY")

// 获取配置
config, ok := provider.GetServiceDatabaseConfig(tenantID, serviceCode)
if !ok {
    log.Fatal("config not found")
}

// 检查是否有加密密码
if !config.HasEncryptedPassword() {
    log.Fatal("no encrypted password in config")
}

// 解密密码
password, err := config.DecryptPassword(encryptionKey)
if err != nil {
    log.Fatalf("failed to decrypt password: %v", err)
}

// 构建数据库连接
dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
    config.Username, password, config.Host, config.Port, config.Database)
```

注意：加密密钥（`ENCRYPTION_KEY`）必须是 32 字节，并且所有服务必须使用相同的密钥。
EOF
```

**Step 3: Commit**

```bash
git add sdk/pkg/tenant/README.md
git commit -m "docs(tenant): add password decryption guide to README"
```

---

## 验收标准

完成所有任务后，验证：

- [ ] `sdk/pkg/crypto/` 模块创建完成，包含 `aes_gcm.go`, `errors.go`, `crypto_test.go`
- [ ] `ServiceDatabaseConfig` 新增 `Password` 和 `PasswordEncrypted` 字段
- [ ] `ServiceDatabaseConfig` 新增 `DecryptPassword()` 和 `HasEncryptedPassword()` 方法
- [ ] 所有单元测试通过（crypto: 6+ 测试，provider: 4+ 测试）
- [ ] `sdk/pkg/crypto/README.md` 文档完整
- [ ] `sdk/pkg/tenant/README.md` 更新（如果存在）

---

## 下一步

实施完成后，可以：

1. **集成测试**: 创建 `provider_crypto_test.go` 测试完整的 ETCD → Provider → 解密流程
2. **微服务适配**: 更新各个微服务使用新的密码解密 API
3. **tenant-service 适配**: tenant-service 也可以使用 jxt-core 的加密服务
