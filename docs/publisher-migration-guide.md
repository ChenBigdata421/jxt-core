# 事件发布器迁移指南

## 概述

本指南详细说明如何将现有的 evidence-management 中的 `KafkaPublisherManager` 迁移到 jxt-core 的高级事件发布器。重点关注统一健康检查、发布端管理和业务层适配。

## 迁移前后对比

### 迁移前（evidence-management）

```go
// 业务层需要管理所有技术细节
type KafkaPublisherManagerConfig struct {
    KafkaPublisherConfig kafka.PublisherConfig
    HealthCheckTopic     string        // 业务层定义健康检查主题
    HealthCheckInterval  time.Duration // 业务层配置检查间隔
    MaxReconnectAttempts int
    MaxBackoff           time.Duration
}

// 业务层实现健康检查逻辑
func (km *KafkaPublisherManager) healthCheck() error {
    msg := message.NewMessage(watermill.NewUUID(), []byte("health check"))
    err := publisher.Publish(km.Config.HealthCheckTopic, msg)
    return err
}

// 业务层处理消息格式化
func (km *KafkaPublisherManager) PublishMessage(topic string, uuid string, aggregateID interface{}, payload []byte) error {
    aggID := ""
    if id, ok := aggregateID.(int64); ok {
        aggID = strconv.FormatInt(id, 10)
    }
    msg := message.NewMessage(uuid, payload)
    msg.Metadata.Set("aggregate_id", aggID)
    return publisher.Publish(topic, msg)
}
```

### 迁移后（jxt-core + 业务适配）

```go
// jxt-core 统一管理技术基础设施
type AdvancedEventBusConfig struct {
    ServiceName string `mapstructure:"serviceName"` // 只需指定服务名
    HealthCheck HealthCheckConfig `mapstructure:"healthCheck"`
    Publisher   PublisherConfig   `mapstructure:"publisher"`
}

// jxt-core 统一健康检查（业务层无需关心）
const DefaultHealthCheckTopic = "jxt-core-health-check"

// 业务层只需要适配器
type EventPublisherAdapter struct {
    advancedBus eventbus.AdvancedEventBus
}

func (a *EventPublisherAdapter) PublishMessage(topic string, uuid string, aggregateID interface{}, payload []byte) error {
    opts := eventbus.PublishOptions{
        AggregateID: aggregateID,
        Timeout:     30 * time.Second,
    }
    return a.advancedBus.PublishWithOptions(context.Background(), topic, payload, opts)
}
```

## 分阶段迁移计划

### 阶段 1：配置迁移（第1天）

#### 1.1 更新配置文件

**原有配置** (`evidence-management/config/config.yaml`):
```yaml
kafka:
  publisher:
    brokers: ["localhost:9092"]
    
publisher_manager:
  health_check_topic: "health_check_topic"  # 删除：由 jxt-core 统一管理
  health_check_interval: "2m"               # 删除：使用 jxt-core 默认配置
  max_reconnect_attempts: 5
  max_backoff: "1m"
```

**新配置**:
```yaml
eventbus:
  type: "kafka"
  serviceName: "evidence-management"  # 新增：服务标识
  
  kafka:
    brokers: ["localhost:9092"]
    
  healthCheck:
    enabled: true
    # topic 由 jxt-core 统一管理，无需配置
    # interval、timeout 等使用默认值
    
  publisher:
    maxReconnectAttempts: 5
    maxBackoff: "1m"
    initialBackoff: "1s"
    publishTimeout: "30s"
```

#### 1.2 更新配置结构

**文件**: `evidence-management/config/eventbus.go`

```go
import "jxt-core/sdk/config"

func GetEventBusConfig() config.AdvancedEventBusConfig {
    return config.AdvancedEventBusConfig{
        ServiceName: "evidence-management",
        EventBusConfig: config.EventBusConfig{
            Type: "kafka",
            Kafka: config.KafkaConfig{
                Brokers: viper.GetStringSlice("eventbus.kafka.brokers"),
            },
        },
        HealthCheck: config.HealthCheckConfig{
            Enabled: viper.GetBool("eventbus.healthCheck.enabled"),
            // 其他配置使用 jxt-core 默认值
        },
        Publisher: config.PublisherConfig{
            MaxReconnectAttempts: viper.GetInt("eventbus.publisher.maxReconnectAttempts"),
            MaxBackoff:          viper.GetDuration("eventbus.publisher.maxBackoff"),
            InitialBackoff:      viper.GetDuration("eventbus.publisher.initialBackoff"),
            PublishTimeout:      viper.GetDuration("eventbus.publisher.publishTimeout"),
        },
    }
}
```

