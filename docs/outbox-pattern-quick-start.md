# Outbox 模式快速开始指南

## 🚀 5 分钟快速上手

本指南帮助你在 5 分钟内将 jxt-core Outbox 模式集成到你的微服务中。

---

## 📋 前置条件

- ✅ 已安装 jxt-core
- ✅ 已配置数据库（MySQL/PostgreSQL/SQLite）
- ✅ 已配置 EventBus（Kafka/NATS/Memory）

---

## 🎯 四步集成

### 步骤 1：定义 Topic 映射（1 分钟）

创建文件：`internal/outbox/topics.go`

```go
package outbox

import "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"

// NewTopicMapper 创建业务特定的 Topic 映射
func NewTopicMapper() outbox.TopicMapper {
    return outbox.NewTopicMapper(map[string]string{
        // 聚合类型 → Kafka/NATS Topic
        "Archive": "archive-events",
        "Media":   "media-events",
        "User":    "user-events",
        // 添加你的业务聚合类型...
    })
}
```

### 步骤 2：创建 EventBus 适配器（1 分钟）⭐

创建文件：`internal/outbox/eventbus_adapter.go`

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
    return a.eventBus.Publish(ctx, topic, data)
}
```

### 步骤 3：初始化 Outbox（2 分钟）

在 `cmd/main.go` 中添加：

```go
package main

import (
    "context"
    "time"
    "github.com/ChenBigdata421/jxt-core/sdk"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
    localoutbox "your-service/internal/outbox"
)

func main() {
    ctx := context.Background()

    // 1. 获取数据库连接
    db := sdk.Runtime.GetDB()

    // 2. 自动迁移 outbox 表
    db.AutoMigrate(&gormadapter.OutboxEventModel{})

    // 3. 创建 EventBus 适配器 ⭐
    eventBus := sdk.Runtime.GetEventBus()
    eventPublisher := localoutbox.NewEventBusAdapter(eventBus)

    // 4. 创建并启动 Outbox 调度器
    scheduler := outbox.NewScheduler(
        outbox.WithRepository(gormadapter.NewGormOutboxRepository(db)),
        outbox.WithEventPublisher(eventPublisher),  // 注入 EventPublisher ⭐
        outbox.WithTopicMapper(localoutbox.NewTopicMapper()),
    )

    scheduler.Start(ctx)
    defer scheduler.Stop(ctx)

    // 你的应用启动代码...
}
```

### 步骤 4：在业务代码中使用（1 分钟）

```go
package service

import (
    "context"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    "your-service/internal/domain/aggregate"
    "your-service/shared/transaction"
)

type YourService struct {
    repo       YourRepository
    outboxRepo outbox.OutboxRepository
    txManager  transaction.TransactionManager
}

func (s *YourService) CreateSomething(ctx context.Context, cmd *Command) error {
    // 在事务中保存业务数据和事件
    return transaction.RunInTransaction(ctx, s.txManager, func(tx transaction.Transaction) error {
        // 1. 创建聚合根
        agg := aggregate.NewYourAggregate(cmd.Data)
        
        // 2. 保存业务数据
        if err := s.repo.CreateInTx(ctx, tx, agg); err != nil {
            return err
        }
        
        // 3. 保存领域事件到 outbox
        for _, event := range agg.Events() {
            if err := s.outboxRepo.SaveInTx(ctx, tx, event); err != nil {
                return err
            }
        }
        
        return nil
        // 事务提交后，调度器会自动发布事件！
    })
}
```

---

## ✅ 完成！

现在你的微服务已经集成了 Outbox 模式：

- ✅ **原子性保证**：业务数据和事件在同一事务中
- ✅ **自动发布**：调度器每 3 秒自动发布未发布事件
- ✅ **自动重试**：失败事件自动重试（最多 5 次）
- ✅ **健康检查**：自动监控事件积压

---

## 🔧 可选配置

### 自定义调度器配置

```go
scheduler := outbox.NewScheduler(
    outbox.WithRepository(repo),
    outbox.WithEventBus(eventBus),
    outbox.WithTopicMapper(topicMapper),
    
    // 自定义配置
    outbox.WithConfig(&outbox.SchedulerConfig{
        MainProcessInterval:  3 * time.Second,  // 主处理间隔
        RetryProcessInterval: 30 * time.Second, // 重试间隔
        BatchSize:            100,              // 批处理大小
    }),
    
    outbox.WithPublisherConfig(&outbox.PublisherConfig{
        MaxRetries: 5,                    // 最大重试次数
        BaseDelay:  2 * time.Second,      // 基础延迟
        MaxDelay:   30 * time.Minute,     // 最大延迟
    }),
)
```

### 配置文件方式

```yaml
# config.yaml
outbox:
  scheduler:
    mainProcessInterval: 3s
    retryProcessInterval: 30s
    batchSize: 100
  publisher:
    maxRetries: 5
    baseDelay: 2s
    maxDelay: 30m
