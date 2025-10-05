# 高级事件总线迁移指南

## 概述

本指南详细说明如何将现有的 evidence-management 中的 `KafkaSubscriberManager` 迁移到 jxt-core 的高级事件总线。

## 🚀 架构升级：从恢复模式到Keyed-Worker池

### 核心改进

jxt-core的事件总线采用了全新的**Keyed-Worker池架构**，完全替代了传统的恢复模式机制，带来以下显著优势：

#### 1. **架构简化**
- ❌ **旧架构**：复杂的恢复模式切换逻辑
  - 正常模式 ⟷ 恢复模式状态切换
  - 积压检测和模式切换逻辑
  - 复杂的配置管理
- ✅ **新架构**：统一的Keyed-Worker池处理
  - 无状态切换，架构简洁
  - 一致性哈希路由
  - 配置简单直观

#### 2. **性能提升**
- **消除性能抖动**：无模式切换，处理延迟稳定
- **并发性能优异**：不同聚合ID并行处理
- **资源可控**：固定Worker池，内存使用可预测
- **自然背压**：有界队列提供优雅降级

#### 3. **顺序保证增强**
- **严格顺序**：同一聚合ID通过哈希路由到固定Worker
- **无竞争条件**：每个Worker独立处理
- **故障隔离**：单Worker故障不影响其他聚合ID

#### 4. **运维友好**
- **配置简单**：只需配置Worker数量和队列大小
- **监控直观**：Worker利用率、队列深度等指标清晰
- **故障诊断**：无复杂状态，问题定位容易

## 迁移前准备

### 1. 环境准备

```bash
# 确保 jxt-core 版本支持高级事件总线
cd jxt-core
git checkout feature/advanced-eventbus

# 更新依赖
go mod tidy
```

### 2. 备份现有代码

```bash
# 备份现有的事件总线相关代码
cp -r evidence-management/shared/common/eventbus evidence-management/shared/common/eventbus.backup
```

### 3. 依赖检查

确保以下依赖已正确安装：
- `github.com/hashicorp/golang-lru/v2`
- `golang.org/x/time/rate`
- `github.com/Shopify/sarama`

## 分阶段迁移计划

### 阶段 1：配置迁移

#### 1.1 更新配置结构

**原有配置** (`evidence-management/shared/common/eventbus/kafka_subscriber_manager.go`):
```go
type KafkaSubscriberManagerConfig struct {
    KafkaConfig                     kafka.SubscriberConfig
    HealthCheckConfig               HealthCheckConfig
    MaxReconnectAttempts            int
    IdleTimeout                     time.Duration
    ProcessingRateLimit             rate.Limit
    ProcessingRateBurst             int
    MaxGetOrCreateProcessorAttempts int
    AggregateProcessorCacheSize     int
    MaxLagThreshold                 int64
    MaxTimeThreshold                time.Duration
    NoBacklogCheckInterval          time.Duration
    Threshold                       float64
}
```

**新配置** (`evidence-management/config/eventbus.go`):
```go
import "jxt-core/sdk/config"

func GetEventBusConfig() config.AdvancedEventBusConfig {
    return config.AdvancedEventBusConfig{
        EventBusConfig: config.EventBusConfig{
            Type: "kafka",
            Kafka: config.KafkaConfig{
                Brokers:       []string{"localhost:9092"},
                ConsumerGroup: "evidence-management",
            },
        },

        BacklogDetection: config.BacklogDetectionConfig{
            Enabled:          true,
            MaxLagThreshold:  10,
            MaxTimeThreshold: 30 * time.Second,
            CheckInterval:    3 * time.Minute,
        },
        AggregateProcessor: config.AggregateProcessorConfig{
            Enabled:           true,
            CacheSize:         1000,
            ChannelBufferSize: 100,
            MaxCreateAttempts: 3,
            IdleTimeout:       5 * time.Minute,
            ActiveThreshold:   0.1,
        },
        RateLimit: config.RateLimitConfig{
            Enabled: true,
            Limit:   1000,
            Burst:   1000,
        },
    }
}
```

#### 1.2 配置文件更新

**原有配置文件** (`evidence-management/config/config.yaml`):
```yaml
kafka:
  brokers: ["localhost:9092"]
  consumer_group: "evidence-management"
  
subscriber:
  max_lag_threshold: 10
  max_time_threshold: "30s"
  processing_rate_limit: 1000
  processing_rate_burst: 1000
```