### 阶段 2：创建适配器（第2天）

#### 2.1 创建消息格式化器

**文件**: `evidence-management/shared/common/eventbus/message_formatter.go`

```go
package eventbus

import "jxt-core/sdk/pkg/eventbus"

// EvidenceMessageFormatter 保持与现有代码兼容的消息格式化器
type EvidenceMessageFormatter struct {
    eventbus.DefaultMessageFormatter
}

func (f *EvidenceMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*eventbus.Message, error) {
    msg, err := f.DefaultMessageFormatter.FormatMessage(uuid, aggregateID, payload)
    if err != nil {
        return nil, err
    }
    
    // 保持与现有代码兼容：使用 aggregate_id 字段名
    if aggID := f.ExtractAggregateID(aggregateID); aggID != "" {
        msg.Metadata["aggregate_id"] = aggID
    }
    
    return msg, nil
}
```

#### 2.2 创建事件发布器适配器

**文件**: `evidence-management/shared/common/eventbus/publisher_adapter.go`

```go
package eventbus

import (
    "context"
    "fmt"
    "log"
    "time"
    
    "jxt-core/sdk/pkg/eventbus"
    "jxt-evidence-system/evidence-management/shared/domain/event"
)

type EventPublisherAdapter struct {
    advancedBus eventbus.AdvancedEventBus
}

func NewEventPublisherAdapter(bus eventbus.AdvancedEventBus) *EventPublisherAdapter {
    adapter := &EventPublisherAdapter{
        advancedBus: bus,
    }
    
    // 设置业务特定的消息格式化器
    bus.SetMessageFormatter(&EvidenceMessageFormatter{})
    
    // 注册业务回调
    bus.RegisterReconnectCallback(adapter.handleReconnect)
    bus.RegisterPublishCallback(adapter.handlePublishResult)
    bus.RegisterHealthCheckCallback(adapter.handleHealthCheck)
    
    return adapter
}

// PublishMessage 保持与原有 KafkaPublisherManager 接口兼容
func (a *EventPublisherAdapter) PublishMessage(topic string, uuid string, aggregateID interface{}, payload []byte) error {
    opts := eventbus.PublishOptions{
        AggregateID: aggregateID,
        Timeout:     30 * time.Second,
        Metadata: map[string]string{
            "messageUUID": uuid,
        },
    }
    
    return a.advancedBus.PublishWithOptions(context.Background(), topic, payload, opts)
}

// Publish 实现 EventPublisher 接口
func (a *EventPublisherAdapter) Publish(ctx context.Context, topic string, event event.Event) error {
    if event == nil {
        return fmt.Errorf("event cannot be nil")
    }
    
    payload, err := event.MarshalJSON()
    if err != nil {
        return fmt.Errorf("failed to marshal event: %w", err)
    }
    
    opts := eventbus.PublishOptions{
        AggregateID: event.GetAggregateID(),
        Metadata: map[string]string{
            "eventType": event.GetEventType(),
            "eventID":   event.GetEventID(),
        },
        Timeout: 30 * time.Second,
    }
    
    return a.advancedBus.PublishWithOptions(ctx, topic, payload, opts)
}

func (a *EventPublisherAdapter) RegisterReconnectCallback(callback func(ctx context.Context) error) error {
    return a.advancedBus.RegisterReconnectCallback(callback)
}

// 业务回调处理
func (a *EventPublisherAdapter) handleReconnect(ctx context.Context) error {
    log.Println("Kafka publisher reconnected successfully")
    // 这里可以添加业务特定的重连处理逻辑
    return nil
}

func (a *EventPublisherAdapter) handlePublishResult(ctx context.Context, topic string, message []byte, err error) error {
    if err != nil {
        log.Printf("Publish failed for topic %s: %v", topic, err)
    }
    return nil
}

func (a *EventPublisherAdapter) handleHealthCheck(ctx context.Context, result eventbus.HealthCheckResult) error {
    if !result.Success {
        log.Printf("Health check failed: %v", result.Error)
        // 可以发送告警、更新监控指标等
    }
    return nil
}
```

