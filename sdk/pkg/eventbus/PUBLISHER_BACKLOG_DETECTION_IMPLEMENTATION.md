# 发送端积压检测实现总结

## 概述

我们成功实现了 EventBus 的发送端积压检测功能，并根据配置文件来决定是否同时启动发送端和订阅端的积压检测。

## 主要功能

### 1. 发送端积压检测器 (PublisherBacklogDetector)

**文件**: `jxt-core/sdk/pkg/eventbus/publisher_backlog_detector.go`

**核心功能**:
- **队列深度监控**: 监控消息队列的深度，防止队列过载
- **发送延迟监控**: 跟踪消息发送的平均延迟
- **发送速率监控**: 监控消息发送速率，检测是否超过阈值
- **积压比例计算**: 综合多个指标计算积压比例 (0.0-1.0)
- **严重程度评估**: 根据积压比例评估严重程度 (NORMAL, LOW, MEDIUM, HIGH, CRITICAL)

**配置参数**:
```go
type PublisherBacklogDetectionConfig struct {
    Enabled           bool          // 是否启用
    MaxQueueDepth     int64         // 最大队列深度
    MaxPublishLatency time.Duration // 最大发送延迟
    RateThreshold     float64       // 发送速率阈值 (msg/sec)
    CheckInterval     time.Duration // 检测间隔
}
```

### 2. 配置结构更新

**文件**: `jxt-core/sdk/config/eventbus.go`

**新增配置类型**:
- `PublisherBacklogDetectionConfig`: 发送端积压检测配置
- `SubscriberBacklogDetectionConfig`: 订阅端积压检测配置（重命名）

**配置层次结构**:
```yaml
eventbus:
  publisher:
    backlogDetection:
      enabled: true
      maxQueueDepth: 1000
      maxPublishLatency: 5s
      rateThreshold: 500.0
      checkInterval: 30s
  
  subscriber:
    backlogDetection:
      enabled: true
      maxLagThreshold: 1000
      maxTimeThreshold: 5m
      checkInterval: 30s
```

### 3. EventBus 接口扩展

**文件**: `jxt-core/sdk/pkg/eventbus/type.go`

**新增方法**:
```go
// 发送端积压检测
RegisterPublisherBacklogCallback(callback PublisherBacklogCallback) error
StartPublisherBacklogMonitoring(ctx context.Context) error
StopPublisherBacklogMonitoring() error

// 统一积压检测管理
StartAllBacklogMonitoring(ctx context.Context) error
StopAllBacklogMonitoring() error
```

### 4. 实现类更新

#### Kafka EventBus (`kafka.go`)
- 添加 `publisherBacklogDetector` 字段
- 添加 `fullConfig` 字段用于访问完整配置
- 实现根据配置启动相应的积压检测器
- 支持同时启动发送端和订阅端检测

#### NATS EventBus (`nats.go`)
- 添加 `publisherBacklogDetector` 字段
- 添加 `fullConfig` 字段用于访问完整配置
- 实现根据配置启动相应的积压检测器
- 支持同时启动发送端和订阅端检测

#### Memory EventBus (`eventbus.go`)
- 添加占位符实现，支持接口兼容性

## 核心特性

### 1. 配置驱动的启动策略

系统根据配置文件自动决定启动哪些积压检测器：

```go
// 只启动发送端检测
if fullConfig.Enterprise.Publisher.BacklogDetection.Enabled {
    // 初始化发送端积压检测器
}

// 只启动订阅端检测
if fullConfig.Enterprise.Subscriber.BacklogDetection.Enabled {
    // 初始化订阅端积压检测器
}

// 同时启动两者
// 两个 if 条件都满足时，会同时启动
```

### 2. 智能积压检测算法

发送端积压检测器使用多维度指标：

```go
func (pbd *PublisherBacklogDetector) isBacklogged(queueDepth int64, avgLatency time.Duration, publishRate float64) bool {
    // 队列深度检查
    if pbd.maxQueueDepth > 0 && queueDepth > pbd.maxQueueDepth {
        return true
    }
    
    // 发送延迟检查
    if pbd.maxPublishLatency > 0 && avgLatency > pbd.maxPublishLatency {
        return true
    }
    
    // 发送速率检查
    if pbd.rateThreshold > 0 && publishRate > pbd.rateThreshold {
        return true
    }
    
    return false
}
```

