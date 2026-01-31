# Tenant Management Module

[![Go Report Card](https://goreportcard.com/badge/github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant)]
[![Coverage](https://codecov.io/gh/ChenBigdata421/jxt-core/pkg/tenant/branch/main/graph/badge.svg)]

> Shared tenant configuration management for all JXT microservices with ETCD integration, automatic reconnection, and persistent cache fallback.

## Table of Contents

- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
- [Architecture Design](#architecture-design)
- [API Reference](#api-reference)
- [Usage Examples](#usage-examples)
- [Configuration](#configuration)
- [Reliability Features](#reliability-features)
- [Troubleshooting](#troubleshooting)
- [Extension Guide](#extension-guide)

---

## Quick Start

### Installation

```bash
go get github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant
```

### Basic Usage

```go
package main

import (
    "context"
    "log"

    "github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/database"
    clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
    // 1. Create ETCD client
    etcdClient, err := clientv3.New(clientv3.Config{
        Endpoints:   []string{"localhost:2379"},
        DialTimeout: 5 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer etcdClient.Close()

    // 2. Create provider with retry and cache
    prov, err := provider.NewProviderWithRetry(etcdClient,
        provider.WithNamespace("jxt/"),
        provider.WithConfigTypes(
            provider.ConfigTypeDatabase,
            provider.ConfigTypeFtp,
            provider.ConfigTypeStorage,
        ),
        provider.WithCache(provider.NewFileCache()),
    )
    if err != nil {
        log.Fatal(err)
    }

    // 3. Start watch with retry
    ctx := context.Background()
    if err := prov.StartWatchWithRetry(ctx); err != nil {
        log.Fatal(err)
    }
    defer prov.StopWatch()

    // 4. Use cache
    dbCache := database.NewCache(prov)
    cfg, err := dbCache.GetByID(ctx, 1)
    if err != nil {
        log.Fatal(err)
    }

    // Use configuration...
    _ = cfg
}
```

---

## Core Concepts

### Configuration vs State Separation

The tenant module follows CQRS principles with clear separation:

```
┌─────────────────────────────────────────────────┐
│  ETCD (tenant-service manages)                   │
│  - database configuration                         │
│  - ftp configuration                             │
│  - storage configuration                          │
│  - meta: code, name, status                       │
└─────────────────────────────────────────────────┘
                    ↓ Watch
┌─────────────────────────────────────────────────┐
│  jxt-core Provider (in-memory cache)             │
│  - Copy-on-Write lock-free reads                 │
│  - Automatic reconnection with exponential backoff│
│  - Persistent file cache fallback                 │
└─────────────────────────────────────────────────┘
                    ↓ Cache API
┌─────────────────────────────────────────────────┐
│  Microservices (file-storage, evidence, etc.)    │
│  - database.Cache → DatabaseConfig               │
│  - ftp.Cache → FtpConfig                          │
│  - storage.Cache → StorageConfig                 │
└─────────────────────────────────────────────────┘
```

### Tenant Data Flow

1. **Configuration Write**: tenant-service → ETCD
2. **Configuration Distribution**: ETCD Watch → Provider → Microservices
3. **Local Fallback**: ETCD unavailable → Local cache file → Memory

---

## Architecture Design

### Layered Architecture

```
┌─────────────────────────────────────────────────────┐
│  Middleware Layer                                     │
│  - ExtractTenantID (HTTP header parsing)              │
└─────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────┐
│  Cache Layer (On-Demand Loading)                      │
│  - database.Cache  → DatabaseConfig                  │
│  - ftp.Cache       → FtpConfig                       │
│  - storage.Cache   → StorageConfig                   │
└─────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────┐
│  Provider Layer (Core)                               │
│  - ETCD Watch with exponential backoff reconnection   │
│  - Copy-on-Write lock-free reads                      │
│  - Persistent file cache fallback                     │
│  - Retry mechanism for initialization                 │
└─────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────┐
│  Persistence Layer                                   │
│  - ETCD (configuration source)                        │
│  - FileCache (local fallback)                         │
└─────────────────────────────────────────────────────┘
```

### Concurrency Safety

| Operation | Mechanism | Latency |
|-----------|-----------|---------|
| Read Config | `atomic.Value` | ~25ns |
| Update Config | Copy-on-Write | ~1μs |
| Watch Reconnect | Exponential backoff 1s→30s | - |

---

## API Reference

### Provider Options

```go
// Namespace configuration
WithNamespace(ns string) Option

// Configuration type filtering (on-demand loading)
WithConfigTypes(types ...ConfigType) Option

// Local cache fallback
WithCache(cache FileCache) Option
```

### Core Methods

| Method | Description | Returns |
|--------|-------------|---------|
| `NewProvider(client, opts...)` | Create basic provider | `*Provider` |
| `NewProviderWithRetry(client, opts...)` | Create and initialize with retry | `*Provider, error` |
| `LoadAll(ctx)` | Load all from ETCD | `error` |
| `StartWatch(ctx)` | Start watching ETCD | `error` |
| `StartWatchWithRetry(ctx)` | Start watch with retry | `error` |
| `StopWatch()` | Stop watching | - |
| `GetDatabaseConfig(id)` | Get database config | `*DatabaseConfig, bool` |
| `GetFtpConfig(id)` | Get FTP config | `*FtpConfig, bool` |
| `GetStorageConfig(id)` | Get storage config | `*StorageConfig, bool` |
| `GetTenantMeta(id)` | Get tenant metadata | `*TenantMeta, bool` |
| `IsTenantEnabled(id)` | Check if tenant is active | `bool` |
| `NewFileCache()` | Create default file cache | `FileCache` |
| `NewFileCacheWithPath(path)` | Create file cache with path | `FileCache` |

### Cache API

#### database.Cache

```go
func NewCache(prov *provider.Provider) *Cache
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantDatabaseConfig, error)
```

#### ftp.Cache

```go
func NewCache(prov *provider.Provider) *Cache
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantFtpConfig, error)
```

#### storage.Cache

```go
func NewCache(prov *provider.Provider) *Cache
func (c *Cache) GetByID(ctx context.Context, tenantID int) (*TenantStorageConfig, error)
```

---

## Usage Examples

### Scenario 1: Database Connection

```go
dbCache := database.NewCache(prov)

cfg, err := dbCache.GetByID(ctx, tenantID)
if err != nil {
    return err
}

// Create GORM DB connection
gormDB, err := gorm.Open(mysql.Open(fmt.Sprintf(
    "%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True",
    cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.DbName,
)), &gorm.Config{
    MaxIdleConns: cfg.MaxIdleConns,
    MaxOpenConns: cfg.MaxOpenConns,
})
```

### Scenario 2: Quota Check

```go
storageCache := storage.NewCache(prov)

cfg, _ := storageCache.GetByID(ctx, tenantID)
if file.Size > cfg.MaxFileSizeBytes {
    return errors.New("file exceeds size limit")
}

// Check concurrent uploads
if activeUploads >= cfg.MaxConcurrentUploads {
    return errors.New("max concurrent uploads reached")
}
```

### Scenario 3: Tenant Status Validation

```go
if !prov.IsTenantEnabled(tenantID) {
    return errors.New("tenant is not active")
}
```

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TENANT_CACHE_PATH` | `./cache/` | Local cache file directory |
| `ETCD_ENDPOINTS` | `localhost:2379` | ETCD service address |
| `TENANT_NAMESPACE` | `jxt/` | ETCD namespace prefix |

### ETCD Data Structure

```
jxt/tenants/{id}/meta        → {"id":1,"code":"tenant1","name":"租户1","status":"active"}
jxt/tenants/{id}/database    → {"host":"localhost","port":3306,...}
jxt/tenants/{id}/ftp         → {"username":"ftp1","passwordHash":"...",...}
jxt/tenants/{id}/storage     → {"uploadQuotaGb":100,"maxFileSizeMb":500,...}
```

---

## Reliability Features

### 1. Automatic Reconnection

- **Exponential backoff**: 1s → 30s (max)
- **Reset condition**: Backoff resets to 1s after successful event processing
- **All reconnection events**: Logged for monitoring

### 2. Initialization with Retry

| Operation | Retry Attempts | Interval | Total Time |
|-----------|----------------|---------|------------|
| LoadAll | 5 | 1s→2s→4s→8s→16s | ~31s |
| StartWatch | 3 | 1s→2s→5s | ~8s |

### 3. Local Cache Fallback

```
ETCD Available:  ETCD Watch → Memory → Local File (sync)
ETCD Unavailable: Local File → Memory (read-only mode)
```

### 4. Graceful Shutdown

```go
defer prov.StopWatch()  // Stop watch, flush cache
```

---

## Troubleshooting

### Problem 1: ETCD Connection Failed

**Symptom**: `failed to load tenant data after retries`

**Investigation**:
```bash
# Check ETCD service
curl http://localhost:2379/health

# Check tenant data
etcdctl get "" --prefix --keys-only
```

**Solution**:
- Enable local cache fallback (`WithCache()`)
- Check network connectivity
- Review reconnection logs

### Problem 2: Configuration Not Updated

**Symptom**: tenant-service updated but microservice not aware

**Investigation**:
```go
// Check if watch is running
if !prov.running.Load() {
    log.Error("Provider not running")
}
```

**Solution**:
- Confirm `StartWatch()` succeeded
- Check ETCD event delivery
- Review reconnection logs

### Problem 3: Local Cache File Corrupted

**Symptom**: `invalid cache file format`

**Solution**:
```go
// Delete cache file, will regenerate on next start
os.Remove("./cache/tenant_metadata.json")
```

---

## Extension Guide

### Adding New Configuration Type

1. Define ConfigType:
```go
const ConfigTypeCustom ConfigType = "custom"
```

2. Define Config Model:
```go
type CustomConfig struct {
    TenantID int    `json:"tenantId"`
    // ...
}
```

3. Extend processKey/handleDeleteKey:
```go
case "custom":
    p.processCustomKey(tenantID, parts, value, data)
```

4. Create Cache:
```go
// cache/custom/cache.go
type Cache struct { provider *provider.Provider }

func (c *Cache) GetByID(ctx, tenantID int) (*CustomConfig, error) {
    // ...
}
```

---

## Performance Metrics

| Metric | Value | Description |
|--------|-------|-------------|
| Read Latency | ~25ns | atomic.Value lock-free read |
| Update Latency | ~1μs | Copy-on-Write |
| Memory Usage | ~1KB/tenant | Includes all config types |
| Watch Sync | <1s | ETCD push latency |

---

## Related Documentation

- [ETCD Client v3 Documentation](https://etcd.io/docs/v3.5/learning/api/)
- [avast/retry-go Documentation](https://github.com/avast/retry-go)
- [jxt-core EventBus](../../jxt-core/README_EventBus.md)
- [Implementation Plan](../../../../../docs/plans/2026-01-31-tenant-reliability-enhancement.md)

---

## License

Copyright © 2026 JXT-Evidence-System
