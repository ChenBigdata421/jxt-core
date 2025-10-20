# TopicBuilder 压缩配置指南

## 📦 概述

TopicBuilder 现在支持 Kafka 消息压缩配置，可以显著减少网络传输和磁盘存储开销。

---

## 🎯 支持的压缩算法

| 算法 | 速度 | 压缩率 | CPU开销 | 推荐场景 | Kafka版本要求 |
|------|------|--------|---------|----------|---------------|
| **none** | 最快 | 无 | 无 | 低延迟场景 | 所有版本 |
| **lz4** | 极快 | 低 (2-3x) | 极低 | 高吞吐量场景 | Kafka 0.8.2+ |
| **snappy** | 快 | 中 (2-4x) | 低 | **生产环境推荐** | Kafka 0.8.0+ |
| **gzip** | 中 | 高 (5-10x) | 高 | 存储优先场景 | 所有版本 |
| **zstd** | 快 | 高 (5-12x) | 中 | **最佳平衡** | Kafka 2.1.0+ |

---

## 🚀 快速开始

### 1. 使用预设配置（已包含 Snappy 压缩）

```go
// 高吞吐量预设（默认 snappy 压缩）
err := eventbus.NewTopicBuilder("orders").
    ForHighThroughput().
    Build(ctx, bus)
```

### 2. 使用快捷方法

```go
// Snappy 压缩（推荐）
eventbus.NewTopicBuilder("orders").
    WithPartitions(10).
    SnappyCompression().
    Build(ctx, bus)

// LZ4 压缩（最快）
eventbus.NewTopicBuilder("orders").
    WithPartitions(10).
    Lz4Compression().
    Build(ctx, bus)

// GZIP 压缩（高压缩率）
eventbus.NewTopicBuilder("orders").
    WithPartitions(10).
    GzipCompression(6). // 级别 1-9
    Build(ctx, bus)

// Zstd 压缩（最佳平衡，Kafka 2.1+）
eventbus.NewTopicBuilder("orders").
    WithPartitions(10).
    ZstdCompression(3). // 级别 1-22
    Build(ctx, bus)

// 禁用压缩
eventbus.NewTopicBuilder("orders").
    WithPartitions(10).
    NoCompression().
    Build(ctx, bus)
```

### 3. 使用通用方法

```go
eventbus.NewTopicBuilder("orders").
    WithPartitions(10).
    WithCompression("snappy").
    WithCompressionLevel(6).
    Build(ctx, bus)
```

---

## 📊 压缩算法详解

### 1. Snappy（推荐）

**特点：**
- ✅ 性能和压缩率的最佳平衡
- ✅ CPU 开销低
- ✅ 压缩/解压速度快
- ✅ 生产环境广泛使用

**使用场景：**
- 生产环境默认选择
- 平衡性能和存储成本
- 大多数业务场景

**示例：**
```go
eventbus.NewTopicBuilder("orders").
    SnappyCompression().
    Build(ctx, bus)
```

**压缩效果：**
- 文本/JSON：2-4倍压缩
- 二进制数据：1.5-2倍压缩

---

### 2. LZ4（最快）

**特点：**
- ✅ 最快的压缩/解压速度
- ✅ CPU 开销极低
- ⚠️ 压缩率较低

**使用场景：**
- 高吞吐量场景
- CPU 资源受限
- 延迟敏感应用

**示例：**
```go
eventbus.NewTopicBuilder("orders").
    Lz4Compression().
    Build(ctx, bus)
```

**压缩效果：**
- 文本/JSON：2-3倍压缩
- 二进制数据：1.3-1.8倍压缩

---

### 3. GZIP（高压缩率）

**特点：**
- ✅ 高压缩率
- ⚠️ CPU 开销大
- ⚠️ 压缩/解压速度慢

**使用场景：**
- 存储成本高
- 网络带宽受限
- 对延迟不敏感

**示例：**
```go
eventbus.NewTopicBuilder("orders").
    GzipCompression(6). // 级别 1-9
    Build(ctx, bus)
```

**压缩级别：**
- 1-3：快速，低压缩率
- 4-6：平衡（推荐）
- 7-9：慢速，高压缩率

**压缩效果：**
- 文本/JSON：5-10倍压缩
- 二进制数据：2-4倍压缩

---

### 4. Zstd（最佳平衡，Kafka 2.1+）

**特点：**
- ✅ 最佳的压缩率和性能平衡
- ✅ 可调节的压缩级别范围广
- ⚠️ 需要 Kafka 2.1.0+

**使用场景：**
- 新项目推荐
- 需要高压缩率但不牺牲太多性能
- Kafka 版本 >= 2.1

**示例：**
```go
eventbus.NewTopicBuilder("orders").
    ZstdCompression(3). // 级别 1-22
    Build(ctx, bus)
```

**压缩级别：**
- 1-3：快速，中等压缩率（推荐）
- 4-9：平衡
- 10-22：慢速，高压缩率

**压缩效果：**
- 文本/JSON：5-12倍压缩
- 二进制数据：2-5倍压缩

---

### 5. None（不压缩）

**特点：**
- ✅ 无 CPU 开销
- ✅ 最低延迟
- ⚠️ 网络和存储开销大

**使用场景：**
- 极低延迟要求
- 数据已压缩（如图片、视频）
- 网络和存储成本低

