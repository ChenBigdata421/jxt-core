# Outbox 模式依赖注入设计说明

## 📋 设计决策

### 核心问题
**问题**：Outbox 组件应该直接依赖 EventBus 实现，还是通过依赖注入的方式？

**决策**：✅ **采用依赖注入方式，Outbox 依赖 EventPublisher 接口，不依赖具体 EventBus 实现**

---

## 🎯 设计原理

### 依赖倒置原则（DIP）

```
❌ 不好的设计（直接依赖）
┌─────────────────┐
│ Outbox 组件     │
│                 │
│ - OutboxPublisher│──依赖──┐
└─────────────────┘        │
                           ▼
                  ┌─────────────────┐
                  │ EventBus 实现   │
                  │ (jxt-core)      │
                  └─────────────────┘

问题：
- ❌ 高耦合：Outbox 直接依赖 EventBus 包
- ❌ 难以测试：需要真实的 EventBus 实例
- ❌ 不灵活：无法替换 EventBus 实现


✅ 好的设计（依赖注入）
┌─────────────────────────────────┐
│ Outbox 组件                     │
│                                 │
│ - EventPublisher 接口（定义）   │
│ - OutboxPublisher（使用接口）   │
└─────────────────────────────────┘
           ▲
           │ 实现接口
           │
┌──────────┴──────────┬──────────────────┐
│                     │                  │
│ EventBusAdapter     │ MockPublisher    │ 自定义实现
│ (业务微服务)        │ (测试)           │
│                     │                  │
│ ┌─────────────┐     │                  │
│ │ EventBus    │     │                  │
│ │ (jxt-core)  │     │                  │
│ └─────────────┘     │                  │
└─────────────────────┴──────────────────┘

优势：
- ✅ 低耦合：Outbox 只依赖接口
- ✅ 易于测试：可以 mock EventPublisher
- ✅ 灵活扩展：可以使用任何 EventBus 实现
- ✅ 业务控制：业务微服务完全控制 EventBus
```

---

## 📦 核心组件

### 1. EventPublisher 接口（jxt-core）

**位置**：`jxt-core/sdk/pkg/outbox/event_publisher.go`

```go
package outbox

import "context"

// EventPublisher 事件发布器接口
// Outbox 组件通过这个接口发布事件，不依赖具体的 EventBus 实现
type EventPublisher interface {
    // Publish 发布事件到指定 topic
    // ctx: 上下文
    // topic: 目标 topic（由 TopicMapper 提供）
    // data: 事件数据（已序列化为 JSON）
    Publish(ctx context.Context, topic string, data []byte) error
}
```

**设计要点**：
- ✅ **最小化接口**：只包含 Outbox 需要的方法
- ✅ **零外部依赖**：不依赖任何其他包
- ✅ **清晰语义**：方法签名简单明了

### 2. OutboxPublisher（jxt-core）

**位置**：`jxt-core/sdk/pkg/outbox/publisher.go`

```go
package outbox

type OutboxPublisher struct {
    repo            OutboxRepository  // 仓储
    eventPublisher  EventPublisher    // 事件发布器（接口）⭐
    topicMapper     TopicMapper       // Topic 映射
    config          *PublisherConfig  // 配置
}

func NewOutboxPublisher(
    repo OutboxRepository,
    eventPublisher EventPublisher,  // 注入实现
    topicMapper TopicMapper,
    config *PublisherConfig,
) *OutboxPublisher {
    return &OutboxPublisher{
        repo:           repo,
        eventPublisher: eventPublisher,  // 保存注入的实现
        topicMapper:    topicMapper,
        config:         config,
    }
}

func (p *OutboxPublisher) PublishEvent(ctx context.Context, event *OutboxEvent) error {
    // 1. 获取 topic
    topic := p.topicMapper.GetTopic(event.AggregateType)
    
    // 2. 序列化事件
    data, err := json.Marshal(event)
    if err != nil {
        return err
    }
    
    // 3. 通过 EventPublisher 接口发布 ⭐
    if err := p.eventPublisher.Publish(ctx, topic, data); err != nil {
        return err
    }
    
    // 4. 更新状态
    return p.repo.MarkAsPublished(ctx, event.ID)
}
```

