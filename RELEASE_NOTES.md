# go-admin-core Release Notes

发布日期：2026-01-31

## 🎉 重大更新

### Go 1.25.1 升级

- ✅ **Go 版本**：1.24.3 → **1.25.1**
- ✅ **依赖包**：升级 **119 个包**到最新稳定版本
- ✅ **兼容性**：修复 6 个测试以兼容 Go 1.25
- ✅ **性能**：无性能回退，所有测试通过

### Logger 模块增强 🚀

#### 新功能

1. **异步日志**（AsyncLogger）
   - **性能提升：45 倍**（3,358ns → 75ns）
   - **特性**：非阻塞写入、缓冲队列、优雅关闭
   - **适用场景**：高QPS接口（>1k QPS）
   - **内存友好**：仅 120 B/op，2 allocs/op

2. **采样日志**（SamplingLogger）
   - **性能提升：29 倍**（3,357ns → 116ns）
   - **特性**：智能采样、日志风暴防护
   - **适用场景**：高频调试日志、日志量控制
   - **内存友好**：仅 16 B/op，1 allocs/op

3. **脱敏日志**（SanitizerLogger）
   - **特性**：自动脱敏敏感字段（手机号、密码、Token）
   - **规则**：14 个内置规则 + 自定义规则支持
   - **适用场景**：生产环境日志合规
   - **支持模式**：掩码、删除、哈希、替换

4. **生产级配置**（异步+采样+脱敏组合）
   - **性能提升：34 倍**（3,357ns → 98ns）
   - **特性**：高性能 + 安全 + 智能采样
   - **推荐**：生产环境默认配置

#### 性能验证

- ✅ **单元测试**：35/35 通过
- ✅ **性能基准测试**：30+ 个测试通过
- ✅ **并发安全**：Race Detector 验证通过（0 warnings）
- ✅ **压力测试**：支持 100k+ QPS

#### 技术文档

- 📊 [PERFORMANCE_REPORT.md](logger/PERFORMANCE_REPORT.md) - 完整性能测试报告
- 🔧 [CALLER_OPTIMIZATION_ANALYSIS.md](logger/CALLER_OPTIMIZATION_ANALYSIS.md) - Caller 优化分析
- ⚡ [ZAP_PERFORMANCE_ANALYSIS.md](logger/ZAP_PERFORMANCE_ANALYSIS.md) - Zap 性能深度分析
- 🔒 [CONCURRENCY_SAFETY_ANALYSIS.md](logger/CONCURRENCY_SAFETY_ANALYSIS.md) - 并发安全验证
- 📖 [README_ADVANCED.md](logger/README_ADVANCED.md) - 高级功能使用指南

---

## 🔧 修复和改进

- **修复**：6 个测试失败（Go 1.25 兼容性）
- **修复**：3 个异步测试数据竞争
- **优化**：Caller 追踪性能
- **改进**：错误处理和日志格式

---

## 📦 依赖更新

升级 119 个依赖包，包括：

```go
github.com/casbin/casbin/v2 v2.128.0 → v2.135.0
github.com/casbin/gorm-adapter/v3 v3.37.0 → v3.41.0
github.com/gin-gonic/gin v1.10.1 → v1.11.0
github.com/go-playground/validator/v10 v10.20.0 → v10.27.0
golang.org/x/crypto v0.37.0 → v0.47.0
google.golang.org/protobuf v1.36.6 → v1.36.9
gorm.io/gorm v1.30.0 → v1.31.1
// ... (完整列表见 go.mod)
```

---

## 🚀 使用示例

### 1. 异步日志（推荐生产环境）

```go
import "github.com/go-admin-team/go-admin-core/logger"

// 创建基础 logger
baseLog := logger.NewLogrusLogger(
    logger.WithPath("logs/app.log"),
    logger.WithLevel(logger.InfoLevel),
)

// 包装为异步 logger
asyncLog := logger.NewAsyncLogger(baseLog, logger.DefaultAsyncConfig)
defer asyncLog.(interface{ Close() error }).Close()

// 高性能日志（45x 性能提升）
asyncLog.Log(logger.InfoLevel, "High performance logging")
```

### 2. 采样日志（日志风暴防护）

```go
// 创建采样 logger（前 10 条全记录，之后 1/10 采样）
samplingLog := logger.NewSamplingLogger(baseLog, logger.DefaultSamplingConfig)

// 高频日志自动采样（80% 日志减少，29x 性能提升）
for i := 0; i < 100; i++ {
    samplingLog.Log(logger.InfoLevel, "High frequency log")
}
```

### 3. 脱敏日志（敏感数据保护）