### 阶段 3：更新依赖注入（第3天）

#### 3.1 更新发布器注册

**原有代码** (`evidence-management/command/internal/infrastructure/eventbus/publisher.go`):
```go
func registerKafkaEventPublisherDependencies() {
    err := di.Provide(func() publisher.EventPublisher {
        return &KafkaEventPublisher{
            kafkaManager: eventbus.DefaultKafkaPublisherManager, // 删除
        }
    })
}
```

**新代码**:
```go
func registerKafkaEventPublisherDependencies() {
    // 注册高级事件总线
    err := di.Provide(func() (eventbus.AdvancedEventBus, error) {
        config := GetEventBusConfig()
        return eventbus.NewKafkaAdvancedEventBus(config)
    })
    if err != nil {
        logger.Fatalf("failed to provide AdvancedEventBus: %v", err)
    }
    
    // 注册适配器作为 EventPublisher
    err = di.Provide(func(bus eventbus.AdvancedEventBus) publisher.EventPublisher {
        return NewEventPublisherAdapter(bus)
    })
    if err != nil {
        logger.Fatalf("failed to provide EventPublisher: %v", err)
    }
}
```

#### 3.2 更新应用启动代码

**文件**: `evidence-management/cmd/command/main.go`

```go
func main() {
    // 初始化配置
    config := GetEventBusConfig()
    
    // 创建高级事件总线
    bus, err := eventbus.NewKafkaAdvancedEventBus(config)
    if err != nil {
        log.Fatal("Failed to create advanced event bus:", err)
    }
    
    // 启动健康检查（jxt-core 统一管理）
    if err := bus.StartHealthCheck(context.Background()); err != nil {
        log.Fatal("Failed to start health check:", err)
    }
    
    // 注册到 DI 容器
    di.Provide(func() eventbus.AdvancedEventBus {
        return bus
    })
    
    // 其他初始化逻辑...
}
```

### 阶段 4：清理旧代码（第4天）

#### 4.1 删除旧的 KafkaPublisherManager

```bash
# 备份旧代码
mv evidence-management/shared/common/eventbus/kafka_publisher_manager.go \
   evidence-management/shared/common/eventbus/kafka_publisher_manager.go.backup

# 删除相关的全局变量和初始化代码
```

#### 4.2 更新导入和引用

搜索并更新所有对 `KafkaPublisherManager` 的引用：

```bash
# 搜索需要更新的文件
grep -r "KafkaPublisherManager" evidence-management/
grep -r "DefaultKafkaPublisherManager" evidence-management/
```

### 阶段 5：测试验证（第5天）

#### 5.1 单元测试

```go
// evidence-management/shared/common/eventbus/publisher_adapter_test.go
func TestEventPublisherAdapter(t *testing.T) {
    // 创建模拟的高级事件总线
    mockBus := &MockAdvancedEventBus{}
    adapter := NewEventPublisherAdapter(mockBus)
    
    // 测试 PublishMessage 方法
    err := adapter.PublishMessage("test-topic", "test-uuid", int64(123), []byte("test payload"))
    assert.NoError(t, err)
    assert.True(t, mockBus.PublishWithOptionsCalled)
    
    // 验证消息格式化
    assert.Equal(t, "123", mockBus.LastOptions.AggregateID)
}

func TestEvidenceMessageFormatter(t *testing.T) {
    formatter := &EvidenceMessageFormatter{}
    
    msg, err := formatter.FormatMessage("uuid-123", int64(456), []byte("payload"))
    assert.NoError(t, err)
    assert.Equal(t, "456", msg.Metadata["aggregate_id"])
    assert.Equal(t, "456", msg.Metadata["aggregateID"])
}
```

#### 5.2 集成测试