**设计要点**：
- ✅ **依赖接口**：`eventPublisher EventPublisher`
- ✅ **构造注入**：通过构造函数注入实现
- ✅ **面向接口编程**：不关心具体实现

### 3. EventBusAdapter（业务微服务）

**位置**：`evidence-management/internal/outbox/eventbus_adapter.go`

```go
package outbox

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// EventBusAdapter 将 jxt-core EventBus 适配为 Outbox EventPublisher
type EventBusAdapter struct {
    eventBus eventbus.EventBus
}

func NewEventBusAdapter(eventBus eventbus.EventBus) outbox.EventPublisher {
    return &EventBusAdapter{eventBus: eventBus}
}

func (a *EventBusAdapter) Publish(ctx context.Context, topic string, data []byte) error {
    // 调用 jxt-core EventBus 的 Publish 方法
    return a.eventBus.Publish(ctx, topic, data)
}
```

**设计要点**：
- ✅ **适配器模式**：将 EventBus 适配为 EventPublisher
- ✅ **简单实现**：只需 5-10 行代码
- ✅ **业务控制**：业务微服务创建和管理 EventBus

---

## 🚀 使用方式

### 业务微服务集成

```go
// evidence-management/cmd/main.go
package main

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
    localoutbox "evidence-management/internal/outbox"
)

func main() {
    ctx := context.Background()
    
    // 1. 获取数据库
    db := sdk.Runtime.GetDB()
    
    // 2. 创建仓储
    outboxRepo := gormadapter.NewGormOutboxRepository(db)
    
    // 3. 创建 EventBus（业务微服务控制）⭐
    eventBus := sdk.Runtime.GetEventBus()
    
    // 4. 创建 EventBus 适配器 ⭐
    eventPublisher := localoutbox.NewEventBusAdapter(eventBus)
    
    // 5. 创建调度器（注入 EventPublisher）⭐
    scheduler := outbox.NewScheduler(
        outbox.WithRepository(outboxRepo),
        outbox.WithEventPublisher(eventPublisher),  // 注入实现
        outbox.WithTopicMapper(localoutbox.NewTopicMapper()),
    )
    
    // 6. 启动调度器
    scheduler.Start(ctx)
    defer scheduler.Stop(ctx)
}
```

---

## ✅ 设计优势

### 1. 符合 SOLID 原则

#### 依赖倒置原则（DIP）
- ✅ 高层模块（Outbox）不依赖低层模块（EventBus）
- ✅ 两者都依赖抽象（EventPublisher 接口）

#### 开闭原则（OCP）
- ✅ 对扩展开放：可以添加新的 EventPublisher 实现
- ✅ 对修改关闭：Outbox 代码无需修改

#### 单一职责原则（SRP）
- ✅ Outbox：负责事件持久化和调度
- ✅ EventPublisher：负责事件发布
- ✅ EventBusAdapter：负责适配

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

**优势**：
- ✅ Outbox 包完全独立
- ✅ 不依赖 EventBus 包
- ✅ 不依赖任何第三方库
- ✅ 易于维护和测试

### 3. 易于测试

```go
// outbox_publisher_test.go
package outbox_test

import (
    "context"
    "testing"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
)

// MockEventPublisher 用于测试
type MockEventPublisher struct {
    publishedEvents []PublishedEvent
}

func (m *MockEventPublisher) Publish(ctx context.Context, topic string, data []byte) error {
    m.publishedEvents = append(m.publishedEvents, PublishedEvent{
        Topic: topic,
        Data:  data,
    })
    return nil
}

func TestOutboxPublisher(t *testing.T) {
    // 创建 mock
    mockPublisher := &MockEventPublisher{}
    
    // 创建 OutboxPublisher（注入 mock）
    publisher := outbox.NewOutboxPublisher(
        repo,
        mockPublisher,  // 注入 mock
        topicMapper,
        config,
    )
    
    // 测试发布
    err := publisher.PublishEvent(ctx, event)
    
    // 验证
    assert.NoError(t, err)
    assert.Equal(t, 1, len(mockPublisher.publishedEvents))
}
```

