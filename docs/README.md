# jxt-core 文档中心

## 概述

欢迎来到 jxt-core 文档中心。这里包含了 jxt-core 项目的完整文档，包括架构设计、实施方案、API 参考和最佳实践。

## 🎉 重构完成状态

**✅ jxt-core EventBus 重构已完成！**

- ✅ **75% 功能成功迁移**：技术基础设施统一实现
- ✅ **统一健康检查**：100% 由 jxt-core 负责（主题、消息、验证）
- ✅ **高级事件总线**：支持企业级功能（积压检测、恢复模式、聚合处理器）
- ✅ **业务兼容性**：通过适配器保持现有业务代码兼容
- ✅ **可扩展架构**：支持多种事件总线实现和业务组件

**📖 查看详细信息**：[重构完成总结](./refactoring-completion-summary.md)

## 文档结构

### 🎯 重构完成文档

#### [重构完成总结](./refactoring-completion-summary.md) ⭐ **最新状态**
**重构完成报告**：详细的重构完成情况，包括：
- ✅ 已完成功能清单
- ✅ 核心设计目标达成情况
- ✅ 重构效果和性能提升
- ✅ 使用方式和示例代码

### 📋 原始实施方案

#### [完整实施方案总结](./complete-implementation-summary.md) ⭐
**推荐首先阅读**：完整的订阅端和发布端实施方案总结，包括：
- 整体功能迁移分析（75% 可迁移到 jxt-core）
- 统一架构设计和分层图
- 6周完整实施计划
- 配置示例和预期收益

#### [高级事件总线实施方案](./advanced-eventbus-implementation-plan.md)
订阅端详细实施方案，包括：
- 订阅端架构设计
- 积压检测和恢复模式
- 分阶段实施计划
- 风险评估和缓解措施

#### [高级事件发布器实施方案](./advanced-publisher-implementation-plan.md)
发布端详细实施方案，包括：
- 发布端功能分析和迁移策略
- 统一健康检查设计
- 消息格式化器设计
- 业务层适配方案

#### [代码实现示例](./advanced-eventbus-code-examples.md)
订阅端详细的代码实现示例，包括：
- 聚合处理器管理器实现
- 恢复模式管理器实现
- Kafka 积压检查器实现
- 默认组件实现
- 业务层使用示例

#### [发布端代码实现示例](./advanced-publisher-code-examples.md)
发布端详细的代码实现示例，包括：
- 统一健康检查消息和检查器实现
- 高级发布管理器实现
- 消息格式化器实现
- 业务层适配器实现

#### [订阅端迁移指南](./migration-guide.md)
从现有订阅端系统迁移到新架构的详细指南，包括：
- 迁移前准备工作
- 分阶段迁移计划
- 配置和代码更新
- 测试策略
- 回滚计划

#### [发布端迁移指南](./publisher-migration-guide.md)
从现有发布端系统迁移到新架构的详细指南，包括：
- KafkaPublisherManager 迁移步骤
- 统一健康检查迁移
- 业务适配器创建
- 配置更新和测试验证

### 📖 业务使用指南

#### [业务微服务使用指南](./business-microservice-usage-guide.md) ⭐ **业务开发必读**
完整的业务微服务使用统一 EventBus 指南，包括：
- **发布侧使用示例**：创建业务发布器、事件发布、消息格式化
- **订阅侧使用示例**：创建业务订阅器、事件处理器、回调管理
- **企业特性配置**：灵活启用/禁用高级功能
- **最佳实践**：错误处理策略、性能优化、监控告警、配置管理
- **完整示例**：从配置到启动的完整业务服务代码

### 🔧 技术参考

#### [Outbox 模式快速开始](./outbox-pattern-quick-start.md) ⭐ **5 分钟上手**
快速集成 Outbox 模式到你的微服务，包括：
- **四步集成**：定义 Topic 映射、创建 EventBus 适配器、初始化 Outbox、业务代码使用
- **完整示例**：可直接运行的代码示例
- **验证集成**：检查数据库、日志、监控指标
- **最佳实践**：事件设计、幂等性处理、定期清理
- **常见问题**：快速排查和解决方案

#### [Outbox 模式设计方案](./outbox-pattern-design.md) ⭐ **完整文档**
完整的 Outbox 模式通用框架设计，包括：
- **整体架构**：分层设计和组件交互
- **核心组件**：OutboxEvent、Repository、EventPublisher、Publisher、Scheduler
- **依赖注入设计**：EventPublisher 接口和适配器模式
- **数据库适配器**：GORM 适配器和扩展指南
- **使用指南**：业务微服务集成步骤
- **配置说明**：调度器和发布器配置
- **事件处理流程**：完整的事件生命周期
- **多租户支持**：租户隔离和查询
- **监控运维**：健康检查、指标收集、事件清理
- **最佳实践**：事件设计、性能优化、错误处理
- **故障排查**：常见问题和解决方案