**示例：**
```go
eventbus.NewTopicBuilder("orders").
    NoCompression().
    Build(ctx, bus)
```

---

## 🎯 选择压缩算法的决策树

```
开始
  ↓
是否需要压缩？
  ├─ 否 → None
  └─ 是
      ↓
Kafka 版本 >= 2.1？
  ├─ 是 → Zstd (推荐)
  └─ 否
      ↓
主要关注点？
  ├─ 性能 → LZ4
  ├─ 平衡 → Snappy (推荐)
  └─ 存储 → GZIP
```

---

## 📈 性能对比

### 压缩速度对比（相对值）

```
LZ4:    ████████████████████ (最快)
Snappy: ████████████████ (快)
Zstd:   ████████████ (中等)
GZIP:   ████████ (慢)
```

### 压缩率对比（文本/JSON数据）

```
GZIP:   ██████████████████████ (5-10x)
Zstd:   ████████████████████ (5-12x)
Snappy: ██████████ (2-4x)
LZ4:    ████████ (2-3x)
None:   ██ (1x)
```

### CPU 开销对比

```
None:   ░ (无)
LZ4:    ██ (极低)
Snappy: ████ (低)
Zstd:   ████████ (中)
GZIP:   ████████████ (高)
```

---

## 💡 最佳实践

### 1. 生产环境推荐配置

```go
// 方案1：Snappy（稳定可靠）
eventbus.NewTopicBuilder("orders").
    ForHighThroughput().
    SnappyCompression(). // 已包含在预设中
    Build(ctx, bus)

// 方案2：Zstd（最佳平衡，Kafka 2.1+）
eventbus.NewTopicBuilder("orders").
    ForHighThroughput().
    ZstdCompression(3).
    Build(ctx, bus)
```

### 2. 高吞吐量场景

```go
eventbus.NewTopicBuilder("events").
    WithPartitions(20).
    WithReplication(3).
    Lz4Compression(). // 最快速度
    Build(ctx, bus)
```

### 3. 存储优先场景

```go
eventbus.NewTopicBuilder("logs").
    WithPartitions(10).
    WithReplication(3).
    GzipCompression(9). // 最高压缩率
    WithRetention(30 * 24 * time.Hour). // 长期保留
    Build(ctx, bus)
```

### 4. 低延迟场景

```go
eventbus.NewTopicBuilder("realtime").
    WithPartitions(10).
    WithReplication(3).
    NoCompression(). // 无压缩，最低延迟
    Build(ctx, bus)
```

---

## ⚠️ 注意事项

### 1. 压缩算法兼容性

- ✅ 不同压缩算法的消息可以共存于同一 topic
- ✅ 消费者会自动解压，无需额外配置
- ⚠️ 确保 Kafka 版本支持所选压缩算法

### 2. 压缩效果

- ✅ 文本/JSON 数据压缩效果好
- ⚠️ 二进制数据（图片、视频）压缩效果有限
- ⚠️ 已压缩的数据（如 gzip 文件）不建议再次压缩

### 3. 性能影响

- ✅ 压缩可以减少网络传输时间
- ⚠️ 压缩/解压会增加 CPU 开销
- ⚠️ 需要根据实际场景权衡

### 4. 配置建议

- ✅ 压缩算法一旦设置，建议不要频繁更改
- ✅ 在测试环境验证压缩效果
- ✅ 监控 CPU 和网络指标

---

## 🔧 完整示例

```go
package main

import (
    "context"
    "time"
    
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

func main() {
    ctx := context.Background()
    
    // 创建 Kafka EventBus
    kafkaConfig := &eventbus.KafkaConfig{
        Brokers: []string{"localhost:9092"},
        // ... 其他配置
    }
    bus, _ := eventbus.NewKafkaEventBus(kafkaConfig)
    defer bus.Close()
    
    // 创建高吞吐量 topic（Snappy 压缩）
    eventbus.NewTopicBuilder("orders").
        ForHighThroughput().
        Build(ctx, bus)
    
    // 创建存储优先 topic（GZIP 压缩）
    eventbus.NewTopicBuilder("logs").
        WithPartitions(10).
        WithReplication(3).
        GzipCompression(9).
        WithRetention(30 * 24 * time.Hour).
        Build(ctx, bus)
    
    // 创建低延迟 topic（无压缩）
    eventbus.NewTopicBuilder("realtime").
        WithPartitions(10).
        WithReplication(3).
        NoCompression().
        Build(ctx, bus)
}
```

---

## 📚 参考资料

- [Kafka 官方文档 - Compression](https://kafka.apache.org/documentation/#compression)
- [Snappy 压缩算法](https://github.com/google/snappy)
- [LZ4 压缩算法](https://github.com/lz4/lz4)
- [Zstandard 压缩算法](https://github.com/facebook/zstd)

---

## ✅ 总结

TopicBuilder 的压缩配置功能提供了：

1. ✅ **5种压缩算法**：none, lz4, snappy, gzip, zstd
2. ✅ **快捷方法**：一行代码配置压缩
3. ✅ **预设配置**：高/中/低吞吐量预设已包含 snappy 压缩
4. ✅ **灵活覆盖**：可以覆盖预设的压缩配置
5. ✅ **完整验证**：自动验证压缩算法和级别

**推荐配置：**
- 生产环境：Snappy 或 Zstd
- 高吞吐量：LZ4
- 存储优先：GZIP
- 低延迟：None

