# 服务级数据库配置指南

## 概述

jxt-core v2.0 开始支持服务级数据库配置，允许为每个租户的每个微服务配置独立的数据库连接。

## 配置结构

### 传统配置方式（向后兼容）

```yaml
tenants:
  default:
    database:
      driver: postgres
      host: postgres-security
      port: 5432
      database: securitydb
```

### 新配置方式（推荐）

```yaml
tenants:
  default:
    service_databases:
      evidence-command:
        driver: mysql
        host: mysql-command
        port: 3306
        database: tenant_command

      evidence-query:
        driver: postgres
        host: postgres-query
        port: 5432
        database: tenant_query
```

## 服务代码

系统支持以下服务代码：

- `evidence-command` - 证据管理写服务
- `evidence-query` - 证据管理读服务
- `file-storage` - 文件存储服务
- `security-management` - 安全管理服务

## 完整配置示例

```yaml
tenants:
  default:
    # 传统配置（向后兼容）
    database:
      driver: postgres
      host: postgres-security
      port: 5432
      database: securitydb

    # 服务级数据库配置（推荐）
    service_databases:
      evidence-command:
        driver: mysql
        host: mysql-command
        port: 3306
        database: tenant_command
        username: tenant_cmd
        password: password123
        sslmode: disable
        max_open_conns: 100
        max_idle_conns: 20
        conn_max_idle_time: 300
        conn_max_life_time: 3600
        connect_timeout: 10
        read_timeout: 30
        write_timeout: 30

      evidence-query:
        driver: postgres
        host: postgres-query
        port: 5432
        database: tenant_query
        username: tenant_query
        password: password123
        sslmode: disable
        max_open_conns: 50
        max_idle_conns: 10

      file-storage:
        driver: postgres
        host: postgres-storage
        port: 5432
        database: tenant_storage
        username: tenant_storage
        password: password123

      security-management:
        driver: postgres
        host: postgres-security
        port: 5432
        database: securitydb
        username: tenant
        password: password123
```

## Runtime API

### 设置服务数据库连接

```go
import (
    "github.com/ChenBigdata421/jxt-core/sdk"
    "gorm.io/gorm"
)

// 设置租户 1001 的 evidence-command 服务数据库
app.SetTenantServiceDB(1001, "evidence-command", db)
```

### 获取服务数据库连接

```go
// 获取租户 1001 的 evidence-command 服务数据库
db := app.GetTenantServiceDB(1001, "evidence-command")
if db != nil {
    // 使用数据库连接
    db.Where("status = ?", "active").Find(&evidence)
}
```

### 遍历所有服务数据库连接

```go
// 遍历所有租户的所有服务数据库连接
count := 0
app.GetTenantServiceDBs(func(tenantID int, serviceCode string, db *gorm.DB) bool {
    count++
    fmt.Printf("租户 %d 的 %s 服务数据库已连接\n", tenantID, serviceCode)
    return true // 返回 false 可停止遍历
})
fmt.Printf("总连接数: %d\n", count)
```

## 向后兼容

为了保持向后兼容性，旧 API 仍然可用，并自动映射到服务级配置：

- `GetTenantDB()` - 优先返回 `security-management` 服务数据库，否则返回默认 `tenantDBs`
- `GetTenantCommandDB()` - 返回 `evidence-command` 服务数据库
- `GetTenantQueryDB()` - 返回 `evidence-query` 服务数据库

### 兼容性示例

```go
// 旧代码仍然可用
db := app.GetTenantDB(tenantID)          // 返回 security-management 服务数据库
commandDB := app.GetTenantCommandDB(tenantID)  // 返回 evidence-command 服务数据库
queryDB := app.GetTenantQueryDB(tenantID)      // 返回 evidence-query 服务数据库
```

## 配置优先级

当同时配置了传统配置和服务级配置时：

1. **服务级配置优先**：`service_databases` 中的配置优先
2. **传统配置作为回退**：当服务级配置不存在时，使用 `database` 配置
3. **代码设置最高**：通过 `SetTenantServiceDB()` 设置的连接优先级最高

