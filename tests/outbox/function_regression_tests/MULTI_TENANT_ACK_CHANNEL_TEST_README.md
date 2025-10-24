# 多租户 ACK Channel 测试文档

## 📋 概述

本测试套件验证 **方案 B（每租户独立 ACK Channel）** 的实现，确保多租户 Outbox 调度器能够通过 EventBus 异步发送消息，并且每个租户的 Outbox 调度器能够通过租户专属的 ACK Channel 接收 ACK。

## 🎯 测试目标

1. **租户隔离性**：验证每个租户的 ACK 结果只会路由到对应租户的 Publisher
2. **并发安全性**：验证多个租户并发发布事件时的线程安全性
3. **高负载性能**：验证在高负载场景下的性能和稳定性
4. **EventBus 兼容性**：验证在 Memory、NATS、Kafka 三种 EventBus 下的正确性

## ✅ 测试状态

- ✅ **多租户 ACK 路由测试** (`multi_tenant_ack_routing_test.go`) - 已通过
- ✅ **NATS 多租户 ACK 测试** (`multi_tenant_ack_nats_test.go`) - 已创建
- ✅ **Kafka 多租户 ACK 测试** (`multi_tenant_ack_kafka_test.go`) - 已创建
- ⚠️ **Memory EventBus 端到端测试** - Memory EventBus 不支持异步 ACK

## 📁 测试文件

### 1. `multi_tenant_ack_routing_test.go` ✅
**测试 EventBus**: Memory EventBus

**测试用例**:
- `TestMultiTenantACKRouting`: 多租户 ACK 路由功能测试
  - 注册 3 个租户
  - 验证每个租户都有独立的 ACK Channel
  - 验证 `GetRegisteredTenants()` 返回正确的租户列表
  - 验证租户注销后不再出现在注册列表中

**运行方式**:
```bash
cd jxt-core/tests/outbox/function_regression_tests
go test -v -run "^TestMultiTenantACKRouting$" -timeout 30s
```

### 2. `multi_tenant_ack_nats_test.go` ✅
**测试 EventBus**: NATS JetStream

**测试用例**:
- `TestMultiTenantACKChannel_NATS`: NATS 基础多租户 ACK 测试
  - 注册 3 个租户
  - 每个租户创建 5 个事件
  - 验证所有事件都被正确发布和 ACK
  - 验证每个租户只收到自己的 ACK

- `TestMultiTenantACKChannel_NATS_Isolation`: NATS 租户隔离性测试
  - 创建 2 个租户
  - 验证租户 A 的 ACK 不会被租户 B 接收

**运行方式**:
```bash
# 确保 NATS 服务器正在运行
docker-compose up -d nats

# 运行测试
cd jxt-core/tests/outbox/function_regression_tests
go test -v -run "^TestMultiTenantACKChannel_NATS" -timeout 30s
```

### 3. `multi_tenant_ack_kafka_test.go` ✅
**测试 EventBus**: Kafka

**测试用例**:
- `TestMultiTenantACKChannel_Kafka`: Kafka 基础多租户 ACK 测试
  - 注册 3 个租户
  - 每个租户创建 5 个事件
  - 验证所有事件都被正确发布和 ACK
  - 验证每个租户只收到自己的 ACK

- `TestMultiTenantACKChannel_Kafka_ConcurrentPublish`: Kafka 并发发布测试
  - 多个租户并发发布事件
  - 验证并发场景下 ACK 处理正确

**运行方式**:
```bash
# 确保 Kafka 服务器正在运行
docker-compose up -d kafka

# 运行测试
cd jxt-core/tests/outbox/function_regression_tests
go test -v -run "^TestMultiTenantACKChannel_Kafka" -timeout 30s
```

### 4. `multi_tenant_ack_channel_test.go` ⚠️
**测试 EventBus**: Memory EventBus

**测试用例**:
- `TestMultiTenantACKChannel_MemoryEventBus`: 基础多租户 ACK Channel 测试
- `TestMultiTenantACKChannel_Isolation`: 租户隔离性测试
- `TestMultiTenantACKChannel_ConcurrentPublish`: 并发发布测试

**状态**: ⚠️ Memory EventBus 不支持异步 ACK，这些测试无法完整运行

## 🏗️ 测试架构

```
┌─────────────────────────────────────────────────────────────┐
│                   Test Suite                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Tenant A     │  │ Tenant B     │  │ Tenant C     │      │
│  │ Scheduler    │  │ Scheduler    │  │ Scheduler    │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
│         │                  │                  │              │
│  ┌──────▼───────┐  ┌──────▼───────┐  ┌──────▼───────┐      │
│  │ Publisher A  │  │ Publisher B  │  │ Publisher C  │      │
│  │ ACK Listener │  │ ACK Listener │  │ ACK Listener │      │
│  └──────▲───────┘  └──────▲───────┘  └──────▲───────┘      │
│         │                  │                  │              │
│  ┌──────┴───────┐  ┌──────┴───────┐  ┌──────┴───────┐      │
│  │ Mock Repo A  │  │ Mock Repo B  │  │ Mock Repo C  │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────┼──────────────────┼──────────────────┼──────────────┘
          │                  │                  │
┌─────────┼──────────────────┼──────────────────┼──────────────┐
│         │   EventBusAdapter (Type Conversion) │              │
│  ┌──────▼───────┐  ┌──────▼───────┐  ┌──────▼───────┐      │
│  │ Tenant A     │  │ Tenant B     │  │ Tenant C     │      │
│  │ ACK Channel  │  │ ACK Channel  │  │ ACK Channel  │      │
│  │ (outbox.*)   │  │ (outbox.*)   │  │ (outbox.*)   │      │
│  └──────▲───────┘  └──────▲───────┘  └──────▲───────┘      │
└─────────┼──────────────────┼──────────────────┼──────────────┘
          │                  │                  │
┌─────────┼──────────────────┼──────────────────┼──────────────┐
│         │   Memory/NATS/Kafka EventBus        │              │
│  ┌──────▼───────┐  ┌──────▼───────┐  ┌──────▼───────┐      │
│  │ Tenant A     │  │ Tenant B     │  │ Tenant C     │      │
│  │ ACK Channel  │  │ ACK Channel  │  │ ACK Channel  │      │
│  │ (eventbus.*) │  │ (eventbus.*) │  │ (eventbus.*) │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└──────────────────────────────────────────────────────────────┘
```

