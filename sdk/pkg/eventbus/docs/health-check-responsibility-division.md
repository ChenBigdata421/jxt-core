# EventBus 健康检查职责分工方案

## 📋 概述

本文档定义了 jxt-core EventBus 和业务微服务（如 evidence-management）在健康检查功能上的职责分工，旨在避免重复实现，提高代码复用性，同时保持基础设施和业务逻辑的清晰分离。

## 🎯 设计原则

### 1. **分层职责原则**
- **jxt-core**：负责基础设施层健康检查，提供通用的、与业务无关的健康检查能力
- **业务微服务**：负责业务层健康检查，实现特定于业务逻辑的健康指标

### 2. **复用性原则**
- 基础设施层的健康检查应该能被所有微服务复用
- 避免每个微服务都重复实现相同的基础健康检查逻辑

### 3. **扩展性原则**
- 提供标准接口，允许业务微服务注册自定义健康检查
- 支持组合式健康检查，基础设施 + 业务健康检查

## 🏗️ 职责分工

### 📤 **jxt-core 负责的健康检查**

#### 1. **基础连接检查**
```go
// Kafka/NATS 连接状态检查
func (e *EventBus) CheckConnection(ctx context.Context) error {
    // 检查客户端连接状态
    // 验证 broker 可达性
    // 检查认证状态
}
```

**职责范围**：
- Kafka/NATS 客户端连接状态
- Broker 可达性验证
- 认证和授权状态检查
- 网络连接质量评估

#### 2. **端到端消息传输测试**
```go
// 消息发布/订阅能力验证
func (e *EventBus) CheckMessageTransport(ctx context.Context) error {
    // 发布健康检查消息
    // 验证消息传输延迟
    // 检查消息完整性
}
```

**职责范围**：
- 发布能力测试
- 订阅能力测试
- 消息传输延迟测量
- 消息完整性验证

#### 3. **EventBus 性能指标**
```go
type EventBusHealthMetrics struct {
    ConnectionStatus     string        `json:"connectionStatus"`
    PublishLatency      time.Duration `json:"publishLatency"`
    SubscribeLatency    time.Duration `json:"subscribeLatency"`
    LastSuccessTime     time.Time     `json:"lastSuccessTime"`
    ConsecutiveFailures int           `json:"consecutiveFailures"`
    ThroughputPerSecond int64         `json:"throughputPerSecond"`
}
```

**职责范围**：
- 连接状态监控
- 性能指标收集
- 故障统计
- 吞吐量监控

#### 4. **自动恢复机制**
```go
// 连接故障自动恢复
func (e *EventBus) AutoReconnect(ctx context.Context) error {
    // 检测连接故障
    // 执行重连策略
    // 恢复订阅状态
}
```

**职责范围**：
- 连接故障检测
- 自动重连逻辑
- 订阅状态恢复
- 故障通知机制

#### 5. **标准健康检查接口**
```go
type EventBusHealthChecker interface {
    // 基础健康检查
    CheckConnection(ctx context.Context) error
    CheckMessageTransport(ctx context.Context) error
    
    // 指标获取
    GetMetrics() EventBusHealthMetrics
    
    // 业务健康检查注册
    RegisterBusinessHealthCheck(checker BusinessHealthChecker)
    
    // 综合健康检查
    HealthCheck(ctx context.Context) (*HealthStatus, error)
}
```

### 📥 **业务微服务负责的健康检查**

#### 1. **业务数据健康检查**
```go
// evidence-management 示例
func (s *EvidenceHealthChecker) CheckBusinessHealth(ctx context.Context) error {
    // 检查 Outbox 事件积压
    if err := s.checkOutboxBacklog(ctx); err != nil {
        return err
    }
    
    // 检查租户数据一致性
    if err := s.checkTenantDataConsistency(ctx); err != nil {
        return err
    }
    
    return nil
}
```

**职责范围**：
- Outbox 模式事件积压检查
- 租户数据库状态检查
- 业务数据一致性验证
- 业务流程完整性检查

#### 2. **业务性能指标**
```go
type EvidenceBusinessMetrics struct {
    MediaProcessingBacklog   int64         `json:"mediaProcessingBacklog"`
    ArchiveCreationLatency  time.Duration `json:"archiveCreationLatency"`
    QueryResponseTime       time.Duration `json:"queryResponseTime"`
    TenantDataConsistency   bool          `json:"tenantDataConsistency"`
    UnpublishedEventCount   int64         `json:"unpublishedEventCount"`
}
```