**新配置文件**:
```yaml
eventbus:
  type: "kafka"
  kafka:
    brokers: ["localhost:9092"]
    consumer_group: "evidence-management"
  

  backlogDetection:
    enabled: true
    maxLagThreshold: 10
    maxTimeThreshold: "30s"
    checkInterval: "3m"
    
  aggregateProcessor:
    enabled: true
    cacheSize: 1000
    channelBufferSize: 100
    maxCreateAttempts: 3
    idleTimeout: "5m"
    activeThreshold: 0.1
    
  rateLimit:
    enabled: true
    limit: 1000
    burst: 1000
```

### 阶段 2：接口适配

#### 2.1 创建适配器

**文件**: `evidence-management/shared/common/eventbus/adapter.go`

```go
package eventbus

import (
    "context"
    "time"
    "github.com/ThreeDotsLabs/watermill/message"
    "jxt-core/sdk/pkg/eventbus"
)

// EventSubscriberAdapter 事件订阅器适配器
type EventSubscriberAdapter struct {
    advancedBus eventbus.AdvancedEventBus
}

func NewEventSubscriberAdapter(bus eventbus.AdvancedEventBus) *EventSubscriberAdapter {
    return &EventSubscriberAdapter{
        advancedBus: bus,
    }
}

// Subscribe 保持与原有接口兼容
func (a *EventSubscriberAdapter) Subscribe(topic string, handler func(msg *message.Message) error, timeout time.Duration) error {
    // 包装处理器以适配新接口
    wrappedHandler := func(ctx context.Context, data []byte) error {
        // 创建 watermill 消息
        msg := message.NewMessage(generateUUID(), data)
        
        // 从上下文中提取元数据（如果有）
        if md := extractMetadataFromContext(ctx); md != nil {
            for k, v := range md {
                msg.Metadata.Set(k, v)
            }
        }
        
        return handler(msg)
    }
    
    // 构造订阅选项
    opts := eventbus.SubscribeOptions{
        UseAggregateProcessor: true,
        ProcessingTimeout:     timeout,
        RateLimit:            1000,
        RateBurst:            1000,
        RetryPolicy: eventbus.RetryPolicy{
            MaxRetries:      3,
            InitialInterval: 1 * time.Second,
            MaxInterval:     30 * time.Second,
            Multiplier:      2.0,
        },
        DeadLetterEnabled: true,
    }
    
    return a.advancedBus.SubscribeWithOptions(context.Background(), topic, wrappedHandler, opts)
}

func generateUUID() string {
    // 实现 UUID 生成逻辑
    return "uuid-" + time.Now().Format("20060102150405")
}

func extractMetadataFromContext(ctx context.Context) map[string]string {
    // 从上下文中提取元数据的逻辑
    return nil
}
```

#### 2.2 更新依赖注入

**原有代码** (`evidence-management/query/internal/application/eventhandler/media_event_handler.go`):
```go
func registerMediaEventHandlerDependencies() {
    err := di.Provide(func(subscriber EventSubscriber, ...) *MediaEventHandler {
        return NewMediaEventHandler(subscriber, ...)
    })
}
```

**新代码**:
```go
func registerMediaEventHandlerDependencies() {
    err := di.Provide(func(bus eventbus.AdvancedEventBus, ...) *MediaEventHandler {
        adapter := NewEventSubscriberAdapter(bus)
        return NewMediaEventHandler(adapter, ...)
    })
}

// 添加事件总线提供者
func registerEventBusDependencies() {
    err := di.Provide(func() (eventbus.AdvancedEventBus, error) {
        config := GetEventBusConfig()
        return eventbus.NewKafkaAdvancedEventBus(config)
    })
}
```

### 阶段 3：业务逻辑迁移

#### 3.1 消息路由器迁移

**原有逻辑** (在 `processMessage` 方法中):
```go
aggregateID := msg.Metadata.Get("aggregateID")
if aggregateID != "" && km.aggregateProcessors.Contains(aggregateID) {
    km.processMessageWithAggregateProcessor(ctx, msg, handler, timeout)
} else {
    km.processMessageImmediately(ctx, msg, handler, timeout)
}
```

**新实现** (`evidence-management/shared/common/eventbus/message_router.go`):
```go
type EvidenceMessageRouter struct{}

func (r *EvidenceMessageRouter) ShouldUseAggregateProcessor(msg eventbus.MessageContext) bool {
    aggregateID := r.ExtractAggregateID(msg)
    return aggregateID != ""
}

func (r *EvidenceMessageRouter) ExtractAggregateID(msg eventbus.MessageContext) string {
    if aggregateID, exists := msg.Metadata["aggregateID"]; exists {
        return aggregateID
    }
    return ""
}

func (r *EvidenceMessageRouter) GetProcessingTimeout(msg eventbus.MessageContext) time.Duration {
    // 根据消息类型返回不同的超时时间
    if eventType, exists := msg.Metadata["eventType"]; exists {
        switch eventType {
        case "MediaUploaded":
            return 60 * time.Second
        case "ArchiveCreated":
            return 30 * time.Second
        default:
            return 15 * time.Second
        }
    }
    return 30 * time.Second
}
```