### 4. 灵活扩展

#### 使用 jxt-core EventBus
```go
eventPublisher := localoutbox.NewEventBusAdapter(sdk.Runtime.GetEventBus())
```

#### 使用自定义 EventBus
```go
type CustomEventPublisher struct {
    // 自定义实现
}

func (c *CustomEventPublisher) Publish(ctx context.Context, topic string, data []byte) error {
    // 自定义发布逻辑
    return nil
}

eventPublisher := &CustomEventPublisher{}
```

#### 使用第三方库
```go
type KafkaEventPublisher struct {
    producer *kafka.Producer
}

func (k *KafkaEventPublisher) Publish(ctx context.Context, topic string, data []byte) error {
    return k.producer.Produce(topic, data)
}

eventPublisher := &KafkaEventPublisher{producer: kafkaProducer}
```

---

## 📊 对比分析

| 方面 | 直接依赖 EventBus | 依赖注入 EventPublisher（推荐）|
|------|------------------|-------------------------------|
| **耦合度** | ❌ 高耦合 | ✅ 低耦合 |
| **外部依赖** | ❌ 依赖 EventBus 包 | ✅ 零外部依赖 |
| **可测试性** | ⚠️ 需要真实 EventBus | ✅ 可以 mock |
| **灵活性** | ❌ 固定 EventBus 实现 | ✅ 可替换实现 |
| **业务控制** | ❌ Outbox 控制 EventBus | ✅ 业务微服务控制 |
| **SOLID 原则** | ❌ 违反 DIP | ✅ 符合 DIP、OCP、SRP |
| **复杂度** | ✅ 简单（无适配器） | ⚠️ 需要适配器（5-10 行） |
| **维护成本** | ❌ 高（紧耦合） | ✅ 低（松耦合） |

---

## 🎓 最佳实践

### 1. 适配器命名规范

```go
// ✅ 好的命名
type EventBusAdapter struct { ... }
type KafkaEventPublisher struct { ... }
type NatsEventPublisher struct { ... }

// ❌ 不好的命名
type Adapter struct { ... }
type Publisher struct { ... }
```

### 2. 适配器位置

```
evidence-management/
  internal/
    outbox/
      eventbus_adapter.go    ✅ 推荐：放在 outbox 包内
      topics.go
```

### 3. 错误处理

```go
func (a *EventBusAdapter) Publish(ctx context.Context, topic string, data []byte) error {
    // ✅ 透传错误
    return a.eventBus.Publish(ctx, topic, data)
    
    // ❌ 不要吞掉错误
    // a.eventBus.Publish(ctx, topic, data)
    // return nil
}
```

### 4. 日志记录

```go
func (a *EventBusAdapter) Publish(ctx context.Context, topic string, data []byte) error {
    // ✅ 可以添加日志
    log.Debugf("Publishing event to topic: %s", topic)
    
    err := a.eventBus.Publish(ctx, topic, data)
    
    if err != nil {
        log.Errorf("Failed to publish event: %v", err)
    }
    
    return err
}
```

---

## 📝 总结

### 核心决策
✅ **Outbox 依赖 EventPublisher 接口，不依赖具体 EventBus 实现**

### 关键优势
1. ✅ **符合 SOLID 原则**（DIP、OCP、SRP）
2. ✅ **零外部依赖**：Outbox 包完全独立
3. ✅ **易于测试**：可以轻松 mock
4. ✅ **灵活扩展**：支持任何 EventBus 实现
5. ✅ **业务控制**：业务微服务完全控制 EventBus

### 实施成本
- ⚠️ 需要业务微服务创建适配器（5-10 行代码）
- ✅ 这个成本完全值得！

### 推荐指数
⭐⭐⭐⭐⭐ **强烈推荐**

---

**文档版本**：v1.0  
**最后更新**：2025-10-19  
**作者**：jxt-core 团队


