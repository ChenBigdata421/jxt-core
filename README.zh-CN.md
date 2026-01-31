# go-admin-core

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

[English](README.md)

## ✨ 核心特性

### 📝 Logger 模块（企业级日志解决方案）

- **异步日志** - 45x 性能提升（3,358ns → 75ns），支持 100k+ QPS
- **采样日志** - 29x 性能提升（3,357ns → 116ns），智能频率控制
- **脱敏日志** - 自动脱敏敏感数据（手机号、密码、邮箱等），满足合规要求
- **Logrus 适配器** - 完整 Logrus 生态支持，50+ Hooks 可用
- **生产级配置** - 内置最佳实践配置（异步 + 采样 + 脱敏）
- **并发安全** - Race Detector 验证通过，零数据竞争

### 🚀 其他组件

- [x] 缓存组件（支持 memory）
- [x] 队列组件（支持 memory）
- [x] 配置管理（支持多种数据源）
- [x] 日志写入器（支持文件分割）

> **最新版本:** Go 1.25.1 | 119 个依赖包已升级 | 35 个单元测试 + 30+ 性能基准测试

---

## 🚀 快速开始

### 安装

```bash
go get -u github.com/go-admin-team/go-admin-core
```

**系统要求:** Go 1.25.1 或更高版本

### 基础日志使用

```go
package main

import "github.com/go-admin-team/go-admin-core/logger"

func main() {
    // 创建 Logrus 日志实例
    log := logger.NewLogrusLogger(
        logger.WithPath("logs/app.log"),
        logger.WithLevel(logger.InfoLevel),
    )
    
    log.Log(logger.InfoLevel, "Application started")
    log.Fields(map[string]interface{}{
        "user_id": 12345,
        "action":  "login",
    }).Log(logger.InfoLevel, "User action")
}
```

### 高性能异步日志

```go
// 创建异步日志（推荐用于高并发场景）
baseLog := logger.NewLogrusLogger(
    logger.WithPath("logs/app.log"),
    logger.WithLevel(logger.InfoLevel),
)

asyncLog := logger.NewAsyncLogger(baseLog, logger.DefaultAsyncConfig)
defer asyncLog.(interface{ Close() error }).Close()

// 45x 性能提升，延迟仅 75ns
asyncLog.Log(logger.InfoLevel, "High performance logging")
```

### 采样日志（频率控制）

```go
// 创建采样日志（自动过滤高频重复日志）
samplingLog := logger.NewSamplingLogger(baseLog, logger.DefaultSamplingConfig)

// 每秒仅记录前 100 条，其后每 100 条记录 1 条
for i := 0; i < 10000; i++ {
    samplingLog.Log(logger.InfoLevel, "High frequency log")
}
```

### 脱敏日志（数据安全）

```go
// 创建脱敏日志（自动处理敏感数据）
sanitizerLog := logger.NewSanitizerLogger(baseLog, logger.DefaultSanitizerConfig)

sanitizerLog.Fields(map[string]interface{}{
    "phone":    "13812345678",  // 自动脱敏 → "138****5678"
    "password": "secret123",    // 自动脱敏 → "[REDACTED]"
    "email":    "user@example.com", // 自动脱敏 → "u***@example.com"
}).Log(logger.InfoLevel, "User login")
```

### 生产级配置（推荐）

```go
// 组合使用：脱敏 → 采样 → 异步
sanitized := logger.NewSanitizerLogger(baseLog, logger.DefaultSanitizerConfig)
sampled := logger.NewSamplingLogger(sanitized, logger.DefaultSamplingConfig)
asyncLog := logger.NewAsyncLogger(sampled, logger.DefaultAsyncConfig)
defer asyncLog.(interface{ Close() error }).Close()

// 34x 性能提升 + 数据安全 + 频率控制
asyncLog.Fields(map[string]interface{}{
    "phone":   "13812345678",
    "user_id": 12345,
}).Log(logger.InfoLevel, "Production logging")
```

---

## 📦 配置管理

### 配置文件读取

```go
package main

import "github.com/go-admin-team/go-admin-core/config"

func main() {
    source := config.FileSource("config.json")
    config.Setup(source, func() {
        // 配置变更回调
    })
}
```

---

## 📊 性能指标

| 功能 | 性能提升 | 延迟 | 内存占用 | 适用场景 |
|------|---------|------|---------|---------|
| 异步日志 | 45x | 75ns | 120 B/op | 高并发写入 |
| 采样日志 | 29x | 116ns | 16 B/op | 高频重复日志 |
| 生产配置 | 34x | 98ns | 149 B/op | 生产环境推荐 |

**测试环境:** Apple M1 Pro | Go 1.25.1 | Race Detector 验证通过

---

## 🧪 测试覆盖

```bash
# 运行所有测试
go test ./... -v

# 性能基准测试
go test ./logger -bench=. -benchmem

# 并发安全检查
go test ./logger -race
```

**测试结果:**
- ✅ 单元测试: 35/35 通过
- ✅ 性能基准测试: 30+ 通过
- ✅ 并发安全: Race Detector 0 warnings
- ✅ 代码覆盖率: 85%+

---

## 📦 主要依赖

```go
github.com/casbin/casbin/v2         v2.135.0
github.com/gin-gonic/gin            v1.11.0
gorm.io/gorm                        v1.31.1
github.com/sirupsen/logrus          v1.9.3
golang.org/x/crypto                 v0.47.0
```

---

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

---

## 📝 License

Apache License 2.0 - 详见 [LICENSE](LICENSE) 文件
