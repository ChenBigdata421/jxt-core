# jxt-core - 企业级微服务基础框架

[![Go Version](https://img.shields.io/github/go-mod/go-version/ChenBigdata421/jxt-core)](https://golang.org/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

本仓库基于 [go-admin-core](https://github.com/ChenBigdata421/jxt-core) 修改，原项目遵循 Apache 2.0 协议

## 项目简介

jxt-core 是一个基于 Go 语言的企业级微服务基础框架，提供了构建现代化分布式应用所需的核心组件和工具。框架采用模块化设计，支持多种部署模式，适用于大型企业级应用开发。

## 核心特性

### 🚀 基础设施组件
- [x] **日志组件** - 基于 zap 的高性能结构化日志，支持多种输出格式和日志分级
- [x] **配置管理** - 基于 viper 的多格式配置支持（YAML、JSON、TOML等）
- [x] **缓存系统** - 支持 Memory、Redis 多种缓存后端
- [x] **消息队列** - 支持 Memory、Redis、NSQ 多种队列实现
- [x] **分布式锁** - 基于 Redis 的分布式锁实现
- [x] **Outbox 模式** - 保证业务操作与事件发布的原子性和最终一致性 ⭐ **新增**

### 🔧 服务治理
- [x] **服务发现** - 基于 ETCD 的服务注册与发现
- [x] **gRPC 支持** - 完整的 gRPC 服务端和客户端实现
- [x] **HTTP 服务** - 基于 Gin 的 RESTful API 支持
- [x] **负载均衡** - 支持多种负载均衡策略
- [x] **监控指标** - 集成 Prometheus 监控指标收集

### 🔐 安全认证
- [x] **JWT 认证** - 完整的 JWT token 生成和验证
- [x] **权限控制** - 基于 Casbin 的 RBAC 权限管理
- [x] **验证码** - 图形验证码生成和验证
- [x] **加密工具** - 常用的加密解密工具集

### 📡 通信协议
- [x] **WebSocket** - 实时双向通信支持
- [x] **gRPC** - 高性能 RPC 通信
- [x] **HTTP/HTTPS** - 标准 Web API 支持

### 💾 数据存储
- [x] **多数据库支持** - MySQL、PostgreSQL、SQLite、SQL Server
- [x] **ORM 集成** - 基于 GORM 的数据库操作
- [x] **读写分离** - 支持主从数据库配置
- [x] **事务管理** - 完整的数据库事务支持

### ⏰ 任务调度
- [x] **定时任务** - 基于 cron 的任务调度
- [x] **异步任务** - 基于队列的异步任务处理

### 🛠 开发工具
- [x] **代码生成** - 自动生成常用代码模板
- [x] **工具函数** - 丰富的工具函数库
- [x] **中间件** - 常用的 HTTP 中间件
- [x] **响应封装** - 统一的 API 响应格式

## 快速开始

### 环境要求

- Go 1.23+ 
- Redis 6.0+ (可选)
- MySQL 8.0+ / PostgreSQL 12+ (可选)
- ETCD 3.5+ (用于服务发现，可选)

### 安装

```bash
go get github.com/ChenBigdata421/jxt-core
```

### 基本使用

```go
package main

import (
    "github.com/ChenBigdata421/jxt-core/sdk"
    "github.com/ChenBigdata421/jxt-core/sdk/config"
)

func main() {
    // 初始化配置
    cfg := config.NewConfig()
    
    // 创建应用实例
    app := sdk.NewApplication(cfg)
    
    // 启动应用
    app.Run()
}
```

### 配置示例

```yaml
# 应用配置
application:
  name: "my-service"
  mode: "dev"
  version: "1.0.0"

# HTTP 服务配置
http:
  host: "0.0.0.0"
  port: 8080

# 数据库配置
database:
  masterDB:
    driver: "mysql"
    source: "user:pass@tcp(localhost:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local"

# 缓存配置
cache:
  redis:
    addr: "localhost:6379"
    password: ""
    db: 0

# 日志配置
logger:
  level: "info"
  format: "json"
```

## 项目结构

```
jxt-core/
├── sdk/                    # 核心 SDK
│   ├── config/            # 配置管理
│   │   └── config.go     # 配置管理逻辑
│   ├── pkg/               # 核心组件包
│   │   ├── logger/        # 日志组件
│   │   ├── jwtauth/       # JWT 认证
│   │   ├── casbin/        # 权限控制
│   │   ├── captcha/       # 验证码
│   │   ├── ws/            # WebSocket
│   │   └── cronjob/       # 定时任务
│   ├── middleware/        # 中间件
│   └── service/           # 服务层
├── storage/               # 存储层
│   ├── cache/             # 缓存实现
│   ├── queue/             # 队列实现
│   └── locker/            # 分布式锁
├── examples/              # 使用示例
├── tools/                 # 开发工具
└── docs/                  # 文档
```

## 文档

### 核心文档
- [文档中心](docs/README.md) - 完整的文档索引
- [EventBus 文档](sdk/pkg/eventbus/README.md) - 事件总线使用指南
- [Outbox 模式快速开始](docs/outbox-pattern-quick-start.md) ⭐ - 5 分钟快速上手
- [Outbox 模式完整设计](docs/outbox-pattern-design.md) - 完整的架构设计和使用指南

### 其他文档
- [快速开始指南](docs/quickstart.md)
- [配置文档](docs/config.md)
- [API 文档](docs/api.md)
- [部署指南](docs/deployment.md)

## 贡献

欢迎提交 Issue 和 Pull Request 来帮助改进项目。

## 许可证

本项目采用 [Apache 2.0](LICENSE) 许可证。

## 版本历史

- v1.0.0 - 初始版本，提供基础框架功能
