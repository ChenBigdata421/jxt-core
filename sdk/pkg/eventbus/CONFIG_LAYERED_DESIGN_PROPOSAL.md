# EventBus配置分层设计方案

## 🎯 **设计原则**

您的理解完全正确！配置应该分为两层：

### **用户配置层** (sdk/config/eventbus.go)
- **目的**: 暴露给最终用户的简化配置
- **特点**: 字段较少，用户友好，业务导向
- **使用者**: 业务开发人员、运维人员

### **程序员配置层** (sdk/pkg/eventbus/type.go)  
- **目的**: 程序员内部使用的完整配置
- **特点**: 字段丰富，技术导向，包含默认值和计算值
- **使用者**: EventBus组件内部代码

## 📋 **当前问题分析**

### 问题1: 配置职责混乱
```go
// ❌ 当前sdk/config/eventbus.go - 过于复杂
type ProducerConfig struct {
    RequiredAcks    int           // 用户需要配置
    Compression     string        // 用户需要配置  
    FlushFrequency  time.Duration // 用户需要配置
    FlushMessages   int           // 用户需要配置
    FlushBytes      int           // 程序员应该控制
    RetryMax        int           // 程序员应该控制
    Timeout         time.Duration // 用户需要配置
    BatchSize       int           // 程序员应该控制
    BufferSize      int           // 程序员应该控制
    Idempotent      bool          // 程序员应该控制
    MaxMessageBytes int           // 程序员应该控制
    PartitionerType string        // 程序员应该控制
}
```

### 问题2: type.go配置不完整
```go
// ❌ 当前sdk/pkg/eventbus/type.go - 字段缺失
type ProducerConfig struct {
    RequiredAcks   int           // ✅ 有
    Compression    string        // ✅ 有
    FlushFrequency time.Duration // ✅ 有
    FlushMessages  int           // ✅ 有
    RetryMax       int           // ✅ 有
    Timeout        time.Duration // ✅ 有
    BatchSize      int           // ✅ 有
    BufferSize     int           // ✅ 有
    // ❌ 缺失程序员控制的字段:
    // FlushBytes, Idempotent, MaxMessageBytes, PartitionerType
}
```

## 🔧 **正确的分层设计**

### **用户配置层** (sdk/config/eventbus.go)
```go
// ✅ 简化的用户配置 - 只包含用户需要关心的字段
type ProducerConfig struct {
    // 基础配置 - 用户可配置
    RequiredAcks    int           `mapstructure:"requiredAcks"`    // 消息确认级别
    Compression     string        `mapstructure:"compression"`     // 压缩算法
    FlushFrequency  time.Duration `mapstructure:"flushFrequency"`  // 刷新频率
    FlushMessages   int           `mapstructure:"flushMessages"`   // 批量消息数
    Timeout         time.Duration `mapstructure:"timeout"`         // 发送超时
}

type ConsumerConfig struct {
    // 基础配置 - 用户可配置
    GroupID            string        `mapstructure:"groupId"`            // 消费者组ID
    AutoOffsetReset    string        `mapstructure:"autoOffsetReset"`    // 偏移量重置策略
    SessionTimeout     time.Duration `mapstructure:"sessionTimeout"`     // 会话超时
    HeartbeatInterval  time.Duration `mapstructure:"heartbeatInterval"`  // 心跳间隔
}

type KafkaConfig struct {
    Brokers  []string       `mapstructure:"brokers"`  // Kafka集群地址
    Producer ProducerConfig `mapstructure:"producer"` // 生产者配置
    Consumer ConsumerConfig `mapstructure:"consumer"` // 消费者配置
    // 移除复杂的Net配置，由程序员在内部处理
}
```

