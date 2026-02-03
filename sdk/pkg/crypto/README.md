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
