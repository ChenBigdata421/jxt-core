# Outbox Pattern 通用框架

## 📋 概述

Outbox 模式是一种用于确保分布式系统中数据一致性的设计模式。本框架提供了一个通用的、可复用的 Outbox 实现，可以被所有微服务使用。

## 🎯 核心特性

- ✅ **数据库无关**：领域模型完全独立于数据库实现
- ✅ **依赖注入**：通过 EventPublisher 接口实现依赖倒置
- ✅ **零外部依赖**：核心包不依赖任何外部库
- ✅ **多租户支持**：内置租户隔离
- ✅ **自动重试**：支持失败事件自动重试
- ✅ **定时发布**：支持延迟发布
- ✅ **自动清理**：自动清理已发布的事件
- ✅ **健康检查**：内置健康检查机制
- ✅ **指标收集**：支持指标收集和监控

## 🏗️ 架构设计

### 核心组件

```
┌─────────────────────────────────────────────────────────┐
│              jxt-core/sdk/pkg/outbox/                   │
├─────────────────────────────────────────────────────────┤
│  ✅ 1. OutboxEvent（领域模型）                           │
│  ✅ 2. OutboxRepository（仓储接口）                      │
│  ✅ 3. EventPublisher（事件发布接口）⭐                  │
│  ✅ 4. OutboxPublisher（发布器）                         │
│  ✅ 5. OutboxScheduler（调度器）                         │
│  ✅ 6. TopicMapper（Topic 映射）                         │
│                                                         │
│  ✅ 7. GORM 适配器（adapters/gorm/）                    │
│     - OutboxEventModel（数据库模型）                    │
│     - GormOutboxRepository（仓储实现）                  │
└─────────────────────────────────────────────────────────┘
```

### 依赖注入设计

```
业务微服务
    │
    ├─ 创建 EventBus（jxt-core）
    │
    ├─ 创建 EventBusAdapter（实现 EventPublisher 接口）⭐
    │
    └─ 注入到 OutboxScheduler
           │
           └─ OutboxPublisher（依赖 EventPublisher 接口）
```

## 🚀 快速开始

### 步骤 1：定义 TopicMapper

```go
// internal/outbox/topics.go
package outbox

import (
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

func NewTopicMapper() outbox.TopicMapper {
    return outbox.NewMapBasedTopicMapper(map[string]string{
        "Archive": "archive-events",
        "Media":   "media-events",
    }, "default-events")
}
```

### 步骤 2：创建 EventBus 适配器 ⭐

```go
// internal/outbox/eventbus_adapter.go
package outbox

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

type EventBusAdapter struct {
    eventBus eventbus.EventBus
}

func NewEventBusAdapter(eventBus eventbus.EventBus) outbox.EventPublisher {
    return &EventBusAdapter{eventBus: eventBus}
}

func (a *EventBusAdapter) Publish(ctx context.Context, topic string, data []byte) error {
    return a.eventBus.Publish(ctx, topic, data)
}
```

### 步骤 3：初始化 Outbox

```go
// cmd/main.go
package main

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
    localoutbox "your-service/internal/outbox"
)

func main() {
    ctx := context.Background()
    
    // 1. 获取数据库
    db := sdk.Runtime.GetDB()
    
    // 2. 自动迁移（创建表）
    db.AutoMigrate(&gormadapter.OutboxEventModel{})
    
    // 3. 创建仓储
    outboxRepo := gormadapter.NewGormOutboxRepository(db)
    
    // 4. 创建 EventBus 适配器 ⭐
    eventBus := sdk.Runtime.GetEventBus()
    eventPublisher := localoutbox.NewEventBusAdapter(eventBus)
    
    // 5. 创建并启动调度器
    scheduler := outbox.NewScheduler(
        outbox.WithRepository(outboxRepo),
        outbox.WithEventPublisher(eventPublisher),  // 注入 EventPublisher ⭐
        outbox.WithTopicMapper(localoutbox.NewTopicMapper()),
    )
    
    // 6. 启动调度器
    if err := scheduler.Start(ctx); err != nil {
        panic(err)
    }
    defer scheduler.Stop(ctx)
    
    // 7. 启动应用...
}
```

### 步骤 4：业务代码使用

```go
// internal/service/archive_service.go
package service

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
)

type ArchiveService struct {
    db          *gorm.DB
    outboxRepo  outbox.OutboxRepository
}

func (s *ArchiveService) CreateArchive(ctx context.Context, archive *Archive) error {
    // 开始事务
    return s.db.Transaction(func(tx *gorm.DB) error {
        // 1. 保存业务数据
        if err := tx.Create(archive).Error; err != nil {
            return err
        }
        
        // 2. 创建 Outbox 事件
        event, err := outbox.NewOutboxEvent(
            archive.TenantID,
            archive.ID,
            "Archive",
            "ArchiveCreated",
            archive,
        )
        if err != nil {
            return err
        }
        
        // 3. 在同一事务中保存 Outbox 事件
        if err := s.outboxRepo.Save(ctx, event); err != nil {
            return err
        }
        
        return nil
    })
}
```

