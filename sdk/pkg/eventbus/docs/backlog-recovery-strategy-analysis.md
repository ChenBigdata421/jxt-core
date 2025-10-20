# 消息积压恢复策略设计分析

## 🎯 **核心问题：恢复逻辑应该由谁负责？**

### 📊 **方案对比**

| 维度 | 业务层实现 | EventBus 实现 |
|------|------------|---------------|
| **职责分离** | ✅ 清晰分离 | ❌ 职责混合 |
| **灵活性** | ✅ 高度灵活 | ❌ 固化策略 |
| **可扩展性** | ✅ 易于扩展 | ❌ 难以扩展 |
| **业务适配** | ✅ 完美适配 | ❌ 通用妥协 |
| **测试性** | ✅ 独立测试 | ❌ 耦合测试 |
| **维护性** | ✅ 独立维护 | ❌ 框架绑定 |

---

## 🏗️ **推荐架构：业务层实现恢复逻辑**

### 1. **职责分工**

#### jxt-core EventBus 职责（基础设施层）
```go
// 积压检测和通知
type BacklogDetector interface {
    // 检测积压状态
    IsNoBacklog(ctx context.Context) (bool, error)
    
    // 获取详细积压信息
    GetBacklogInfo(ctx context.Context) (*BacklogInfo, error)
    
    // 注册状态变化回调
    RegisterCallback(callback BacklogStateCallback) error
    
    // 启动/停止监控
    Start(ctx context.Context) error
    Stop() error
}

// 状态通知，不包含恢复逻辑
type BacklogState struct {
    HasBacklog    bool              `json:"hasBacklog"`
    ConsumerGroup string            `json:"consumerGroup"`
    LagCount      int64             `json:"lagCount"`
    LagTime       time.Duration     `json:"lagTime"`
    Timestamp     time.Time         `json:"timestamp"`
    Topic         string            `json:"topic"`
    Details       map[string]interface{} `json:"details,omitempty"`
}
```

#### 业务层职责（业务逻辑层）
```go
// 业务层实现恢复策略
type BacklogRecoveryManager struct {
    strategy RecoveryStrategy
    state    RecoveryState
    metrics  RecoveryMetrics
}

// 不同的恢复策略
type RecoveryStrategy interface {
    ShouldEnterRecovery(state eventbus.BacklogState) bool
    ShouldExitRecovery(state eventbus.BacklogState) bool
    ProcessMessage(msg *eventbus.Message) error
}
```

### 2. **多样化的恢复策略**

#### 策略1：有序恢复（evidence-management 风格）
```go
type OrderedRecoveryStrategy struct {
    noBacklogCount    int
    requiredCount     int
    processorManager  *AggregateProcessorManager
}

func (s *OrderedRecoveryStrategy) ShouldEnterRecovery(state eventbus.BacklogState) bool {
    return state.HasBacklog
}

func (s *OrderedRecoveryStrategy) ShouldExitRecovery(state eventbus.BacklogState) bool {
    if !state.HasBacklog {
        s.noBacklogCount++
        return s.noBacklogCount >= s.requiredCount
    }
    s.noBacklogCount = 0
    return false
}

func (s *OrderedRecoveryStrategy) ProcessMessage(msg *eventbus.Message) error {
    aggregateID := msg.Headers["aggregateId"]
    processor := s.processorManager.GetOrCreate(aggregateID)
    return processor.ProcessInOrder(msg)
}
```

#### 策略2：限流恢复
```go
type RateLimitedRecoveryStrategy struct {
    normalRate    rate.Limit
    recoveryRate  rate.Limit
    limiter       *rate.Limiter
}

func (s *RateLimitedRecoveryStrategy) ShouldEnterRecovery(state eventbus.BacklogState) bool {
    if state.HasBacklog {
        s.limiter.SetLimit(s.recoveryRate)
        return true
    }
    return false
}

func (s *RateLimitedRecoveryStrategy) ProcessMessage(msg *eventbus.Message) error {
    s.limiter.Wait(context.Background())
    return s.processNormally(msg)
}
```