**职责范围**：
- 业务处理延迟监控
- 业务队列积压统计
- 业务响应时间测量
- 业务数据质量指标

#### 3. **业务健康端点**
```go
// HTTP 业务健康检查端点
func (h *EvidenceHealthHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
    // 组合基础设施 + 业务健康检查
    baseHealth := h.eventBus.HealthCheck(ctx)
    businessHealth := h.businessChecker.CheckBusinessHealth(ctx)
    
    // 返回综合健康状态
}
```

**职责范围**：
- 业务特有的 HTTP 端点
- 业务监控数据暴露
- 业务告警阈值配置
- 业务健康状态聚合

#### 4. **业务告警和恢复策略**
```go
type BusinessHealthConfig struct {
    OutboxBacklogThreshold    int64         `json:"outboxBacklogThreshold"`
    MaxProcessingLatency     time.Duration `json:"maxProcessingLatency"`
    DataConsistencyCheck     bool          `json:"dataConsistencyCheck"`
    AlertWebhookURL          string        `json:"alertWebhookURL"`
}
```

**职责范围**：
- 业务告警阈值设定
- 业务恢复策略定义
- 业务监控配置管理
- 业务特有的故障处理

## 🔄 集成架构

### **jxt-core 提供的接口**
```go
// 业务健康检查接口
type BusinessHealthChecker interface {
    CheckBusinessHealth(ctx context.Context) error
    GetBusinessMetrics() interface{}
    GetBusinessConfig() interface{}
}

// 健康状态结构
type HealthStatus struct {
    Overall           string                 `json:"overall"`
    Infrastructure    InfrastructureHealth   `json:"infrastructure"`
    Business          interface{}            `json:"business,omitempty"`
    Timestamp         time.Time              `json:"timestamp"`
    CheckDuration     time.Duration          `json:"checkDuration"`
}

type InfrastructureHealth struct {
    EventBus          EventBusHealthMetrics  `json:"eventBus"`
    Database          DatabaseHealth         `json:"database,omitempty"`
    Cache             CacheHealth            `json:"cache,omitempty"`
}
```

### **业务微服务集成方式**
```go
// evidence-management 集成示例
func setupHealthCheck() {
    // 创建业务健康检查器
    evidenceChecker := &EvidenceHealthChecker{
        outboxService: outboxService,
        queryService:  queryService,
        config:        businessHealthConfig,
    }
    
    // 注册到 EventBus
    eventBus.RegisterBusinessHealthCheck(evidenceChecker)
    
    // 设置 HTTP 端点
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        status, err := eventBus.HealthCheck(r.Context())
        if err != nil {
            w.WriteHeader(http.StatusServiceUnavailable)
        } else {
            w.WriteHeader(http.StatusOK)
        }
        json.NewEncoder(w).Encode(status)
    })
}
```

## 📊 迁移计划

### **阶段一：jxt-core 基础能力建设**
1. **重构现有健康检查实现**
   - 移除空壳实现，添加真正的检查逻辑
   - 实现 Kafka/NATS 连接检查
   - 实现端到端消息传输测试

2. **建立标准接口**
   - 定义 EventBusHealthChecker 接口
   - 定义 BusinessHealthChecker 接口
   - 实现健康状态聚合逻辑

### **阶段二：evidence-management 重构**
1. **迁移基础设施检查到 jxt-core**
   - 移除 `subscriber_health_checker.go` 中的连接检查
   - 移除 `kafka_publisher_manager.go` 中的基础健康检查
   - 保留业务特有的健康检查逻辑

2. **实现业务健康检查接口**
   - 重构 `enhanced_event_resend_service.go` 的健康检查
   - 实现 BusinessHealthChecker 接口
   - 注册到 jxt-core EventBus

### **阶段三：标准化和推广**
1. **文档和最佳实践**
   - 编写健康检查开发指南
   - 提供业务健康检查模板
   - 建立监控和告警规范

2. **其他微服务适配**
   - 为其他微服务提供健康检查集成指导
   - 建立统一的健康检查标准
   - 完善监控和运维工具链

## 🎯 预期收益

