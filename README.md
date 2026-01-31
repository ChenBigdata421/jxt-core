# go-admin-team 公共代码库

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

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

## 配置文件读取

使用 `Setup` 函数初始化配置：

```shell
package main

import "github.com/GoAdminTeam/go-admin-core/logger"

source := config.FileSource("config.json")
config.Setup(source, func() {
   // 回调函数逻辑
})
```

## 日志记录

使用 Log 和 Logf 方法记录日志：

```shell
package main

// import "github.com/GoAdminTeam/go-admin-core/logger"

logger := logger.NewDefaultLogger()
logger.Log(logger.INFO, "This is an info message")
logger.Logf(logger.ERROR, "This is an error message: %s", "error details")
```
