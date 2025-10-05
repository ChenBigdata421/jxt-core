# EventBus 配置迁移指南

## 📋 **迁移概述**

jxt-core EventBus配置结构已经重构，提供了更清晰、更易用的配置方式。本指南将帮助您从旧配置迁移到新的`UnifiedEventBusConfig`。

## 🔄 **配置对比**

### **旧配置方式**

```go
// 方式1：基础配置
cfg := &config.EventBus{
    Type: "kafka",
    Kafka: config.KafkaConfig{
        Brokers: []string{"localhost:9092"},
        HealthCheckInterval: 2 * time.Minute,
        Producer: config.ProducerConfig{
            RequiredAcks: 1,
            Timeout:      10 * time.Second,
        },
        Consumer: config.ConsumerConfig{
            GroupID: "my-group",
        },
    },
}

// 方式2：高级配置
advCfg := &config.AdvancedEventBusConfig{
    EventBus: *cfg,
    ServiceName: "my-service",
    HealthCheck: config.HealthCheckConfig{
        Enabled:          true,
        Interval:         2 * time.Minute,
        Timeout:          10 * time.Second,
        FailureThreshold: 3,
    },
}
```

### **新配置方式（推荐）**

```go
// 统一配置方式
cfg := &config.UnifiedEventBusConfig{
    // 基础配置
    Type:        "kafka",
    ServiceName: "my-service",
    
    // 统一健康检查配置
    HealthCheck: config.HealthCheckConfig{
        Enabled: true,
        Sender: config.HealthCheckSenderConfig{
            Interval:         2 * time.Minute,
            Timeout:          10 * time.Second,
            FailureThreshold: 3,
        },
        Subscriber: config.HealthCheckSubscriberConfig{
            Enabled:         true,
            MonitorInterval: 30 * time.Second,
        },
    },
    
    // 发布端配置
    Publisher: config.PublisherConfig{
        PublishTimeout: 10 * time.Second,
        MaxReconnectAttempts: 5,
    },
    
    // 订阅端配置
    Subscriber: config.SubscriberConfig{
        MaxConcurrency: 10,
        ProcessTimeout: 30 * time.Second,
    },
    
    // Kafka特定配置
    Kafka: config.KafkaSpecificConfig{
        Brokers: []string{"localhost:9092"},
        Producer: config.ProducerConfig{
            RequiredAcks: 1,
            Timeout:      10 * time.Second,
        },
        Consumer: config.ConsumerConfig{
            GroupID: "my-group",
        },
    },
}

// 设置默认值
cfg.SetDefaults()

// 验证配置
if err := cfg.Validate(); err != nil {
    log.Fatal("Invalid config:", err)
}
```

## 🔧 **迁移步骤**

### **步骤1：更新配置结构**

```go
// 旧方式
type MyServiceConfig struct {
    EventBus *config.AdvancedEventBusConfig `mapstructure:"eventbus"`
}

// 新方式
type MyServiceConfig struct {
    EventBus *config.UnifiedEventBusConfig `mapstructure:"eventbus"`
}
```

### **步骤2：更新健康检查配置**

```go
// 旧方式
HealthCheck: config.HealthCheckConfig{
    Enabled:          true,
    Interval:         2 * time.Minute,
    Timeout:          10 * time.Second,
    FailureThreshold: 3,
    MessageTTL:       5 * time.Minute,
}

// 新方式
HealthCheck: config.HealthCheckConfig{
    Enabled: true,
    Sender: config.HealthCheckSenderConfig{
        Interval:         2 * time.Minute,
        Timeout:          10 * time.Second,
        FailureThreshold: 3,
        MessageTTL:       5 * time.Minute,
    },
    Subscriber: config.HealthCheckSubscriberConfig{
        Enabled:         true,
        MonitorInterval: 30 * time.Second,
    },
}
```

### **步骤3：分离特定实现配置**

```go
// 旧方式
Kafka: config.KafkaConfig{
    Brokers: []string{"localhost:9092"},
    HealthCheckInterval: 2 * time.Minute, // 重复配置
    Producer: config.ProducerConfig{...},
    Consumer: config.ConsumerConfig{...},
    BacklogDetection: config.BacklogDetectionConfig{...}, // 混合配置
}

// 新方式
HealthCheck: config.HealthCheckConfig{...}, // 统一健康检查
Publisher: config.PublisherConfig{
    BacklogDetection: config.BacklogDetectionConfig{...}, // 发布端特性
},
Kafka: config.KafkaSpecificConfig{
    Brokers: []string{"localhost:9092"}, // 只包含Kafka特定配置
    Producer: config.ProducerConfig{...},
    Consumer: config.ConsumerConfig{...},
}
```

## 🛠️ **自动迁移工具**

我们提供了配置转换函数来帮助迁移：