#### 3.2 错误处理器迁移

**原有逻辑**:
```go
func (km *KafkaSubscriberManager) isRetryableError(err error) bool {
    switch err.(type) {
    case *net.OpError, *os.SyscallError:
        return true
    case *json.SyntaxError, *json.UnmarshalTypeError:
        return false
    default:
        return false
    }
}
```

**新实现** (`evidence-management/shared/common/eventbus/error_handler.go`):
```go
type EvidenceErrorHandler struct{}

func (h *EvidenceErrorHandler) IsRetryable(err error) bool {
    switch err.(type) {
    case *json.SyntaxError, *json.UnmarshalTypeError:
        return false
    case *net.OpError, *os.SyscallError:
        return true
    default:
        return isBusinessLogicError(err) == false
    }
}

func (h *EvidenceErrorHandler) HandleRetryableError(ctx context.Context, msg eventbus.MessageContext, err error) error {
    log.Printf("Retryable error for message %s: %v", msg.Topic, err)
    return nil // 使用默认重试机制
}

func (h *EvidenceErrorHandler) HandleNonRetryableError(ctx context.Context, msg eventbus.MessageContext, err error) error {
    log.Printf("Non-retryable error for message %s: %v", msg.Topic, err)
    return h.SendToDeadLetter(ctx, msg, err.Error())
}

func (h *EvidenceErrorHandler) SendToDeadLetter(ctx context.Context, msg eventbus.MessageContext, reason string) error {
    deadLetterTopic := fmt.Sprintf("%s-dead-letter", msg.Topic)
    log.Printf("Sending message to dead letter queue: %s", deadLetterTopic)
    // 实现死信队列逻辑
    return nil
}

func isBusinessLogicError(err error) bool {
    // 实现业务逻辑错误判断
    errMsg := strings.ToLower(err.Error())
    return strings.Contains(errMsg, "validation") ||
           strings.Contains(errMsg, "business rule") ||
           strings.Contains(errMsg, "invalid data")
}
```

### 阶段 4：回调处理迁移

#### 4.1 积压状态回调

**原有逻辑** (在 `startNoBacklogCheckLoop` 中):
```go
if km.checkNoBacklog(ctx) {
    km.noBacklogCount++
    if km.noBacklogCount >= 3 {
        log.Println("连续三次检测无积压，切换到正常消费者状态")
        km.noBacklogCount = 0
        return
    }
}
```

**新实现** (`evidence-management/shared/common/eventbus/callbacks.go`):
```go
func HandleBacklogStateChange(ctx context.Context, state eventbus.BacklogState) error {
    if state.HasBacklog {
        log.Printf("积压检测到: 总积压 %d 条消息，活跃处理器 %d 个", 
                  state.TotalLag, state.ActiveProcessors)
        
        // 发送告警
        if err := sendBacklogAlert(state); err != nil {
            log.Printf("发送积压告警失败: %v", err)
        }
        
        // 可以在这里实现其他业务逻辑，如暂停非关键任务
        pauseNonCriticalTasks()
        
    } else {
        log.Println("无积压检测到，系统状态正常")
        
        // 清除告警
        clearBacklogAlert()
        
        // 恢复正常任务
        resumeNormalTasks()
    }
    
    return nil
}

func sendBacklogAlert(state eventbus.BacklogState) error {
    // 实现告警发送逻辑
    return nil
}

func clearBacklogAlert() {
    // 实现告警清除逻辑
}

func pauseNonCriticalTasks() {
    // 暂停非关键任务
}

func resumeNormalTasks() {
    // 恢复正常任务
}
```

### 阶段 5：初始化代码更新

#### 5.1 应用启动代码

**原有代码** (`evidence-management/cmd/query/main.go`):
```go
func main() {
    // 初始化 Kafka 订阅管理器
    config := eventbus.DefaultKafkaSubscriberManagerConfig()
    manager, err := eventbus.NewKafkaSubscriberManager(config)
    if err != nil {
        log.Fatal(err)
    }
    
    if err := manager.Start(); err != nil {
        log.Fatal(err)
    }
    
    // 注册到 DI 容器
    di.Provide(func() *eventbus.KafkaSubscriberManager {
        return manager
    })
}
```

