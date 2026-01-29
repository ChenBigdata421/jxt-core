# Core 项目职责边界审阅报告

## 审阅日期
2026-01-30

## 审阅目的
确认 go-admin-core 的代码符合基础设施库的定位，不包含业务逻辑。

---

## 核心原则

### ✅ Core 应该包含的内容
1. **基础抽象** - 通用的接口定义（Logger 接口、Storage 接口等）
2. **适配器实现** - 对第三方库的封装（Logrus、Zap 适配器）
3. **工具函数** - 无状态的通用工具（加密、验证、转换）
4. **配置绑定** - 配置文件到代码的映射层
5. **中间件框架** - 通用的中间件骨架（不含业务规则）

### ❌ Core 不应该包含的内容
1. **业务逻辑** - 特定业务规则（跳过 login/logout 路径）
2. **业务中间件** - 包含业务判断的中间件
3. **框架绑定** - 强依赖特定框架的实现
4. **项目特定配置** - 写死的业务路径、规则

---

## 本次修改审阅

### ❌ 已删除的文件（不属于 Core）

#### 1. `sdk/pkg/middleware/request_logger.go`

**问题**：
```go
// 跳过登录/登出路径（可配置）
if strings.Contains(url, "logout") || strings.Contains(url, "login") {
    return
}
```

**理由**：
- ❌ 包含**业务规则**（login/logout 是业务路径）
- ❌ 每个项目的需求不同，不应硬编码
- ❌ API 项目已有自己的实现（`whitelist-api/common/middleware/logger.go`）
- ❌ 强依赖 Gin 框架

**正确做法**：
每个 API 项目根据自己的需求实现请求日志中间件，Core 只提供 Logger 接口。

---

#### 2. `sdk/pkg/logger/options.go`

**问题**：
- ⚠️ 这是**遗留代码**，与新的 `logger.Options` 功能重复
- ⚠️ 使用旧的字符串类型（`stdout string`）
- ⚠️ 维护成本高，容易混淆

**删除原因**：
已有更好的实现（`logger/options.go`），保留会导致混乱。

---

### ✅ 重构的文件（简化职责）

#### 3. `sdk/pkg/logger/log.go`

**修改前**：
```go
// 68 行代码，包含：
// - 创建目录
// - 选择输出方式
// - 选择适配器
// - 错误处理
func SetupLogger(opts ...Option) logger.Logger {
    // ... 68 行复杂逻辑
}
```

**修改后**：
```go
// 11 行代码，职责单一
func SetupLogger(opts ...logger.Option) logger.Logger {
    logger.DefaultLogger = logger.NewLogrusLogger(opts...)
    return logger.DefaultLogger
}
```

**改进**：
- ✅ 职责单一 - 只负责初始化
- ✅ 复用核心 - 使用 `logger` 包的能力
- ✅ 减少代码 - 从 68 行降到 11 行
- ✅ 标记废弃 - 建议直接使用 `logger.NewFromConfig`

---

#### 4. `sdk/config/logger.go`

**修改前**：
```go
func (e Logger) Setup() {
    logger.SetupLogger(
        logger.WithType(e.Type),    // 使用旧的选项
        logger.WithPath(e.Path),
        // ...
    )
}
```

**修改后**：
```go
func (e Logger) Setup() {
    // 确保目录存在
    if e.Path != "" && !pkg.PathExist(e.Path) {
        pkg.PathCreate(e.Path)
    }

    // 构建选项（使用新的 logger.Option）
    opts := []logger.Option{
        logger.WithName("go-admin"),
        logger.WithLevel(level),
        logger.WithStdout(true/false),
        logger.WithPath(e.Path),
    }

    // 选择适配器
    switch e.Type {
    case "zap":
        log.DefaultLogger = logger.NewZapLogger(opts...)
    default:
        log.DefaultLogger = logger.NewLogrusLogger(opts...)
    }
}
```

**改进**：
- ✅ 使用新的 Option 系统
- ✅ 支持新的配置（MaxSize、MaxAge、Compress 等）
- ✅ 保留配置绑定职责（这是 Core 应该做的）

---

## 职责边界总结

### ✅ Core 的正确职责

| 层级 | 职责 | 示例 |
|------|------|------|
| **接口层** | 定义通用抽象 | `Logger` 接口、`Storage` 接口 |
| **适配器层** | 封装第三方库 | `logrus.go`、`zap.go` |
| **工厂层** | 创建实例 | `NewLogrusLogger()`、`NewFromConfig()` |
| **配置层** | 绑定配置文件 | `sdk/config/logger.go` |
| **工具层** | 通用函数 | `GetLevel()`、字段序列化 |

### ❌ API 项目的职责

| 层级 | 职责 | 示例 |
|------|------|------|
| **业务中间件** | 包含业务规则的中间件 | 跳过特定路径的日志 |
| **业务配置** | 项目特定的配置 | 哪些路径需要跳过 |
| **上下文管理** | 请求级别的 Logger 管理 | `GetRequestLogger(c)` |
| **业务日志** | 业务流程日志 | "用户登录成功" |

---

## 验证结果

```bash
✅ 编译成功
✅ 所有测试通过
✅ 代码行数减少 100+ 行
✅ 职责更清晰
```

---

## 最终建议

### 1. Core 项目应该保持的文件
```
logger/
├── logger.go          ✅ 接口定义
├── logrus.go          ✅ Logrus 适配器
├── zap.go             ✅ Zap 适配器
├── default.go         ✅ 默认实现
├── config.go          ✅ 配置结构
├── factory.go         ✅ 工厂函数
├── sampling.go        ✅ 采样插件（通用）
├── field.go           ✅ 字段系统
├── level.go           ✅ 日志级别
└── options.go         ✅ 选项系统

sdk/pkg/logger/
└── log.go             ✅ 简化的初始化函数

sdk/config/
└── logger.go          ✅ 配置绑定层
```

### 2. API 项目应该实现的内容
```
common/middleware/
└── logger.go          ✅ 请求日志中间件（含业务规则）

common/apis/
└── request_logger.go  ✅ 上下文 Logger 管理
```

### 3. 未来的改进方向

#### 短期（建议）
- [ ] 删除 `sdk/pkg/logger/log.go`（标记为 Deprecated）
- [ ] 更新文档说明配置使用方式
- [ ] API 项目迁移到新的 Logger API

#### 中期（可选）
- [ ] 提供中间件**骨架**（不含业务规则）
  ```go
  // ✅ Core 提供骨架
  func NewRequestLoggerMiddleware(config Config) gin.HandlerFunc
  
  // ✅ API 项目实现业务规则
  middleware := core.NewRequestLoggerMiddleware(core.Config{
      SkipPaths: []string{"/login", "/logout"},  // 业务配置
  })
  ```

#### 长期（规划）
- [ ] 抽象中间件接口（支持多框架）
- [ ] 插件生态（让 API 项目可扩展）

---

## 总结

本次审阅清理了 **2 个不属于 Core 的文件**，重构了 **2 个职责不清的文件**，明确了 Core 和 API 项目的边界：

✅ **Core** - 提供基础设施、接口定义、适配器实现  
✅ **API** - 实现业务逻辑、业务规则、项目特定配置  

代码质量和职责划分现已符合企业级基础库的标准。
