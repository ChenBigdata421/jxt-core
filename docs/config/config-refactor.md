# ETCD配置重构说明

## 重构背景

原先的etcd配置只在gRPC服务端配置中，但实际上：
- gRPC客户端也需要etcd进行服务发现
- 其他模块可能也需要etcd作为配置中心或服务发现
- etcd作为基础设施组件，应该作为公共配置

## 主要变化

### 1. 配置结构调整

**之前**：etcd配置嵌套在gRPC服务端配置中
```go
type GRPCServerConfig struct {
    // ...
    Etcd EtcdConfig `mapstructure:"etcd" json:"etcd"`
    // ...
}
```

**现在**：etcd配置提升为顶层公共配置
```go
type Config struct {
    // ...
    Etcd *EtcdConfig `mapstructure:"etcd" json:"etcd"`
    // ...
}
```

### 2. 新增功能

1. **增强的etcd配置**：
   - 支持用户名/密码认证
   - 支持TLS加密
   - 支持命名空间隔离
   - 支持连接和操作超时配置

2. **gRPC客户端服务发现**：
   - 新增`UseDiscovery`字段控制是否使用服务发现
   - 新增`ServiceKey`字段指定服务发现键名
   - 保留`Endpoints`字段用于直连模式

3. **统一的etcd客户端接口**：
   - 提供标准的etcd操作接口
   - 支持服务注册/发现
   - 支持配置监听

## 配置示例

```yaml
# ETCD配置 - 作为公共基础设施
etcd:
  enabled: true
  hosts:
    - "localhost:2379"
    - "localhost:2380"
  username: ""
  password: ""
  namespace: "jxt/"  # 命名空间前缀，用于隔离不同环境
  dialTimeout: 5     # 连接超时(秒)
  timeout: 3         # 操作超时(秒)
  tls:
    enabled: false

# gRPC配置
grpc:
  server:
    enabled: true
    listenOn: ":9000"
    key: "jxt.grpc.server"  # 服务注册键名
  
  client:
    enabled: true
    useDiscovery: true                    # 使用服务发现
    serviceKey: "jxt.grpc.server"         # 服务发现键名
    endpoints: []                         # 直连端点（不使用服务发现时）
```

## 使用方法

### 1. 获取etcd客户端

```go
import (
    "github.com/ChenBigdata421/jxt-core/sdk/config"
    "github.com/ChenBigdata421/jxt-core/tools/etcd"
)

// 从配置创建并设置全局etcd客户端
client, err := etcd.NewClientFromConfig(config.AppConfig.Etcd)
if err != nil {
    log.Fatalf("Failed to create etcd client: %v", err)
}
etcd.SetClient(client)

// 获取全局etcd客户端
client := etcd.GetClient()
if client != nil {
    // 使用etcd客户端
    err := client.Put(context.Background(), "key", "value")
}
```

### 2. 服务注册

```go
import "github.com/ChenBigdata421/jxt-core/tools/etcd"

// gRPC服务注册
client := etcd.GetClient()
err := client.Register(
    context.Background(),
    "jxt.grpc.server",
    `{"address":"localhost","port":9000}`,
    30, // TTL 30秒
)
```

### 3. 服务发现

```go
import "github.com/ChenBigdata421/jxt-core/tools/etcd"

// 发现服务端点
client := etcd.GetClient()
endpoints, err := client.Discover(context.Background(), "jxt.grpc.server")
for _, endpoint := range endpoints {
    fmt.Printf("Service: %s:%d\n", endpoint.Address, endpoint.Port)
}
```

## 架构说明

### 包结构

```
sdk/config/
├── etcd.go          # etcd配置定义
├── grpc.go          # gRPC配置（移除了etcd配置）
└── config.go        # 顶层配置结构

tools/etcd/
├── client.go        # etcd客户端接口和工具函数
└── etcdv3_client.go # etcd v3具体实现
```

### 职责分离

1. **config包**：
   - 定义配置结构
   - 提供配置验证和辅助方法
   - 保持轻量级，无重依赖

2. **tools/etcd包**：
   - 定义客户端接口
   - 提供具体实现
   - 处理业务逻辑
   - 管理全局客户端实例

### 优势

1. **清晰的职责分离**：配置定义与工具实现分离
2. **更好的测试性**：可以轻松模拟etcd客户端接口
3. **依赖管理**：工具包可以有重依赖，配置包保持轻量
4. **扩展性**：可以轻松添加不同的etcd客户端实现

## 迁移指南

### 1. 配置文件调整

将原来嵌套在gRPC配置中的etcd配置移动到顶层：

```yaml
# 旧配置
grpc:
  server:
    etcd:
      enabled: true
      hosts: ["localhost:2379"]

# 新配置
etcd:
  enabled: true
  hosts: ["localhost:2379"]
  namespace: "your-app/"

grpc:
  server:
    key: "your-app.grpc.server"
  client:
    useDiscovery: true
    serviceKey: "your-app.grpc.server"
```

### 2. 代码调整

如果有直接使用gRPC服务端etcd配置的代码，需要改为使用全局etcd配置：

```go
// 旧方式
etcdConfig := config.GrpcConfig.Server.Etcd

// 新方式
etcdConfig := config.AppConfig.Etcd
```

## 优势

1. **配置复用**：多个模块可以共享同一个etcd配置
2. **服务发现**：gRPC客户端支持基于etcd的服务发现
3. **命名空间隔离**：支持多环境部署
4. **更好的扩展性**：为未来添加更多基于etcd的功能提供基础
5. **统一管理**：etcd相关的所有配置和客户端统一管理 