### 3. 回调机制

支持注册回调函数来处理积压状态变化：

```go
type PublisherBacklogCallback func(ctx context.Context, state PublisherBacklogState) error

type PublisherBacklogState struct {
    HasBacklog        bool          // 是否有积压
    QueueDepth        int64         // 队列深度
    PublishRate       float64       // 发送速率
    AvgPublishLatency time.Duration // 平均发送延迟
    BacklogRatio      float64       // 积压比例 (0.0-1.0)
    Timestamp         time.Time     // 时间戳
    Severity          string        // 严重程度
}
```

## 使用场景

### 1. 高并发写入系统
- **场景**: 日志收集、监控数据上报
- **配置**: 较高的 `maxQueueDepth` 和 `rateThreshold`
- **应对**: 动态调整批量大小、启用压缩

### 2. 实时数据流处理
- **场景**: IoT 数据、金融交易数据
- **配置**: 较低的 `maxPublishLatency`
- **应对**: 快速切换到备用队列、降级处理

### 3. 关键业务系统
- **场景**: 支付、订单处理
- **配置**: 平衡的阈值设置
- **应对**: 业务降级、熔断保护

## 配置示例

### 生产环境配置
```yaml
eventbus:
  type: "kafka"
  publisher:
    backlogDetection:
      enabled: true
      maxQueueDepth: 2000
      maxPublishLatency: 10s
      rateThreshold: 1000.0
      checkInterval: 60s
  
  subscriber:
    backlogDetection:
      enabled: true
      maxLagThreshold: 2000
      maxTimeThreshold: 10m
      checkInterval: 60s
```

### 开发环境配置
```yaml
eventbus:
  type: "memory"
  publisher:
    backlogDetection:
      enabled: false
  
  subscriber:
    backlogDetection:
      enabled: false
```

### 测试环境配置
```yaml
eventbus:
  type: "kafka"
  publisher:
    backlogDetection:
      enabled: true
      maxQueueDepth: 100
      maxPublishLatency: 1s
      rateThreshold: 100.0
      checkInterval: 10s
  
  subscriber:
    backlogDetection:
      enabled: true
      maxLagThreshold: 100
      maxTimeThreshold: 1m
      checkInterval: 10s
```

## API 使用示例

```go
// 初始化 EventBus
bus, err := eventbus.NewEventBus(config)

// 注册回调
bus.RegisterPublisherBacklogCallback(handlePublisherBacklog)
bus.RegisterBacklogCallback(handleSubscriberBacklog)

// 启动所有积压监控
bus.StartAllBacklogMonitoring(ctx)

// 或者分别启动
bus.StartPublisherBacklogMonitoring(ctx)
bus.StartBacklogMonitoring(ctx)

// 停止所有积压监控
bus.StopAllBacklogMonitoring()
```

## 技术优势

1. **配置驱动**: 根据配置自动决定启动策略
2. **多维度监控**: 综合队列深度、延迟、速率等指标
3. **智能评估**: 自动计算积压比例和严重程度
4. **回调机制**: 支持自定义积压处理策略
5. **统一接口**: 提供统一的启动/停止方法
6. **向后兼容**: 保持与现有代码的兼容性

## 文件清单

- `publisher_backlog_detector.go`: 发送端积压检测器实现
- `config/eventbus.go`: 配置结构定义
- `type.go`: 接口和类型定义
- `kafka.go`: Kafka EventBus 实现
- `nats.go`: NATS EventBus 实现
- `eventbus.go`: 通用 EventBus 管理器
- `examples/backlog_detection_config_example.yaml`: 配置示例
- `examples/backlog_detection_usage_example.go`: 使用示例

## 总结

我们成功实现了完整的发送端积压检测功能，支持根据配置文件灵活启动发送端和/或订阅端的积压检测。这个实现提供了强大的监控能力和灵活的配置选项，能够满足不同场景下的需求。