## 数据库连接字符串

框架会根据配置自动生成连接字符串：

### PostgreSQL

```
postgres://username:password@host:port/database?sslmode=disable
```

### MySQL

```
username:password@tcp(host:port)/database
```

## 迁移指南

### 从传统配置迁移到服务级配置

#### 步骤 1：添加服务级配置

在配置文件中为每个服务添加独立的数据库配置：

```yaml
tenants:
  default:
    # 保留旧配置（可选）
    database:
      driver: postgres
      host: postgres-security
      port: 5432
      database: securitydb

    # 添加服务级配置
    service_databases:
      evidence-command:
        driver: mysql
        host: mysql-command
        port: 3306
        database: tenant_command
```

#### 步骤 2：更新代码（可选）

如果需要更细粒度的控制，可以更新代码使用新的 API：

```go
// 旧代码
app.SetTenantDB(tenantID, db)
db := app.GetTenantDB(tenantID)

// 新代码（推荐）
app.SetTenantServiceDB(tenantID, "security-management", db)
db := app.GetTenantServiceDB(tenantID, "security-management")
```

#### 步骤 3：验证迁移

```go
// 验证配置是否正确加载
defaultConfig := config.TenantsConfig.GetDefault()
if defaultConfig.HasServiceDatabases() {
    fmt.Println("服务级数据库配置已启用")

    // 获取特定服务配置
    cmdConfig := defaultConfig.GetServiceDatabase("evidence-command")
    if cmdConfig != nil {
        fmt.Printf("Command DB: %s:%d/%s\n",
            cmdConfig.GetDatabaseHost(),
            cmdConfig.GetDatabasePort(),
            cmdConfig.GetDatabaseName())
    }
}
```

## 最佳实践

### 1. 服务隔离

为不同微服务使用独立的数据库，实现服务级别的数据隔离：

```yaml
service_databases:
  evidence-command:
    database: tenant_command_db
  evidence-query:
    database: tenant_query_db
  file-storage:
    database: tenant_storage_db
```

### 2. 连接池优化

根据服务负载调整连接池参数：

```yaml
evidence-command:
  max_open_conns: 100   # 写服务可能需要更多连接
  max_idle_conns: 20

evidence-query:
  max_open_conns: 50    # 读服务可以少一些
  max_idle_conns: 10
```

### 3. 超时配置

根据服务特性设置合理的超时：

```yaml
evidence-command:
  connect_timeout: 10
  write_timeout: 30

evidence-query:
  connect_timeout: 5
  read_timeout: 30
```

## 故障排查

### 问题：服务数据库连接失败

**症状**：`GetTenantServiceDB()` 返回 `nil`

**解决方案**：

1. 检查配置文件中是否配置了 `service_databases`
2. 验证数据库主机和端口是否正确
3. 检查数据库用户权限

```go
db := app.GetTenantServiceDB(tenantID, "evidence-command")
if db == nil {
    log.Error("数据库连接未配置或连接失败")
    // 使用回退逻辑
    db = app.GetTenantDB(tenantID)
}
```

### 问题：配置未生效

**症状**：配置了 `service_databases` 但未使用

**解决方案**：

1. 确保配置文件格式正确（YAML 缩进）
2. 检查服务代码是否正确（区分大小写）
3. 验证配置是否被正确加载

```go
// 调试：打印所有配置
defaultConfig := config.TenantsConfig.GetDefault()
for serviceCode, dbConfig := range defaultConfig.GetServiceDatabases() {
    fmt.Printf("服务: %s, 数据库: %s\n",
        serviceCode,
        dbConfig.GetDatabaseName())
}
```

## 相关文档

- [多租户配置](./tenant.go) - 配置结构定义
- [应用运行时 API](../runtime/application.go) - Runtime API 文档
- [配置迁移指南](./MIGRATION_GUIDE.md) - 完整迁移指南
