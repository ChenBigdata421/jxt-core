# EventBus 测试策略文档

## 📋 测试架构概览

我们采用 **混合测试策略**，将测试分为两个层次：

### 🏗️ 测试架构图

```
eventbus/
├── internal_test.go          # 内部单元测试（白盒测试）
├── tests/                    # 外部集成测试（黑盒测试）
│   ├── envelope_test.go      # Envelope 公共接口测试
│   ├── eventbus_test.go      # EventBus 核心功能测试
│   ├── keyed_worker_pool_test.go # KeyedWorkerPool 集成测试
│   └── README.md            # 外部测试说明
└── TESTING_STRATEGY.md      # 本文档
```

## 🎯 测试分层策略

### 1. **内部单元测试** (`internal_test.go`)

**目标**：测试包内部的私有函数和核心算法逻辑

**特点**：
- ✅ 使用 `package eventbus` 声明
- ✅ 可以访问所有内部函数和私有类型
- ✅ 专注于算法正确性和边界条件
- ✅ 快速执行，无外部依赖

**测试覆盖**：
- `ExtractAggregateID()` - 聚合ID提取算法
- `validateAggregateID()` - 聚合ID验证逻辑
- `hashToIndex()` - 哈希路由算法
- `runWorker()` - Worker 运行逻辑
- 错误处理和边界条件

### 2. **外部集成测试** (`tests/`)

**目标**：测试公共接口的集成功能和用户使用场景

**特点**：
- ✅ 使用 `package eventbus_test` 声明
- ✅ 只能访问公共接口
- ✅ 模拟真实使用场景
- ✅ 验证端到端功能

**测试覆盖**：
- Envelope 创建、验证、序列化
- EventBus 发布订阅功能
- KeyedWorkerPool 顺序保证
- 配置管理和工厂模式
- 健康检查和监控

## 📊 测试覆盖分析

### 🔍 **内部函数重要性评估**

| 函数名 | 重要性 | 测试状态 | 说明 |
|--------|--------|----------|------|
| `ExtractAggregateID()` | 🔴 **高** | ✅ 已测试 | 业务关键，多种提取策略 |
| `validateAggregateID()` | 🔴 **高** | ✅ 已测试 | 数据完整性关键 |
| `hashToIndex()` | 🔴 **高** | ✅ 已测试 | 性能和一致性关键 |
| `runWorker()` | 🔴 **高** | ✅ 已测试 | 并发处理关键 |
| 消息格式化器内部逻辑 | 🟡 **中** | ⏳ 待补充 | 数据转换逻辑 |
| 积压检测算法 | 🟡 **中** | ⏳ 待补充 | 性能监控逻辑 |
| 健康检查内部逻辑 | 🟢 **低** | ⏳ 可选 | 通过公共接口覆盖 |

### 🎯 **测试优先级**

**P0 - 已完成**：
- ✅ 聚合ID提取和验证
- ✅ KeyedWorkerPool 核心算法
- ✅ 基本的发布订阅功能

**P1 - 建议补充**：
- ⏳ 消息格式化器详细测试
- ⏳ 积压检测算法测试
- ⏳ 错误恢复机制测试

**P2 - 可选**：
- ⏳ 性能基准测试
- ⏳ 并发压力测试
- ⏳ 内存泄漏测试

## 🚀 运行测试指南

### **运行所有测试**
```bash
# 运行内部单元测试
go test -v -run "^TestExtractAggregateID|^TestValidateAggregateID|^TestKeyedWorkerPool"

# 运行外部集成测试
go test ./tests/ -v

# 运行所有测试（包括内部和外部）
go test -v && go test ./tests/ -v
```

### **运行特定测试**
```bash
# 内部函数测试
go test -v -run TestExtractAggregateID
go test -v -run TestValidateAggregateID
go test -v -run TestKeyedWorkerPool_hashToIndex

# 外部接口测试
go test ./tests/ -v -run TestEnvelope
go test ./tests/ -v -run TestKeyedWorkerPool
```

### **性能测试**
```bash
# 运行基准测试（如果有）
go test -bench=. -benchmem

# 运行竞态检测
go test -race -v
```

## 📝 测试编写规范

### **内部测试规范**
1. **文件命名**：`internal_test.go`
2. **包声明**：`package eventbus`
3. **测试命名**：`TestFunctionName_Scenario`
4. **覆盖要点**：
   - 正常情况
   - 边界条件
   - 错误情况
   - 性能关键路径

### **外部测试规范**
1. **目录结构**：`tests/` 子目录
2. **包声明**：`package eventbus_test`
3. **导入要求**：必须导入 eventbus 包
4. **测试命名**：`TestPublicInterface_Scenario`
5. **覆盖要点**：
   - 用户使用场景
   - 接口契约验证
   - 集成功能测试
   - 配置和错误处理

## 🔄 测试维护策略

### **新增功能时**
1. **内部函数**：在 `internal_test.go` 中添加单元测试
2. **公共接口**：在 `tests/` 中添加集成测试
3. **确保覆盖**：新功能必须有对应测试

### **重构代码时**
1. **保持测试通过**：重构不应破坏现有测试
2. **更新测试**：如果接口变化，同步更新测试
3. **验证覆盖**：确保重构后的代码仍有测试覆盖

### **性能优化时**
1. **基准测试**：添加性能基准测试
2. **回归验证**：确保优化不破坏功能
3. **监控指标**：关注关键性能指标

## 🎯 总结

我们的混合测试策略提供了：

✅ **完整覆盖**：内部算法 + 外部接口双重保障
✅ **快速反馈**：单元测试快速定位问题
✅ **真实验证**：集成测试验证用户场景
✅ **维护友好**：清晰的分层和命名规范
✅ **扩展性强**：易于添加新的测试用例

这种策略确保了 EventBus 组件的高质量和可靠性！