#### 策略3：优先级恢复
```go
type PriorityRecoveryStrategy struct {
    priorityQueue *PriorityQueue
    batchSize     int
}

func (s *PriorityRecoveryStrategy) ProcessMessage(msg *eventbus.Message) error {
    priority := s.calculatePriority(msg)
    s.priorityQueue.Push(msg, priority)
    
    if s.priorityQueue.Size() >= s.batchSize {
        return s.processBatch()
    }
    return nil
}
```

#### 策略4：分片恢复
```go
type ShardedRecoveryStrategy struct {
    shardCount    int
    activeShards  map[int]bool
}

func (s *ShardedRecoveryStrategy) ProcessMessage(msg *eventbus.Message) error {
    shard := s.calculateShard(msg)
    if s.activeShards[shard] {
        return s.processInShard(msg, shard)
    }
    return s.processNormally(msg)
}
```

### 3. **统一的恢复管理器**

```go
type BacklogRecoveryManager struct {
    strategy      RecoveryStrategy
    isRecovering  atomic.Bool
    metrics       *RecoveryMetrics
    logger        *zap.Logger
}

// 处理积压状态变化
func (rm *BacklogRecoveryManager) HandleBacklogState(ctx context.Context, state eventbus.BacklogState) error {
    currentlyRecovering := rm.isRecovering.Load()
    
    if !currentlyRecovering && rm.strategy.ShouldEnterRecovery(state) {
        rm.enterRecoveryMode(state)
    } else if currentlyRecovering && rm.strategy.ShouldExitRecovery(state) {
        rm.exitRecoveryMode(state)
    }
    
    rm.updateMetrics(state)
    return nil
}

// 消息处理入口
func (rm *BacklogRecoveryManager) ProcessMessage(msg *eventbus.Message) error {
    if rm.isRecovering.Load() {
        return rm.strategy.ProcessMessage(msg)
    }
    return rm.processNormally(msg)
}
```

---

## 🎯 **为什么业务层实现更好？**

### 1. **业务场景多样性**

#### 电商场景
```go
// 订单处理：需要严格有序
type OrderRecoveryStrategy struct {
    // 按用户ID分组，保证单用户订单有序
}

// 库存更新：需要优先级处理
type InventoryRecoveryStrategy struct {
    // 高价值商品优先处理
}
```

#### 金融场景
```go
// 交易处理：需要时间窗口控制
type TradingRecoveryStrategy struct {
    // 交易时间内快速恢复，非交易时间慢速处理
}

// 风控检查：需要分级处理
type RiskControlRecoveryStrategy struct {
    // 高风险交易优先处理
}
```

#### 物联网场景
```go
// 设备数据：需要地理位置分片
type IoTRecoveryStrategy struct {
    // 按地理区域分片恢复
}

// 告警消息：需要紧急程度分级
type AlertRecoveryStrategy struct {
    // 紧急告警优先处理
}
```

### 2. **配置灵活性**

```go
// 业务层可以灵活配置恢复策略
type RecoveryConfig struct {
    Strategy    string        `yaml:"strategy"`    // "ordered", "rate-limited", "priority"
    Parameters  map[string]interface{} `yaml:"parameters"`
    
    // 有序恢复配置
    OrderedConfig struct {
        RequiredNoBacklogCount int           `yaml:"requiredNoBacklogCount"`
        ProcessorCacheSize     int           `yaml:"processorCacheSize"`
        ProcessorIdleTimeout   time.Duration `yaml:"processorIdleTimeout"`
    } `yaml:"orderedConfig"`
    
    // 限流恢复配置
    RateLimitConfig struct {
        NormalRate   rate.Limit `yaml:"normalRate"`
        RecoveryRate rate.Limit `yaml:"recoveryRate"`
    } `yaml:"rateLimitConfig"`
    
    // 优先级恢复配置
    PriorityConfig struct {
        BatchSize      int               `yaml:"batchSize"`
        PriorityRules  []PriorityRule    `yaml:"priorityRules"`
    } `yaml:"priorityConfig"`
}
```

### 3. **测试和维护优势**