```go
func TestHealthCheckIntegration(t *testing.T) {
    config := GetTestEventBusConfig()
    bus, err := eventbus.NewKafkaAdvancedEventBus(config)
    require.NoError(t, err)
    
    // 启动健康检查
    err = bus.StartHealthCheck(context.Background())
    require.NoError(t, err)
    
    // 等待健康检查执行
    time.Sleep(5 * time.Second)
    
    // 验证健康状态
    status := bus.GetHealthStatus()
    assert.True(t, status.IsRunning)
    
    // 验证健康检查消息使用统一主题
    // 这里需要验证消息确实发布到了 jxt-core 统一的健康检查主题
}
```

#### 5.3 端到端测试

```go
func TestEventPublishingE2E(t *testing.T) {
    // 创建完整的事件发布流程测试
    config := GetTestEventBusConfig()
    bus, err := eventbus.NewKafkaAdvancedEventBus(config)
    require.NoError(t, err)
    
    adapter := NewEventPublisherAdapter(bus)
    
    // 测试业务事件发布
    event := &MediaUploadedEvent{
        MediaID: "test-media-123",
        // ... 其他字段
    }
    
    err = adapter.Publish(context.Background(), "media-events", event)
    assert.NoError(t, err)
    
    // 验证消息格式正确
    // 验证元数据包含正确的 aggregate_id
}
```

## 验证清单

迁移完成后，请验证以下功能：

### ✅ 功能验证
- [ ] 事件发布功能正常
- [ ] 消息格式与原有代码兼容（aggregate_id 字段）
- [ ] 聚合ID类型转换正确（int64 -> string）
- [ ] 业务事件序列化正常
- [ ] 错误处理机制正常

### ✅ 健康检查验证
- [ ] 健康检查使用 jxt-core 统一主题
- [ ] 健康检查消息格式标准化
- [ ] 健康检查状态回调正常
- [ ] 健康检查失败时的处理正确

### ✅ 重连机制验证
- [ ] 连接失败时自动重连
- [ ] 指数退避算法正常
- [ ] 重连成功后回调执行
- [ ] 重连过程中的错误处理

### ✅ 性能验证
- [ ] 发布性能不低于原有实现
- [ ] 内存使用合理
- [ ] 并发发布正常
- [ ] 无内存泄漏

### ✅ 监控验证
- [ ] 健康检查指标正常
- [ ] 发布成功/失败指标正常
- [ ] 重连次数指标正常
- [ ] 日志记录完整

## 回滚计划

如果迁移过程中出现问题：

```bash
#!/bin/bash
echo "开始回滚发布器迁移..."

# 1. 停止应用
kubectl scale deployment evidence-management-command --replicas=0

# 2. 恢复旧代码
mv evidence-management/shared/common/eventbus/kafka_publisher_manager.go.backup \
   evidence-management/shared/common/eventbus/kafka_publisher_manager.go

# 3. 恢复旧的依赖注入
git checkout HEAD~1 -- evidence-management/command/internal/infrastructure/eventbus/publisher.go

# 4. 恢复旧配置
git checkout HEAD~1 -- evidence-management/config/

# 5. 重新构建和部署
docker build -t evidence-management:rollback .
kubectl set image deployment/evidence-management-command app=evidence-management:rollback

# 6. 重启应用
kubectl scale deployment evidence-management-command --replicas=3

echo "回滚完成"
```

## 常见问题

### Q1: 健康检查主题变更后，监控系统如何适配？
A1: 更新监控配置，监听新的统一健康检查主题 `jxt-core-kafka-health-check`。

### Q2: 消息格式化器如何处理不同类型的聚合ID？
A2: `EvidenceMessageFormatter` 继承了 `DefaultMessageFormatter`，支持 string、int64、int 等类型的自动转换。

### Q3: 如何确保与现有 Outbox 模式兼容？
A3: 适配器保持了原有的 `Publish` 接口，Outbox 调度器无需修改。

### Q4: 迁移后如何调试发布问题？
A4: 通过注册 `PublishCallback` 可以监控每次发布的结果，便于调试和监控。

通过遵循这个迁移指南，可以安全、有序地将发布端迁移到新的高级事件总线架构，同时保持与现有业务代码的兼容性。
