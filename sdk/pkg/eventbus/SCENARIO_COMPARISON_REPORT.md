# 🎯 EventBus架构方案对比测试报告

## 📋 测试概述

本报告对比了两种EventBus架构方案在混合业务场景下的性能表现：

- **业务A**：需要使用Envelope+Keyed-Worker池，确保同一聚合ID的事件顺序处理
- **业务B**：普通的发布与订阅事件处理，无顺序要求

### 测试环境
- **消息中间件**: NATS JetStream
- **测试规模**: 3,000个订单事件 + 6,000个通知事件 = 9,000条消息
- **测试平台**: Linux x86_64
- **Go版本**: 1.24.7

## 🏗️ 方案架构对比

### 方案一：独立EventBus实例
```go
// 业务A：订单处理（Envelope + Keyed-Worker池）
orderBus := eventbus.NewEventBus(&eventbus.EventBusConfig{
    Type: "nats",
    NATS: eventbus.NATSConfig{
        URL: "nats://localhost:4222",
        JetStream: eventbus.JetStreamConfig{
            Enabled: true,
            Stream: eventbus.StreamConfig{
                Name: "scenario1-orders-stream",
                Subjects: []string{"scenario1.orders.*"},
            },
        },
    },
    Enterprise: eventbus.EnterpriseConfig{
        Subscriber: eventbus.SubscriberEnterpriseConfig{
            KeyedWorkerPool: eventbus.KeyedWorkerPoolConfig{
                Enabled: true,
                WorkerCount: 16,
                QueueSize: 1000,
            },
        },
    },
})

// 业务B：通知处理（普通发布订阅）
notificationBus := eventbus.NewEventBus(&eventbus.EventBusConfig{
    Type: "nats",
    NATS: eventbus.NATSConfig{
        URL: "nats://localhost:4222",
        JetStream: eventbus.JetStreamConfig{
            Enabled: true,
            Stream: eventbus.StreamConfig{
                Name: "scenario1-notifications-stream",
                Subjects: []string{"scenario1.notifications.*"},
            },
        },
    },
    Enterprise: eventbus.EnterpriseConfig{
        Subscriber: eventbus.SubscriberEnterpriseConfig{
            KeyedWorkerPool: eventbus.KeyedWorkerPoolConfig{
                Enabled: false, // 禁用Keyed-Worker池
            },
        },
    },
})
```

### 方案二：单一EventBus实例
```go
// 统一EventBus实例
unifiedBus := eventbus.NewEventBus(&eventbus.EventBusConfig{
    Type: "nats",
    NATS: eventbus.NATSConfig{
        URL: "nats://localhost:4222",
        JetStream: eventbus.JetStreamConfig{
            Enabled: true,
            Stream: eventbus.StreamConfig{
                Name: "unified-stream-scenario2",
                Subjects: []string{"scenario2.orders.*", "scenario2.notifications.*"},
            },
        },
    },
    Enterprise: eventbus.EnterpriseConfig{
        Subscriber: eventbus.SubscriberEnterpriseConfig{
            KeyedWorkerPool: eventbus.KeyedWorkerPoolConfig{
                Enabled: true,
                WorkerCount: 16,
                QueueSize: 1000,
            },
        },
    },
})

// 业务A：使用SubscribeEnvelope方法（自动路由到Keyed-Worker池）
unifiedBus.SubscribeEnvelope(ctx, "scenario2.orders.events", orderHandler)

// 业务B：使用Subscribe方法（直接处理，无Keyed-Worker池）
unifiedBus.Subscribe(ctx, "scenario2.notifications.events", notificationHandler)
```

## 📊 性能测试结果

### 综合性能对比

| 指标 | 方案一（独立实例） | 方案二（单一实例） | 差异 |
|------|-------------------|-------------------|------|
| **总吞吐量** | 1,192.09 msg/s | 1,173.69 msg/s | -1.54% |
| **内存使用** | 4.15 MB | 3.63 MB | **-12.65%** ✅ |
| **协程数量** | 16 | 15 | **-6.25%** ✅ |
| **EventBus实例** | 2 | 1 | **-50.00%** ✅ |
| **GC次数** | 18 | 35 | +94.44% |
| **总处理量** | 9,000 条 | 9,000 条 | 0% |
| **错误数量** | 0 | 0 | 0% |