## 📚 详细文档

- [完整设计方案](../../../docs/outbox-pattern-design.md)
- [快速开始指南](../../../docs/outbox-pattern-quick-start.md)
- [依赖注入设计](../../../docs/outbox-pattern-dependency-injection-design.md)

## 🎁 核心优势

### 1. 符合 SOLID 原则

- ✅ **依赖倒置原则（DIP）**：Outbox 依赖 EventPublisher 接口，不依赖具体实现
- ✅ **开闭原则（OCP）**：可以扩展新的 EventPublisher 实现，无需修改 Outbox 代码
- ✅ **单一职责原则（SRP）**：每个组件职责明确

### 2. 零外部依赖

```go
// jxt-core/sdk/pkg/outbox/ 的依赖
import (
    "context"
    "time"
    "encoding/json"
    // 没有任何外部包依赖！
)
```

### 3. 易于测试

```go
// 可以轻松 mock EventPublisher
type MockEventPublisher struct {
    publishedEvents []PublishedEvent
}

func (m *MockEventPublisher) Publish(ctx context.Context, topic string, data []byte) error {
    m.publishedEvents = append(m.publishedEvents, PublishedEvent{topic, data})
    return nil
}
```

### 4. 灵活扩展

- ✅ 支持任何 EventBus 实现（Kafka/NATS/Memory/自定义）
- ✅ 支持任何数据库（GORM/MongoDB/自定义）
- ✅ 支持自定义 TopicMapper

## 📊 组件职责

| 组件 | 位置 | 业务微服务需要做什么 |
|------|------|---------------------|
| **OutboxEvent** | jxt-core | ❌ 无需实现 |
| **OutboxRepository** | jxt-core | ❌ 无需实现 |
| **EventPublisher 接口** | jxt-core | ❌ 无需实现（只是接口定义）⭐ |
| **OutboxPublisher** | jxt-core | ❌ 无需实现 |
| **OutboxScheduler** | jxt-core | ❌ 无需实现 |
| **GORM 适配器** | jxt-core | ❌ 无需实现（可选使用） |
| **TopicMapper** | 业务微服务 | ✅ **需要实现**（定义映射） |
| **EventBusAdapter** | 业务微服务 | ✅ **需要实现**（5-10 行代码）⭐ |
| **配置** | 业务微服务 | ✅ **可选**（有默认值） |
| **启动调度器** | 业务微服务 | ✅ **需要启动**（一行代码） |

## 🔧 配置选项

### 调度器配置

```go
config := &outbox.SchedulerConfig{
    PollInterval:        10 * time.Second,  // 轮询间隔
    BatchSize:           100,                // 批量大小
    TenantID:            "",                 // 租户 ID（可选）
    CleanupInterval:     1 * time.Hour,      // 清理间隔
    CleanupRetention:    24 * time.Hour,     // 清理保留时间
    HealthCheckInterval: 30 * time.Second,   // 健康检查间隔
    EnableHealthCheck:   true,               // 启用健康检查
    EnableCleanup:       true,               // 启用自动清理
    EnableMetrics:       true,               // 启用指标收集
}

scheduler := outbox.NewScheduler(
    outbox.WithRepository(repo),
    outbox.WithEventPublisher(eventPublisher),
    outbox.WithTopicMapper(topicMapper),
    outbox.WithSchedulerConfig(config),
)
```

### 发布器配置

```go
config := &outbox.PublisherConfig{
    MaxRetries:     3,                    // 最大重试次数
    RetryDelay:     time.Second,          // 重试延迟
    PublishTimeout: 30 * time.Second,     // 发布超时
    EnableMetrics:  true,                 // 启用指标收集
    ErrorHandler: func(event *outbox.OutboxEvent, err error) {
        // 自定义错误处理
    },
}

scheduler := outbox.NewScheduler(
    outbox.WithRepository(repo),
    outbox.WithEventPublisher(eventPublisher),
    outbox.WithTopicMapper(topicMapper),
    outbox.WithPublisherConfig(config),
)
```

## 📈 监控指标

```go
// 获取调度器指标
metrics := scheduler.GetMetrics()
fmt.Printf("Poll Count: %d\n", metrics.PollCount)
fmt.Printf("Processed Count: %d\n", metrics.ProcessedCount)
fmt.Printf("Error Count: %d\n", metrics.ErrorCount)

// 获取发布器指标
publisherMetrics := publisher.GetMetrics()
fmt.Printf("Published Count: %d\n", publisherMetrics.PublishedCount)
fmt.Printf("Failed Count: %d\n", publisherMetrics.FailedCount)
```

## 📝 许可证

MIT License