```

---

## 📊 验证集成

### 1. 检查数据库表

```sql
-- 查看 outbox 表
SELECT * FROM outbox_events ORDER BY created_at DESC LIMIT 10;

-- 查看未发布事件
SELECT COUNT(*) FROM outbox_events WHERE status = 'CREATED';
```

### 2. 查看日志

```
[INFO] 启动outbox事件调度器
[INFO] 启动outbox主处理循环 interval=3s
[INFO] 启动失败事件重试循环 interval=30s
[INFO] 启动健康检查循环 interval=60s
[DEBUG] 事件发布成功 eventID=xxx topic=archive-events
```

### 3. 监控指标

- 未发布事件数量应该保持在较低水平（< 100）
- 发布成功率应该 > 95%
- 失败事件应该能自动重试成功

---

## 🎓 下一步

### 深入学习

- 📖 [完整设计方案](./outbox-pattern-design.md)：了解架构和原理
- 📖 [EventBus 文档](../sdk/pkg/eventbus/README.md)：了解事件总线
- 📖 [最佳实践](#最佳实践)：生产环境建议

### 最佳实践

#### 1. 事件设计

```go
// ✅ 好的事件设计
type ArchiveCreatedEvent struct {
    EventID     string    `json:"eventId"`
    ArchiveID   string    `json:"archiveId"`
    ArchiveCode string    `json:"archiveCode"`
    ArchiveName string    `json:"archiveName"`
    CreatedAt   time.Time `json:"createdAt"`
    CreatedBy   string    `json:"createdBy"`
}

// ❌ 不好的事件设计
type ArchiveEvent struct {
    Archive *Archive // 不要包含整个聚合根
    Action  string   // 不要用通用事件
}
```

#### 2. 幂等性处理

```go
// 消费端应该实现幂等性
func (h *Handler) Handle(ctx context.Context, event *Event) error {
    // 检查是否已处理
    if h.isProcessed(event.ID) {
        return nil // 幂等返回
    }
    
    // 处理事件
    if err := h.process(event); err != nil {
        return err
    }
    
    // 标记为已处理
    h.markAsProcessed(event.ID)
    return nil
}
```

#### 3. 定期清理

```go
// 定期清理已发布的旧事件
go func() {
    ticker := time.NewTicker(24 * time.Hour)
    for range ticker.C {
        beforeTime := time.Now().AddDate(0, 0, -7) // 7 天前
        outboxRepo.DeleteOldPublishedEvents(ctx, beforeTime)
    }
}()
```

---

## 🐛 常见问题

### Q1: 事件没有被发布？

**检查清单**：
- [ ] 调度器是否启动？
- [ ] EventBus 是否连接正常？
- [ ] TopicMapper 是否配置了对应的聚合类型？
- [ ] 查看日志是否有错误信息？

### Q2: 事件重复发布？

**解决方案**：
- 消费端实现幂等性处理
- 使用事件 ID 去重

### Q3: 事件积压？

**解决方案**：
```go
// 增加批处理大小
config.BatchSize = 200

// 减少处理间隔
config.MainProcessInterval = 1 * time.Second
```

---

## 📞 获取帮助

- 📖 [完整文档](./outbox-pattern-design.md)
- 🐛 [提交 Issue](https://github.com/your-org/jxt-core/issues)
- 💬 [讨论区](https://github.com/your-org/jxt-core/discussions)

---

## 📝 完整示例

### 完整的微服务示例

```go
// cmd/main.go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/ChenBigdata421/jxt-core/sdk"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox"
    gormadapter "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
    "your-service/internal/outbox"
)

func main() {
    ctx := context.Background()
    
    // 初始化 SDK
    app := sdk.NewApplication()
    
    // 获取数据库
    db := sdk.Runtime.GetDB()
    
    // 迁移 outbox 表
    if err := db.AutoMigrate(&gormadapter.OutboxEventModel{}); err != nil {
        panic(err)
    }
    
    // 创建 Outbox 调度器
    outboxRepo := gormadapter.NewGormOutboxRepository(db)
    scheduler := outbox.NewScheduler(
        outbox.WithRepository(outboxRepo),
        outbox.WithEventBus(sdk.Runtime.GetEventBus()),
        outbox.WithTopicMapper(outbox.NewTopicMapper()),
        outbox.WithConfig(&outbox.SchedulerConfig{
            MainProcessInterval:  3 * time.Second,
            RetryProcessInterval: 30 * time.Second,
            BatchSize:            100,
        }),
    )
    
    // 启动调度器
    if err := scheduler.Start(ctx); err != nil {
        panic(err)
    }
    
    // 启动应用
    go app.Run()
    
    // 优雅关闭
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    // 停止调度器
    scheduler.Stop(ctx)
    
    // 停止应用
    app.Shutdown(ctx)
}
```

---

**快速开始指南版本**：v1.0  
**最后更新**：2025-10-19