### **程序员配置层** (sdk/pkg/eventbus/type.go)
```go
// ✅ 完整的程序员配置 - 包含所有技术细节
type ProducerConfig struct {
    // 用户配置字段
    RequiredAcks    int           `mapstructure:"requiredAcks"`
    Compression     string        `mapstructure:"compression"`
    FlushFrequency  time.Duration `mapstructure:"flushFrequency"`
    FlushMessages   int           `mapstructure:"flushMessages"`
    Timeout         time.Duration `mapstructure:"timeout"`
    
    // 程序员控制字段 - 有合理默认值
    FlushBytes      int           `mapstructure:"flushBytes"`      // 默认: 1MB
    RetryMax        int           `mapstructure:"retryMax"`        // 默认: 3
    BatchSize       int           `mapstructure:"batchSize"`       // 默认: 16KB
    BufferSize      int           `mapstructure:"bufferSize"`      // 默认: 32MB
    Idempotent      bool          `mapstructure:"idempotent"`      // 默认: true
    MaxMessageBytes int           `mapstructure:"maxMessageBytes"` // 默认: 1MB
    PartitionerType string        `mapstructure:"partitionerType"` // 默认: "hash"
    
    // 高级技术字段 - 程序员专用
    LingerMs        time.Duration `mapstructure:"lingerMs"`        // 默认: 5ms
    CompressionLevel int          `mapstructure:"compressionLevel"` // 默认: 6
    MaxInFlight     int           `mapstructure:"maxInFlight"`     // 默认: 5
}

type ConsumerConfig struct {
    // 用户配置字段
    GroupID            string        `mapstructure:"groupId"`
    AutoOffsetReset    string        `mapstructure:"autoOffsetReset"`
    SessionTimeout     time.Duration `mapstructure:"sessionTimeout"`
    HeartbeatInterval  time.Duration `mapstructure:"heartbeatInterval"`
    
    // 程序员控制字段 - 有合理默认值
    MaxProcessingTime  time.Duration `mapstructure:"maxProcessingTime"`  // 默认: 30s
    FetchMinBytes      int           `mapstructure:"fetchMinBytes"`      // 默认: 1KB
    FetchMaxBytes      int           `mapstructure:"fetchMaxBytes"`      // 默认: 50MB
    FetchMaxWait       time.Duration `mapstructure:"fetchMaxWait"`       // 默认: 500ms
    
    // 高级技术字段 - 程序员专用
    MaxPollRecords     int           `mapstructure:"maxPollRecords"`     // 默认: 500
    EnableAutoCommit   bool          `mapstructure:"enableAutoCommit"`   // 默认: false
    AutoCommitInterval time.Duration `mapstructure:"autoCommitInterval"` // 默认: 5s
    IsolationLevel     string        `mapstructure:"isolationLevel"`     // 默认: "read_committed"
}

type NetConfig struct {
    // 网络配置 - 程序员专用，用户不需要关心
    DialTimeout    time.Duration `mapstructure:"dialTimeout"`    // 默认: 30s
    ReadTimeout    time.Duration `mapstructure:"readTimeout"`    // 默认: 30s
    WriteTimeout   time.Duration `mapstructure:"writeTimeout"`   // 默认: 30s
    KeepAlive      time.Duration `mapstructure:"keepAlive"`      // 默认: 30s
    MaxIdleConns   int           `mapstructure:"maxIdleConns"`   // 默认: 10
    MaxOpenConns   int           `mapstructure:"maxOpenConns"`   // 默认: 100
}

type KafkaConfig struct {
    Brokers             []string       `mapstructure:"brokers"`
    HealthCheckInterval time.Duration  `mapstructure:"healthCheckInterval"` // 默认: 30s
    Producer            ProducerConfig `mapstructure:"producer"`
    Consumer            ConsumerConfig `mapstructure:"consumer"`
    Security            SecurityConfig `mapstructure:"security"`
    Net                 NetConfig      `mapstructure:"net"`
    
    // 高级功能配置 - 程序员专用
    ClientID            string        `mapstructure:"clientId"`            // 默认: "jxt-eventbus"
    MetadataRefreshFreq time.Duration `mapstructure:"metadataRefreshFreq"` // 默认: 10m
    MetadataRetryMax    int           `mapstructure:"metadataRetryMax"`    // 默认: 3
    MetadataRetryBackoff time.Duration `mapstructure:"metadataRetryBackoff"` // 默认: 250ms
}
```

## 🔄 **配置转换机制**