```go
// ConvertToUnified 将旧配置转换为新配置
func ConvertToUnified(old *config.AdvancedEventBusConfig) *config.UnifiedEventBusConfig {
    unified := &config.UnifiedEventBusConfig{
        Type:        old.Type,
        ServiceName: old.ServiceName,
    }
    
    // 转换健康检查配置
    if old.HealthCheck.Enabled {
        unified.HealthCheck = config.HealthCheckConfig{
            Enabled: true,
            Topic:   old.HealthCheck.Topic,
            Sender: config.HealthCheckSenderConfig{
                Interval:         old.HealthCheck.Interval,
                Timeout:          old.HealthCheck.Timeout,
                FailureThreshold: old.HealthCheck.FailureThreshold,
                MessageTTL:       old.HealthCheck.MessageTTL,
            },
            Subscriber: config.HealthCheckSubscriberConfig{
                Enabled: true,
            },
        }
    }
    
    // 转换Kafka配置
    if old.Type == "kafka" {
        unified.Kafka = config.KafkaSpecificConfig{
            Brokers:  old.Kafka.Brokers,
            Producer: old.Kafka.Producer,
            Consumer: old.Kafka.Consumer,
            Net:      old.Kafka.Net,
        }
        
        // 转换企业特性到发布端配置
        unified.Publisher = config.PublisherConfig{
            BacklogDetection: old.Kafka.BacklogDetection,
            RateLimit:        old.Kafka.RateLimit,
            ErrorHandling:    old.Kafka.ErrorHandling,
        }
    }
    
    // 转换NATS配置
    if old.Type == "nats" {
        unified.NATS = config.NATSSpecificConfig{
            URLs:              old.NATS.URLs,
            ClientID:          old.NATS.ClientID,
            MaxReconnects:     old.NATS.MaxReconnects,
            ReconnectWait:     old.NATS.ReconnectWait,
            ConnectionTimeout: old.NATS.ConnectionTimeout,
            JetStream:         old.NATS.JetStream,
            Security:          old.NATS.Security,
        }
    }
    
    // 设置默认值
    unified.SetDefaults()
    
    return unified
}
```

## 📝 **配置文件迁移**

### **YAML配置文件**

```yaml
# 旧配置文件
eventbus:
  type: kafka
  serviceName: my-service
  healthCheck:
    enabled: true
    interval: 2m
    timeout: 10s
    failureThreshold: 3
  kafka:
    brokers:
      - localhost:9092
    healthCheckInterval: 2m  # 重复配置
    producer:
      requiredAcks: 1
      timeout: 10s

# 新配置文件
eventbus:
  type: kafka
  serviceName: my-service
  healthCheck:
    enabled: true
    sender:
      interval: 2m
      timeout: 10s
      failureThreshold: 3
    subscriber:
      enabled: true
      monitorInterval: 30s
  publisher:
    publishTimeout: 10s
    maxReconnectAttempts: 5
  subscriber:
    maxConcurrency: 10
    processTimeout: 30s
  kafka:
    brokers:
      - localhost:9092
    producer:
      requiredAcks: 1
      timeout: 10s
```

## ⚠️ **注意事项**

### **1. 向后兼容性**
- 旧配置方式仍然有效，不会立即破坏现有代码
- 建议在新项目中使用新配置方式
- 现有项目可以逐步迁移

### **2. 配置优先级**
- 新配置中的统一特性配置优先级更高
- 如果同时配置了旧字段和新字段，新字段生效

### **3. 默认值变化**
- 新配置提供了更合理的默认值
- 迁移后请检查默认值是否符合预期

## 🎯 **最佳实践**

### **1. 渐进式迁移**
```go
// 第一步：保持现有配置，添加验证
oldCfg := &config.AdvancedEventBusConfig{...}
if err := validateOldConfig(oldCfg); err != nil {
    log.Fatal("Config validation failed:", err)
}

// 第二步：转换为新配置
newCfg := ConvertToUnified(oldCfg)

// 第三步：逐步替换配置使用
```

### **2. 配置测试**
```go
func TestConfigMigration(t *testing.T) {
    oldCfg := &config.AdvancedEventBusConfig{...}
    newCfg := ConvertToUnified(oldCfg)
    
    // 验证转换结果
    assert.Equal(t, oldCfg.Type, newCfg.Type)
    assert.Equal(t, oldCfg.ServiceName, newCfg.ServiceName)
    
    // 验证配置有效性
    assert.NoError(t, newCfg.Validate())
}
```

### **3. 监控迁移过程**
```go
// 记录配置迁移
logger.Info("Migrating EventBus config",
    "from", "AdvancedEventBusConfig",
    "to", "UnifiedEventBusConfig",
    "service", cfg.ServiceName)
```

## 🚀 **迁移完成后的优势**

1. **更清晰的配置结构**：统一特性与具体实现分离
2. **更好的类型安全**：编译时和运行时配置验证
3. **更强的扩展性**：易于添加新的EventBus类型
4. **更完善的默认值**：减少配置错误
5. **更好的文档支持**：清晰的配置说明和示例

## 📞 **获取帮助**

如果在迁移过程中遇到问题，请：

1. 查看配置验证错误信息
2. 参考本指南中的示例
3. 使用提供的转换函数
4. 联系开发团队获取支持

**祝您迁移顺利！** 🎉