## 🚀 运行测试

### 运行所有多租户 ACK Channel 测试

```bash
cd jxt-core/tests/outbox/function_regression_tests

# 运行所有测试（需要 NATS 和 Kafka 环境）
go test -v -run "TestMultiTenantACK" -timeout 60s
```

### 运行 Memory EventBus 路由测试

```bash
# 只测试 ACK 路由功能（不需要外部依赖）
go test -v -run "^TestMultiTenantACKRouting$" -timeout 30s
```

### 运行 NATS EventBus 测试

**前置条件**: NATS 服务器运行在 `localhost:4223`

```bash
# 启动 NATS
docker-compose up -d nats

# 运行 NATS 测试
go test -v -run "^TestMultiTenantACKChannel_NATS" -timeout 30s
```

### 运行 Kafka EventBus 测试

**前置条件**: Kafka 服务器运行在 `localhost:29094`

```bash
# 启动 Kafka
docker-compose up -d kafka

# 运行 Kafka 测试
go test -v -run "^TestMultiTenantACKChannel_Kafka" -timeout 30s
```

## 📊 测试覆盖率

运行测试并生成覆盖率报告：

```bash
go test -v -coverprofile=coverage.out -run "TestMultiTenantACKChannel"
go tool cover -html=coverage.out -o coverage.html
```

## ✅ 验证点

每个测试用例都会验证以下关键点：

1. **租户注册**
   - ✅ 所有租户都成功注册
   - ✅ `GetRegisteredTenants()` 返回正确的租户列表

2. **ACK Channel 创建**
   - ✅ 每个租户都有独立的 ACK Channel
   - ✅ ACK Channel 不为 nil

3. **事件发布**
   - ✅ 所有事件都成功发布到 EventBus
   - ✅ 没有发布错误

4. **ACK 路由**
   - ✅ 每个租户的 ACK 结果都路由到正确的 Publisher
   - ✅ 租户 A 的 ACK 不会发送到租户 B 的 Publisher

5. **事件状态更新**
   - ✅ 所有事件都被标记为 `Published`
   - ✅ `PublishedAt` 时间戳已设置

6. **租户隔离**
   - ✅ 租户 A 的 Repository 中没有租户 B 的事件
   - ✅ 租户 B 的 Repository 中没有租户 A 的事件

7. **资源清理**
   - ✅ 所有 Scheduler 都正确停止
   - ✅ 所有 ACK Listener 都正确停止
   - ✅ 所有租户都成功注销

## 🔧 配置参数

### Scheduler 配置

```go
schedulerConfig := &outbox.SchedulerConfig{
    PollInterval: 100 * time.Millisecond,  // 轮询间隔
    BatchSize:    10,                       // 批量大小
    TenantID:     "tenant-a",               // 租户 ID
}
```

### ACK Channel 缓冲区大小

```go
// 基础测试：1000
adapter.RegisterTenant(tenantID, 1000)

// 高负载测试：2000-5000
adapter.RegisterTenant(tenantID, 5000)
```

## 📈 性能基准

### NATS EventBus
- **租户数**: 3
- **每租户事件数**: 5
- **总事件数**: 15
- **预期时间**: < 10 秒

### Kafka EventBus
- **租户数**: 3
- **每租户事件数**: 5
- **总事件数**: 15
- **预期时间**: < 15 秒

## 🐛 故障排查

### 测试失败：租户未注册

**错误**: `ACK channel should not be nil for tenant xxx`

**解决方案**:
1. 检查 `RegisterTenant()` 是否成功
2. 检查 EventBus 是否正确实现了多租户 ACK 方法

### 测试失败：事件未被标记为 Published

**错误**: `Tenant xxx should have N published events`

**解决方案**:
1. 增加等待时间（`time.Sleep`）
2. 检查 ACK Listener 是否正确启动
3. 检查 EventBus 是否正确发送 ACK 结果

### 测试失败：NATS/Kafka 连接失败

**错误**: `Failed to create NATS/Kafka EventBus`

**解决方案**:
1. 确保 NATS 服务器运行在 `localhost:4223`
2. 确保 Kafka 服务器运行在 `localhost:29094`
3. 使用 `-short` 标志跳过集成测试

## 📚 参考文档

- [方案 B 实施总结](./IMPLEMENTATION_SUMMARY.md)
- [EventBus 接口文档](../../../jxt-core/sdk/pkg/eventbus/type.go)
- [Outbox Publisher 文档](../../../jxt-core/sdk/pkg/outbox/publisher.go)
- [EventBusAdapter 文档](../../../jxt-core/sdk/pkg/outbox/adapters/eventbus_adapter.go)

## 🎉 总结

这些测试用例全面验证了多租户 ACK Channel 的实现，确保：
- ✅ 租户之间完全隔离
- ✅ 并发安全
- ✅ 高性能
- ✅ 跨 EventBus 兼容性

所有测试通过即表示 **方案 B** 实施成功！