**新代码**:
```go
func main() {
    // 初始化统一事件总线
    config := &eventbus.EventBusConfig{
        Type: "kafka",
        Kafka: eventbus.KafkaConfig{
            Brokers: []string{"localhost:9092"},
        },
        Enterprise: eventbus.EnterpriseConfig{
            Subscriber: eventbus.SubscriberEnterpriseConfig{
                BacklogDetection: eventbus.BacklogDetectionConfig{
                    Enabled: true,
                },
            },
        },
    }

    bus, err := eventbus.NewEventBus(config)
    if err != nil {
        log.Fatal(err)
    }

    // 设置业务组件
    bus.SetMessageRouter(&eventbus.EvidenceMessageRouter{})
    bus.SetErrorHandler(&eventbus.EvidenceErrorHandler{})

    // 注册回调
    bus.RegisterBacklogCallback(eventbus.HandleBacklogStateChange)

    // 启动积压监控
    if err := bus.StartBacklogMonitoring(context.Background()); err != nil {
        log.Fatal(err)
    }

    // 注册到 DI 容器
    di.Provide(func() eventbus.EventBus {
        return bus
    })

    // 创建适配器以保持兼容性
    di.Provide(func(bus eventbus.EventBus) EventSubscriber {
        return eventbus.NewEventSubscriberAdapter(bus)
    })
}
```

## 测试策略

### 1. 单元测试

```go
func TestEventBusAdapter(t *testing.T) {
    // 创建模拟的高级事件总线
    mockBus := &MockAdvancedEventBus{}
    adapter := NewEventSubscriberAdapter(mockBus)
    
    // 测试订阅功能
    err := adapter.Subscribe("test-topic", func(msg *message.Message) error {
        return nil
    }, 30*time.Second)
    
    assert.NoError(t, err)
    assert.True(t, mockBus.SubscribeWithOptionsCalled)
}
```

### 2. 集成测试

```go
func TestBacklogDetection(t *testing.T) {
    // 创建真实的事件总线
    config := GetTestEventBusConfig()
    bus, err := eventbus.NewKafkaAdvancedEventBus(config)
    require.NoError(t, err)
    
    // 启动积压监控
    err = bus.StartBacklogMonitoring(context.Background())
    require.NoError(t, err)
    
    // 模拟积压情况
    // ... 测试逻辑
    
    // 验证回调被调用
    // ... 验证逻辑
}
```

### 3. 性能测试

```go
func BenchmarkMessageProcessing(b *testing.B) {
    bus := setupTestEventBus()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // 发送测试消息
        sendTestMessage(bus, fmt.Sprintf("test-message-%d", i))
    }
}
```

## 回滚计划

如果迁移过程中出现问题，可以按以下步骤回滚：

1. **停止新的事件总线**
2. **恢复备份的代码**
3. **重启应用**
4. **验证功能正常**

```bash
# 回滚脚本
#!/bin/bash
echo "开始回滚..."

# 停止应用
kubectl scale deployment evidence-management-query --replicas=0

# 恢复代码
rm -rf evidence-management/shared/common/eventbus
mv evidence-management/shared/common/eventbus.backup evidence-management/shared/common/eventbus

# 重新构建和部署
docker build -t evidence-management:rollback .
kubectl set image deployment/evidence-management-query app=evidence-management:rollback

# 重启应用
kubectl scale deployment evidence-management-query --replicas=3

echo "回滚完成"
```

## 验证清单

迁移完成后，请验证以下功能：

- [ ] 消息订阅功能正常
- [ ] 消息处理功能正常
- [ ] 积压检测功能正常
- [ ] 聚合处理器功能正常
- [ ] 错误处理功能正常
- [ ] 死信队列功能正常
- [ ] 性能指标正常
- [ ] 监控告警正常
- [ ] 日志记录正常

## 常见问题

### Q1: 迁移后性能下降怎么办？
A1: 检查配置参数，特别是缓存大小、处理器数量、速率限制等参数。

### Q2: 消息处理顺序有问题怎么办？
A2: 检查消息路由器的 `ExtractAggregateID` 方法是否正确提取聚合ID。

### Q3: 积压检测不准确怎么办？
A3: 检查积压检测的阈值配置，可能需要根据实际业务调整。

### Q4: 回调函数执行失败怎么办？
A4: 检查回调函数的实现，确保没有阻塞操作，必要时使用异步处理。

通过遵循这个迁移指南，可以安全、有序地将现有系统迁移到新的高级事件总线架构。