#### [Outbox 模式依赖注入设计](./outbox-pattern-dependency-injection-design.md) ⭐ **架构设计**
详细说明 Outbox 模式的依赖注入设计，包括：
- **设计原理**：依赖倒置原则（DIP）
- **核心组件**：EventPublisher 接口、OutboxPublisher、EventBusAdapter
- **使用方式**：业务微服务集成示例
- **设计优势**：符合 SOLID 原则、零外部依赖、易于测试、灵活扩展
- **对比分析**：直接依赖 vs 依赖注入
- **最佳实践**：适配器命名、位置、错误处理、日志记录

#### [EventBus 基础文档](../sdk/pkg/eventbus/README.md)
基础事件总线的使用文档，包括：
- 基本概念和架构
- 支持的实现（Kafka、NATS、Memory）
- 配置说明
- 使用示例
- 健康检查机制

## 快速开始

### 1. 基础事件总线使用

```go
import "jxt-core/sdk/pkg/eventbus"

// 创建基础事件总线
config := eventbus.GetDefaultEventBusConfig()
config.Type = "kafka"
config.Kafka.Brokers = []string{"localhost:9092"}

bus, err := eventbus.NewEventBus(config)
if err != nil {
    log.Fatal(err)
}

// 发布消息
err = bus.Publish(context.Background(), "my-topic", []byte("hello world"))

// 订阅消息
err = bus.Subscribe(context.Background(), "my-topic", func(ctx context.Context, message []byte) error {
    fmt.Printf("Received: %s\n", string(message))
    return nil
})
```

### 2. 业务微服务使用（推荐）

```go
import "jxt-core/sdk/pkg/eventbus"

// 创建统一事件总线
config := &eventbus.EventBusConfig{
    Type: "kafka",
    Kafka: eventbus.KafkaConfig{
        Brokers: []string{"localhost:9092"},
    },
    Enterprise: eventbus.EnterpriseConfig{
        Publisher: eventbus.PublisherEnterpriseConfig{
            RetryPolicy: eventbus.RetryPolicyConfig{
                Enabled: true,
                MaxRetries: 3,
            },
        },
        Subscriber: eventbus.SubscriberEnterpriseConfig{
            AggregateProcessor: eventbus.AggregateProcessorConfig{
                Enabled: true,
            },
            RateLimit: eventbus.RateLimitConfig{
                Enabled: true,
                RateLimit: 100,
            },
        },
    },
}

bus, err := eventbus.NewEventBus(config)
if err != nil {
    log.Fatal(err)
}

// 启动事件总线
err = bus.Start(context.Background())

// 发布业务事件
opts := eventbus.PublishOptions{
    AggregateID: "user-123",
    Metadata: map[string]string{"eventType": "UserCreated"},
    Timeout: 30 * time.Second,
}
err = bus.PublishWithOptions(ctx, "user-events", eventData, opts)

// 订阅业务事件
subscribeOpts := eventbus.SubscribeOptions{
    UseAggregateProcessor: true,
    RateLimit: 100,
}
err = bus.SubscribeWithOptions(ctx, "user-events", handleUserEvent, subscribeOpts)

// 监控状态
metrics := bus.GetMetrics()
log.Printf("Messages processed: %d", metrics.MessagesConsumed)
```

**📖 详细使用方法**：[业务微服务使用指南](./business-microservice-usage-guide.md)



// 高级订阅
subscribeOpts := eventbus.SubscribeOptions{
    UseAggregateProcessor: true,
    ProcessingTimeout:     30 * time.Second,
    RateLimit:            1000,
    RateBurst:            1000,
}
err = bus.SubscribeWithOptions(context.Background(), "my-topic", handler, subscribeOpts)

// 高级发布
publishOpts := eventbus.PublishOptions{
    AggregateID: "aggregate-123",
    Timeout:     30 * time.Second,
    Metadata: map[string]string{
        "eventType": "MyEvent",
    },
}
err = bus.PublishWithOptions(context.Background(), "my-topic", payload, publishOpts)
```

## 架构概览

### 分层架构

```
┌─────────────────────────────────────────┐
│           业务微服务层                    │
│  ┌─────────────────────────────────────┐ │
│  │  - 业务消息处理逻辑                  │ │
│  │  - 业务特定配置                     │ │
│  │  - 业务错误处理策略                 │ │
│  │  - 业务回调处理                     │ │
│  └─────────────────────────────────────┘ │
└─────────────────────────────────────────┘
                    │ 接口调用
                    ▼