### 转换函数设计
```go
// 将用户配置转换为程序员配置
func convertUserConfigToInternalConfig(userConfig *config.KafkaConfig) *KafkaConfig {
    internalConfig := &KafkaConfig{
        Brokers: userConfig.Brokers,
        
        // 用户配置直接映射
        Producer: ProducerConfig{
            RequiredAcks:   userConfig.Producer.RequiredAcks,
            Compression:    userConfig.Producer.Compression,
            FlushFrequency: userConfig.Producer.FlushFrequency,
            FlushMessages:  userConfig.Producer.FlushMessages,
            Timeout:        userConfig.Producer.Timeout,
            
            // 程序员设定的默认值
            FlushBytes:      1024 * 1024,     // 1MB
            RetryMax:        3,               // 3次重试
            BatchSize:       16 * 1024,       // 16KB
            BufferSize:      32 * 1024 * 1024, // 32MB
            Idempotent:      true,            // 启用幂等性
            MaxMessageBytes: 1024 * 1024,     // 1MB
            PartitionerType: "hash",          // 哈希分区
            LingerMs:        5 * time.Millisecond,
            CompressionLevel: 6,
            MaxInFlight:     5,
        },
        
        Consumer: ConsumerConfig{
            GroupID:           userConfig.Consumer.GroupID,
            AutoOffsetReset:   userConfig.Consumer.AutoOffsetReset,
            SessionTimeout:    userConfig.Consumer.SessionTimeout,
            HeartbeatInterval: userConfig.Consumer.HeartbeatInterval,
            
            // 程序员设定的默认值
            MaxProcessingTime:  30 * time.Second,
            FetchMinBytes:      1024,         // 1KB
            FetchMaxBytes:      50 * 1024 * 1024, // 50MB
            FetchMaxWait:       500 * time.Millisecond,
            MaxPollRecords:     500,
            EnableAutoCommit:   false,        // 手动提交
            AutoCommitInterval: 5 * time.Second,
            IsolationLevel:     "read_committed",
        },
        
        // 程序员专用配置
        HealthCheckInterval: 30 * time.Second,
        ClientID:           "jxt-eventbus",
        MetadataRefreshFreq: 10 * time.Minute,
        MetadataRetryMax:   3,
        MetadataRetryBackoff: 250 * time.Millisecond,
        
        Net: NetConfig{
            DialTimeout:   30 * time.Second,
            ReadTimeout:   30 * time.Second,
            WriteTimeout:  30 * time.Second,
            KeepAlive:     30 * time.Second,
            MaxIdleConns:  10,
            MaxOpenConns:  100,
        },
        
        Security: SecurityConfig{
            Enabled: false, // 默认不启用安全认证
        },
    }
    
    return internalConfig
}
```

## 📊 **配置字段分类**

### 用户配置字段 (暴露给外部)
| 字段 | 类型 | 说明 | 用户关心度 |
|------|------|------|-----------|
| Brokers | []string | Kafka集群地址 | 🔴 必须 |
| RequiredAcks | int | 消息确认级别 | 🟡 重要 |
| Compression | string | 压缩算法 | 🟡 重要 |
| GroupID | string | 消费者组ID | 🔴 必须 |
| SessionTimeout | duration | 会话超时 | 🟡 重要 |

### 程序员配置字段 (内部控制)
| 字段 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| FlushBytes | int | 批量字节数 | 1MB |
| Idempotent | bool | 幂等性 | true |
| MaxInFlight | int | 最大飞行请求 | 5 |
| IsolationLevel | string | 隔离级别 | read_committed |
| NetConfig | struct | 网络配置 | 优化值 |

## ✅ **实施建议**

### 阶段1: 重构用户配置 (简化)
1. 移除用户不需要关心的技术字段
2. 保留核心业务配置字段
3. 添加清晰的配置文档

### 阶段2: 增强程序员配置 (完善)
1. 添加所有缺失的技术字段
2. 设定合理的默认值
3. 实现完整的转换函数

### 阶段3: 验证和优化
1. 测试配置转换的正确性
2. 验证默认值的合理性
3. 优化性能参数

这样的设计既保持了用户配置的简洁性，又给了程序员完全的技术控制能力！
