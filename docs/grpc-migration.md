# gRPC 配置优化指南

## 概述

本文档说明如何使用优化后的 gRPC 配置结构。新配置结构更加简洁实用，同时提供转换函数来生成 go-zero 需要的配置格式。

## 设计理念

### 1. 配置与实现分离

我们的设计理念是：**配置结构专注于业务需求，通过转换函数适配技术框架**

- ✅ **简洁实用** - 配置结构只包含必要的业务配置
- ✅ **灵活适配** - 通过转换函数支持不同的技术框架  
- ✅ **易于维护** - 配置结构独立于框架变化
- ✅ **向后兼容** - 保持现有配置的可用性

### 2. 主要优化

#### ETCD 配置独立化
- **原设计**: gRPC 配置中嵌套 ETCD 配置
- **新设计**: ETCD 作为全局基础设施配置
- **优势**: 避免重复配置，支持多服务共享

#### 配置结构简化
- **服务端配置**: 专注于 gRPC 服务本身的配置
- **客户端配置**: 支持多种连接模式（服务发现、直连、Target）
- **中间件配置**: 清晰的功能开关

### 3. 转换函数设计

提供转换函数将我们的配置转换为 go-zero 需要的格式：

```go
// 服务端配置转换
func (c *GRPCServerConfig) ToGoZeroRpcServerConf(etcdConfig *ETCDConfig) map[string]interface{}

// 客户端配置转换  
func (c *GRPCClientConfig) ToGoZeroRpcClientConf(etcdConfig *ETCDConfig) map[string]interface{}
```

**优势：**
- 🔄 **自动适配** - 自动处理配置格式转换
- 🏗️ **框架无关** - 配置结构不依赖特定框架
- 🔧 **灵活扩展** - 可以轻松支持其他 gRPC 框架

### 3. 中间件配置

#### 服务端中间件
```yaml
middlewares:
  trace: true      # 链路跟踪
  recover: true    # 异常恢复
  stat: true       # 统计
  prometheus: true # Prometheus监控
  breaker: true    # 熔断器
```

#### 客户端中间件
```yaml
middlewares:
  trace: true      # 链路跟踪
  duration: true   # 持续时间统计
  prometheus: true # Prometheus监控
  breaker: true    # 熔断器
```

## 迁移步骤

### 步骤 1: 更新配置文件

#### 旧配置格式：
```yaml
grpc:
  server:
    enabled: true
    listenOn: ":9000"
    timeout: 5000
    key: "service.key"
  client:
    enabled: true
    timeout: 5000
    keepAlive: 30
    useDiscovery: true
    serviceKey: "service.key"
```

#### 新配置格式：
```yaml
grpc:
  server:
    # 基础服务配置
    name: "my-service"
    mode: "dev"
    
    # gRPC 配置
    listenOn: ":9000"
    timeout: 2000
    cpuThreshold: 900
    
    # ETCD 配置
    etcd:
      hosts: ["localhost:2379"]
      key: "service.key"
    
    # 中间件配置
    middlewares:
      trace: true
      recover: true
      stat: true
      prometheus: true
      breaker: true
      
  client:
    timeout: 2000
    keepaliveTime: "20s"
    
    etcd:
      key: "service.key"
    
    middlewares:
      trace: true
      duration: true
      prometheus: true
      breaker: true
```

### 步骤 2: 更新代码引用

如果代码中直接引用了配置结构，需要更新：

```go
// 旧代码
type Config struct {
    GRPC struct {
        Server struct {
            Timeout int `mapstructure:"timeout"`
        } `mapstructure:"server"`
    } `mapstructure:"grpc"`
}

// 新代码
type Config struct {
    GRPC config.GRPCConfig `mapstructure:"grpc"`
}
```

### 步骤 3: 利用新功能

#### 启用认证
```yaml
grpc:
  server:
    auth: true
  client:
    app: "my-app"
    token: "secret-token"
```

#### 配置 Prometheus 监控
```yaml
grpc:
  server:
    prometheus:
      host: "0.0.0.0"
      port: 9101
      path: "/metrics"
```

#### 启用严格控制模式
```yaml
grpc:
  server:
    strictControl: true
```

## 向后兼容性

新配置保持了向后兼容性：

1. **保留所有旧字段** - 所有原有字段仍然可用
2. **默认值设置** - 新字段都有合理的默认值
3. **渐进式迁移** - 可以逐步迁移到新配置

## 最佳实践

### 1. 使用 go-zero 标准默认值
- `timeout: 2000` (毫秒)
- `keepaliveTime: "20s"`
- `cpuThreshold: 900`

### 2. 启用关键中间件
```yaml
middlewares:
  trace: true      # 链路跟踪，生产环境推荐
  recover: true    # 异常恢复，必须启用
  prometheus: true # 监控，生产环境必须
  breaker: true    # 熔断器，高并发场景推荐
```

### 3. 生产环境配置
```yaml
grpc:
  server:
    mode: "pro"
    auth: true
    strictControl: true
    middlewares:
      trace: true
      recover: true
      stat: true
      prometheus: true
      breaker: true
```

## 故障排除

### 常见问题

1. **配置解析失败**
   - 检查 YAML 语法
   - 确认字段名称正确
   - 验证数据类型匹配

2. **服务发现不工作**
   - 确认 ETCD 配置正确
   - 检查服务注册键名
   - 验证 ETCD 连接

3. **认证失败**
   - 确认 `auth: true` 已设置
   - 检查 `app` 和 `token` 配置
   - 验证服务端认证配置

### 调试建议

1. 启用调试日志：
```yaml
log:
  level: "debug"
  mode: "console"
```

2. 检查配置加载：
```go
fmt.Printf("Config: %+v\n", config)
```

3. 监控指标检查：
- 访问 `http://localhost:9101/metrics`
- 查看 gRPC 相关指标

## 总结

新的 gRPC 配置结构：
- ✅ 完全符合 go-zero 标准
- ✅ 保持向后兼容性
- ✅ 增强功能支持
- ✅ 更好的可维护性
- ✅ 标准化的监控和治理 