#### 独立测试
```go
func TestOrderedRecoveryStrategy(t *testing.T) {
    strategy := &OrderedRecoveryStrategy{requiredCount: 3}
    
    // 测试进入恢复模式
    assert.True(t, strategy.ShouldEnterRecovery(backlogState))
    
    // 测试退出恢复模式
    for i := 0; i < 3; i++ {
        result := strategy.ShouldExitRecovery(noBacklogState)
        if i == 2 {
            assert.True(t, result)
        } else {
            assert.False(t, result)
        }
    }
}
```

#### 独立维护
```go
// 业务团队可以独立维护恢复策略
// 不需要修改 jxt-core EventBus 代码
// 可以快速响应业务需求变化
```

---

## 🔧 **实现指南**

### 1. **EventBus 提供的基础能力**

```go
// jxt-core EventBus 只负责检测和通知
func main() {
    // 初始化 EventBus
    bus := eventbus.GetGlobal()
    
    // 注册积压状态回调
    bus.RegisterBacklogCallback(recoveryManager.HandleBacklogState)
    
    // 启动积压监控
    bus.StartBacklogMonitoring(ctx)
    
    // 正常订阅消息
    bus.Subscribe("topic", recoveryManager.ProcessMessage)
}
```

### 2. **业务层实现恢复逻辑**

```go
// 业务层创建恢复管理器
func NewRecoveryManager(config RecoveryConfig) *BacklogRecoveryManager {
    var strategy RecoveryStrategy
    
    switch config.Strategy {
    case "ordered":
        strategy = NewOrderedRecoveryStrategy(config.OrderedConfig)
    case "rate-limited":
        strategy = NewRateLimitedRecoveryStrategy(config.RateLimitConfig)
    case "priority":
        strategy = NewPriorityRecoveryStrategy(config.PriorityConfig)
    default:
        strategy = NewDefaultRecoveryStrategy()
    }
    
    return &BacklogRecoveryManager{
        strategy: strategy,
        metrics:  NewRecoveryMetrics(),
        logger:   logger.Logger,
    }
}
```

### 3. **可插拔的策略系统**

```go
// 支持运行时切换策略
func (rm *BacklogRecoveryManager) SwitchStrategy(newStrategy RecoveryStrategy) error {
    rm.mu.Lock()
    defer rm.mu.Unlock()
    
    // 等待当前处理完成
    rm.waitForCurrentProcessing()
    
    // 切换策略
    oldStrategy := rm.strategy
    rm.strategy = newStrategy
    
    rm.logger.Info("Recovery strategy switched",
        zap.String("from", reflect.TypeOf(oldStrategy).Name()),
        zap.String("to", reflect.TypeOf(newStrategy).Name()))
    
    return nil
}
```

---

## 📈 **收益分析**

### 1. **架构收益**
- ✅ **清晰的职责分离**：基础设施与业务逻辑分离
- ✅ **高度的可扩展性**：支持任意恢复策略
- ✅ **良好的可测试性**：策略可以独立测试

### 2. **业务收益**
- ✅ **灵活的策略选择**：根据业务场景选择最适合的策略
- ✅ **快速的需求响应**：业务团队可以独立开发新策略
- ✅ **精确的业务适配**：策略完全贴合业务需求

### 3. **技术收益**
- ✅ **框架的稳定性**：EventBus 专注于基础功能，更稳定
- ✅ **代码的可维护性**：业务逻辑与框架代码分离
- ✅ **系统的可演进性**：策略可以独立演进

---

## 🎯 **总结**

**推荐方案**：消息积压后的恢复应该由**业务层实现**

**核心原因**：
1. **职责分离**：EventBus 专注基础设施，业务层处理业务逻辑
2. **灵活性**：不同业务场景需要不同的恢复策略
3. **可扩展性**：业务团队可以独立开发和维护恢复策略
4. **可测试性**：恢复逻辑可以独立测试和验证

**实现方式**：
- jxt-core EventBus 提供积压检测和状态通知
- 业务层通过回调机制接收状态变化
- 业务层实现具体的恢复策略和处理逻辑

这种设计既保持了框架的通用性和稳定性，又提供了业务层的灵活性和可扩展性，是最佳的架构选择。