```go
// 创建脱敏 logger
sanitizerLog := logger.NewSanitizerLogger(baseLog, logger.DefaultSanitizerConfig)

// 敏感字段自动脱敏
sanitizerLog.Fields(map[string]interface{}{
    "phone":    "13812345678",       // → "138****5678"
    "password": "secret123",         // → "[REDACTED]"
    "token":    "abc123xyz",         // → "abc1...xyz" (哈希)
}).Log(logger.InfoLevel, "User login")
```

### 4. 生产级配置（异步+采样+脱敏）

```go
// 创建基础 logger
baseLog := logger.NewLogrusLogger(
    logger.WithPath("logs/app.log"),
    logger.WithLevel(logger.InfoLevel),
)

// 层层包装：脱敏 → 采样 → 异步
sanitized := logger.NewSanitizerLogger(baseLog, logger.DefaultSanitizerConfig)
sampled := logger.NewSamplingLogger(sanitized, logger.DefaultSamplingConfig)
asyncLog := logger.NewAsyncLogger(sampled, logger.DefaultAsyncConfig)
defer asyncLog.(interface{ Close() error }).Close()

// 生产级高性能日志
// - 性能：10.2M ops/sec，延迟 98ns（34x 性能提升）
// - 安全：自动脱敏敏感字段
// - 智能：高频日志自动采样
asyncLog.Fields(map[string]interface{}{
    "phone":   "13812345678",  // 自动脱敏
    "user_id": 12345,
}).Log(logger.InfoLevel, "User action")
```

---

## 📊 性能对比

### 基准测试结果

| 场景 | 同步基线 | 优化后 | 提升倍数 | 内存占用 |
|------|---------|--------|---------|---------|
| **异步日志** | 3,358ns | 75ns | **45x** ⚡ | 120 B/op |
| **采样日志** | 3,357ns | 116ns | **29x** ⚡ | 16 B/op |
| **生产配置** | 3,357ns | 98ns | **34x** ⚡ | 149 B/op |
| **Logrus 基线** | 3,339ns | - | - | 1,546 B/op |
| **Zap 结构化** | 2,099ns | - | 1.6x | 649 B/op |

### 并发安全验证

```bash
# Race Detector 验证（所有测试通过，0 warnings）
$ go test -race -count=1 ./logger/...
ok  github.com/go-admin-team/go-admin-core/logger  2.456s
```

---

## ⚠️ 破坏性变更

**无破坏性变更，完全向后兼容。**

所有新功能都是**可选的增强**，现有代码无需修改即可继续使用。

---

## 🔗 相关资源

- **GitHub**: https://github.com/go-admin-team/go-admin-core
- **文档**: [logger/README_ADVANCED.md](logger/README_ADVANCED.md)
- **性能报告**: [logger/PERFORMANCE_REPORT.md](logger/PERFORMANCE_REPORT.md)
- **优化分析**: [logger/CALLER_OPTIMIZATION_ANALYSIS.md](logger/CALLER_OPTIMIZATION_ANALYSIS.md)

---

## 📝 Changelog

详细变更记录：

```
feat: Upgrade to Go 1.25.1 and enhance logger with async/sampling/sanitizer

Major Changes:
- Upgrade Go from 1.24.3 to 1.25.1
- Upgrade 119 dependency packages to latest stable versions
- Add async logger with 45x performance improvement
- Add sampling logger with 29x performance improvement
- Add sanitizer logger for sensitive data protection
- Add 30+ performance benchmarks and 35 unit tests
- Fix 6 test failures for Go 1.25 compatibility
- Fix 3 async test data races (Race Detector verified)

Performance:
- Async logger: 3,358ns → 75ns (45x faster)
- Sampling logger: 3,357ns → 116ns (29x faster)
- Production config: 3,357ns → 98ns (34x faster)
- All tests pass: 35/35 unit tests, 30+ benchmarks
- Concurrency safe: Race Detector 0 warnings

Documentation:
- PERFORMANCE_REPORT.md: Comprehensive performance analysis
- CALLER_OPTIMIZATION_ANALYSIS.md: Caller optimization strategies
- ZAP_PERFORMANCE_ANALYSIS.md: Why Zap is faster (allocations analysis)
- CONCURRENCY_SAFETY_ANALYSIS.md: Full concurrency safety verification
- README_ADVANCED.md: Advanced features guide
```

---

## 👥 贡献者

感谢所有贡献者的辛勤工作！

特别感谢：
- [@zhangwenjian](https://github.com/zhangwenjian) - Go 1.25 升级、Logger 增强、性能优化
- GitHub Copilot - 代码审查、测试设计、并发安全分析

---

## 🎯 下一步计划

- [ ] 监控集成：Prometheus metrics 支持
- [ ] 分布式追踪：OpenTelemetry 集成
- [ ] 日志查询：结构化日志查询 API
- [ ] 更多示例：真实生产环境案例

---

完整更新日志：https://github.com/go-admin-team/go-admin-core/releases