### **代码复用性**
- 基础设施健康检查逻辑只需实现一次
- 新微服务可以直接复用 jxt-core 的健康检查能力
- 减少重复代码，提高开发效率

### **运维一致性**
- 统一的健康检查接口和响应格式
- 标准化的监控指标和告警机制
- 简化运维工具和流程

### **可维护性**
- 基础设施和业务逻辑清晰分离
- 健康检查逻辑集中管理和维护
- 便于问题定位和故障排查

### **扩展性**
- 支持新的消息中间件（如引入 Watermill）
- 支持新的业务健康检查需求
- 支持复杂的健康检查组合策略

## 🚀 实现状态

### ✅ **已完成的 jxt-core 健康检查功能**

#### 1. **核心接口和类型定义**
- `EventBusHealthChecker` 接口：提供完整的健康检查能力
- `BusinessHealthChecker` 接口：业务健康检查标准接口
- `HealthStatus` 结构：统一的健康状态响应格式
- `EventBusHealthMetrics` 结构：详细的 EventBus 性能指标

#### 2. **基础设施健康检查实现**
- **连接检查**：`CheckConnection()` - 验证 Kafka/NATS 连接状态
- **消息传输测试**：`CheckMessageTransport()` - 端到端消息发布测试
- **性能指标收集**：`GetEventBusMetrics()` - 获取连接、延迟、吞吐量等指标
- **综合健康检查**：`PerformHealthCheck()` - 组合基础设施和业务健康检查

#### 3. **Kafka 实现**
- 真实的 broker 连接检查
- 可用 broker 数量统计
- Topic 数量获取
- 发布延迟测量
- 连接状态实时监控

#### 4. **NATS 实现**
- NATS 连接状态检查
- JetStream 账户信息验证
- 连接 URL 和状态监控
- 订阅数量统计

#### 5. **HTTP 健康检查端点**
- `/health` - 完整健康检查（基础设施 + 业务）
- `/livez` - 存活检查（简单响应）
- `/readyz` - 就绪检查（连接状态）
- 标准化的 JSON 响应格式
- 适当的 HTTP 状态码

#### 6. **业务健康检查集成**
- `RegisterBusinessHealthCheck()` - 注册业务健康检查器
- 自动组合基础设施和业务健康状态
- 业务指标和配置信息收集

### 📝 **使用示例**

```go
// 1. 创建 EventBus
eventBus, err := NewEventBus(config)
if err != nil {
    panic(err)
}

// 2. 创建业务健康检查器
type MyBusinessHealthChecker struct{}

func (m *MyBusinessHealthChecker) CheckBusinessHealth(ctx context.Context) error {
    // 检查数据库连接、外部服务、业务队列等
    return nil
}

func (m *MyBusinessHealthChecker) GetBusinessMetrics() interface{} {
    return map[string]interface{}{
        "activeUsers": 1000,
        "queueSize":   50,
    }
}

// 3. 注册业务健康检查
if healthChecker, ok := eventBus.(EventBusHealthChecker); ok {
    healthChecker.RegisterBusinessHealthCheck(&MyBusinessHealthChecker{})
}

// 4. 设置 HTTP 端点
mux := http.NewServeMux()
SetupHealthCheckRoutes(mux, eventBus)

// 5. 启动服务器
http.ListenAndServe(":8080", mux)
```

### 🧪 **测试覆盖**
- 单元测试：基础健康检查功能
- 集成测试：业务健康检查集成
- HTTP 端点测试：各种健康检查端点
- 性能测试：健康检查性能基准
- 错误场景测试：连接失败、业务检查失败等

### 📊 **监控指标**
- 连接状态：connected/disconnected/reconnecting
- 发布延迟：消息发布响应时间
- 吞吐量：每秒处理的消息数量
- 错误统计：连续失败次数、最后失败时间
- 资源统计：Broker 数量、Topic 数量
- 业务指标：由业务健康检查器提供

## 📝 总结

通过明确的职责分工，jxt-core 专注于提供稳定可靠的基础设施健康检查能力，业务微服务专注于实现业务特有的健康检查逻辑。这种分层架构既避免了重复实现，又保持了良好的扩展性和可维护性，为构建健壮的微服务监控体系奠定了基础。

**当前实现已经提供了企业级的健康检查能力**，包括完整的连接检查、消息传输测试、性能指标收集和业务健康检查集成，为微服务监控和运维提供了坚实的基础。