┌─────────────────────────────────────────┐
│            jxt-core 层                  │
│  ┌─────────────────────────────────────┐ │
│  │      高级事件总线                    │ │
│  │   - 积压检测                        │ │
│  │   - 恢复模式管理                    │ │
│  │   - 聚合处理器框架                  │ │
│  │   - 流量控制                        │ │
│  │   - 健康检查                        │ │
│  │   - 重连机制                        │ │
│  └─────────────────────────────────────┘ │
│  ┌─────────────────────────────────────┐ │
│  │      基础事件总线                    │ │
│  │   - Kafka 实现                      │ │
│  │   - NATS 实现                       │ │
│  │   - Memory 实现                     │ │
│  └─────────────────────────────────────┘ │
└─────────────────────────────────────────┘
```

### 核心组件

#### 基础组件
1. **EventBus 接口**：基础事件总线接口
2. **AdvancedEventBus 接口**：高级事件总线接口

#### 订阅端组件
3. **BacklogDetector**：积压检测器
4. **RecoveryManager**：恢复模式管理器
5. **AggregateProcessorManager**：聚合处理器管理器
6. **MessageRouter**：消息路由器接口
7. **ErrorHandler**：错误处理器接口

#### 发布端组件
8. **AdvancedPublisher**：高级发布管理器
9. **MessageFormatter**：消息格式化器接口
10. **PublishCallback**：发布回调接口

#### 统一组件
11. **HealthChecker**：统一健康检查器
12. **HealthCheckMessage**：标准健康检查消息
13. **ReconnectCallback**：重连回调接口

## 功能特性

### ✅ 已实现功能

- **多种实现支持**：Kafka、NATS、Memory
- **基础发布订阅**：简单的消息发布和订阅
- **健康检查**：连接状态监控
- **重连机制**：自动重连和回调通知
- **配置管理**：灵活的配置系统

### 🚧 开发中功能（高级事件总线）

#### 订阅端功能
- **积压检测**：自动检测消息积压并通知
- **恢复模式**：积压时的有序消息处理
- **聚合处理器**：按聚合ID分组处理消息
- **流量控制**：令牌桶限流机制
- **错误处理**：可配置的错误处理策略
- **死信队列**：失败消息的处理机制

#### 发布端功能
- **高级发布选项**：支持元数据、超时、重试策略
- **消息格式化**：可插拔的消息格式化器
- **发布回调**：发布结果通知机制
- **重连管理**：发布端连接管理和故障恢复

#### 统一功能
- **统一健康检查**：标准化的健康检查主题和消息格式
- **服务标识**：基于服务名的健康检查和监控
- **回调机制**：统一的重连和状态变化回调

### 🔮 计划功能

- **分布式追踪**：集成 OpenTelemetry
- **指标收集**：Prometheus 指标
- **消息压缩**：支持消息压缩
- **消息加密**：端到端加密
- **Schema 注册**：消息格式管理

## 贡献指南

### 开发环境设置

```bash
# 克隆项目
git clone <repository-url>
cd jxt-core

# 安装依赖
go mod tidy

# 运行测试
go test ./...

# 运行示例
go run examples/basic/main.go
```

### 代码规范

1. **遵循 Go 代码规范**
2. **编写单元测试**：测试覆盖率不低于 80%
3. **添加文档注释**：所有公开接口都要有文档
4. **更新文档**：代码变更时同步更新文档

### 提交流程

1. **创建功能分支**：`git checkout -b feature/your-feature`
2. **编写代码和测试**
3. **运行测试**：`go test ./...`
4. **提交代码**：`git commit -m "feat: your feature description"`
5. **创建 Pull Request**

## 版本历史

### v1.0.0（当前版本）
- 基础事件总线实现
- Kafka、NATS、Memory 支持
- 健康检查机制
- 重连机制

### v2.0.0（计划中）
- 高级事件总线功能
- 积压检测和恢复模式
- 聚合处理器
- 流量控制

## 支持和反馈

### 问题报告
如果您发现 bug 或有功能请求，请在 GitHub Issues 中提交。

### 技术支持
- **文档问题**：查看相关文档或提交 Issue
- **使用问题**：参考示例代码或联系开发团队
- **性能问题**：提供详细的性能测试报告

### 社区
- **讨论**：GitHub Discussions
- **更新**：关注项目 Release 页面

## 许可证

本项目采用 MIT 许可证，详见 [LICENSE](../LICENSE) 文件。

---

**最后更新**：2024-12-19  
**文档版本**：v1.0.0