### 分业务性能对比

#### 订单处理性能（业务A）
| 指标 | 方案一 | 方案二 | 差异 |
|------|--------|--------|------|
| **吞吐量** | 397.36 msg/s | 391.23 msg/s | -1.54% |
| **处理方式** | 独立EventBus + Envelope | 统一EventBus + SubscribeEnvelope |
| **顺序保证** | ✅ Keyed-Worker池 | ✅ Keyed-Worker池 |

#### 通知处理性能（业务B）
| 指标 | 方案一 | 方案二 | 差异 |
|------|--------|--------|------|
| **吞吐量** | 794.73 msg/s | 782.46 msg/s | -1.55% |
| **处理方式** | 独立EventBus + 普通订阅 | 统一EventBus + 普通订阅 |
| **顺序保证** | ❌ 无要求 | ❌ 无要求 |

## 🔍 基准测试结果

### 方案二基准测试详情
```
BenchmarkScenarioTwo_PublishNotification-24    25615    50320 ns/op    1975 B/op    31 allocs/op
```

- **操作延迟**: 50.32 µs/op
- **内存分配**: 1,975 B/op
- **分配次数**: 31 allocs/op
- **测试次数**: 25,615 次

## 🏆 综合评估

### 方案优势对比

#### 方案一优势
- ✅ **架构隔离**: 业务完全独立，故障隔离性好
- ✅ **配置灵活**: 每个业务可独立配置优化
- ✅ **GC压力小**: GC次数较少（18次 vs 35次）

#### 方案二优势
- ✅ **资源高效**: 内存节省12.65%，协程减少6.25%
- ✅ **架构简洁**: 单一实例，减少50%的EventBus管理复杂度
- ✅ **运维友好**: 统一监控、配置和故障处理
- ✅ **成本效益**: 减少连接数和资源占用

### 性能分析

1. **吞吐量**: 两方案性能相近（差异仅1.54%），在可接受范围内
2. **内存效率**: 方案二显著优于方案一，节省12.65%内存
3. **资源利用**: 方案二协程数更少，资源利用更高效
4. **GC影响**: 方案一GC次数较少，但差异不显著

## 🎯 推荐结论

**强烈推荐方案二：单一EventBus实例 + 不同方法**

### 推荐理由

1. **🏗️ 架构优雅**: jxt-core的EventBus设计已完美支持混合场景
   - `SubscribeEnvelope` 自动路由到Keyed-Worker池
   - `Subscribe` 直接处理，无额外开销

2. **💰 成本效益**: 
   - 内存节省12.65%
   - 协程减少6.25%
   - EventBus实例减少50%

3. **🔧 运维优势**:
   - 统一配置管理
   - 统一监控指标
   - 简化故障排查

4. **📈 性能表现**:
   - 吞吐量损失微乎其微（1.54%）
   - 延迟表现优秀（50.32 µs/op）
   - 内存分配合理（1,975 B/op）

5. **🔄 扩展性**:
   - 新业务场景可灵活选择处理模式
   - 统一的消息路由和处理框架
   - 便于未来功能扩展

### 实施建议

1. **配置优化**: 根据实际负载调整Worker池大小
2. **监控完善**: 添加业务级别的监控指标
3. **测试验证**: 在生产环境进行渐进式部署验证
4. **文档更新**: 更新架构文档和最佳实践指南

## 📝 总结

方案二在保持相近性能的同时，显著提升了资源利用效率和架构简洁性。jxt-core的EventBus架构设计使得单一实例能够优雅地处理混合业务场景，是现代事件驱动架构的最佳实践。

**最终推荐**: 采用方案二（单一EventBus实例 + 不同方法）作为生产环境的标准架构。
