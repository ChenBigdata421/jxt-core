# Outbox 数据库迁移脚本

本目录包含 Outbox 模式的数据库迁移脚本。

## 📋 迁移列表

| 版本 | 文件 | 说明 | 日期 |
|------|------|------|------|
| 001 | `001_add_trace_fields.sql` | 添加 TraceID 和 CorrelationID 字段 | 2025-10-20 |
| 002 | `002_add_idempotency_key.sql` | 添加 idempotency_key 字段 + 唯一索引（发布幂等） | 2025-10-22 |
| 003 | `003_add_dead_lettered.sql` | 添加 dead_lettered_at / dlq_notified_at + idx_outbox_dlq_notify（DLQ 终态） | 2026-07-27 |

---

## 🚀 使用方法

### 方法 1：手动执行（推荐用于生产环境）

⚠️ **必须按顺序执行 001 → 002 → 003，不能跳号。** 跳过 003 会让数据库缺少
`dead_lettered_at` / `dlq_notified_at` 列；一旦启用 DLQ（`EnableDLQ=true`），
`FindUnnotifiedDeadLettered` / `MarkDeadLetterNotified` 会在每个 DLQ tick 因列不存在而失败，
死信永远无法通知。

```bash
# 连接到数据库
mysql -u username -p database_name

# 按版本顺序执行
source /path/to/jxt-core/sdk/pkg/outbox/migrations/001_add_trace_fields.sql
source /path/to/jxt-core/sdk/pkg/outbox/migrations/002_add_idempotency_key.sql
source /path/to/jxt-core/sdk/pkg/outbox/migrations/003_add_dead_lettered.sql

# 验证迁移（含 003 的死信列与索引）
DESCRIBE outbox_events;
SHOW INDEX FROM outbox_events;
```

### 方法 2：使用 GORM AutoMigrate（开发环境）

```go
import (
    "github.com/ChenBigdata421/jxt-core/sdk/pkg/outbox/adapters/gorm"
    "gorm.io/gorm"
)

func main() {
    db, _ := gorm.Open(...)
    
    // 自动迁移（会自动添加新字段和索引）
    db.AutoMigrate(&gorm.OutboxEventModel{})
}
```

### 方法 3：使用迁移工具

#### 使用 golang-migrate

```bash
# 安装 golang-migrate
go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# 执行迁移
migrate -path ./migrations -database "mysql://user:pass@tcp(localhost:3306)/dbname" up
```

#### 使用 goose

```bash
# 安装 goose
go install github.com/pressly/goose/v3/cmd/goose@latest

# 执行迁移
goose -dir ./migrations mysql "user:pass@tcp(localhost:3306)/dbname" up
```

---

## 📊 迁移详情

### 001_add_trace_fields.sql

**目的**：为 Outbox 事件添加分布式追踪和事件关联支持

**变更内容**：

1. **新增字段**：
   - `trace_id` VARCHAR(64) - 链路追踪ID
   - `correlation_id` VARCHAR(64) - 关联ID

2. **新增索引**：
   - `idx_trace_id` - 用于按 TraceID 查询
   - `idx_correlation_id` - 用于按 CorrelationID 查询

3. **字段类型修改**：
   - `version` INT → BIGINT - 与 EventBus Envelope 保持一致

**影响**：
- ✅ 向后兼容（新字段默认为空字符串）
- ✅ 不影响现有数据
- ⚠️ 修改 `version` 字段类型可能需要一些时间（取决于表大小）

**回滚**：
```sql
-- 删除索引
DROP INDEX idx_trace_id ON outbox_events;
DROP INDEX idx_correlation_id ON outbox_events;

-- 删除字段
ALTER TABLE outbox_events 
DROP COLUMN trace_id,
DROP COLUMN correlation_id;

-- 恢复 Version 字段类型
ALTER TABLE outbox_events 
MODIFY COLUMN version INT NOT NULL DEFAULT 1 COMMENT '事件版本';
```

---

### 002_add_idempotency_key.sql

**目的**：发布幂等——同一事件重复发布不会产生多行。

**变更内容**：新增 `idempotency_key VARCHAR(512) DEFAULT ''` + 唯一索引 `idx_idempotency_key`。

**影响**：✅ 向后兼容（默认空串）；唯一索引是发布幂等的硬保证（`Save`/`SaveBatch` 重键由索引拒绝）。

---

### 003_add_dead_lettered.sql

**目的**：发布侧死信终态（C1）。把「发布已彻底失败」与「死信通知是否已发」拆成两个独立事实，
避免引入会孤儿化的 `dead_lettering` 中间态——crash 在通知步骤，下一轮 DLQ tick 补发。

**变更内容**：
1. **新增字段**（均可空）：
   - `dead_lettered_at DATETIME(3)` — 死信终态时间（`status='dead_lettered'` 时填充）
   - `dlq_notified_at DATETIME(3)` — 死信通知成功时间；`NULL` = 待补发
2. **新增复合索引**：`idx_outbox_dlq_notify (status, dlq_notified_at, id)` — 服务
   `FindUnnotifiedDeadLettered` 的 `WHERE status='dead_lettered' AND dlq_notified_at IS NULL ORDER BY id`。

**影响**：✅ 纯增量（可空列 + 索引），无需回填，部署窗口安全。

**⚠️ 前置条件**：启用 DLQ（`SchedulerConfig.EnableDLQ=true`）前**必须**已执行本迁移，否则
DLQ 扫描每 tick 报列不存在。

**验证**（必做）：
```sql
-- 字段存在且可空
SHOW COLUMNS FROM outbox_events WHERE Field IN ('dead_lettered_at','dlq_notified_at');
-- 复合索引列序 = (status, dlq_notified_at, id)
SHOW INDEX FROM outbox_events WHERE Key_name='idx_outbox_dlq_notify';
-- 无孤儿：终态行必须有 dead_lettered_at
SELECT COUNT(*) FROM outbox_events
  WHERE status='dead_lettered' AND dead_lettered_at IS NULL;  -- 期望 0
```

**回滚**：见 `003_add_dead_lettered.sql` 末尾的注释块（DROP INDEX + DROP COLUMN）。

---

## ✅ 验证迁移

### 1. 检查表结构

```sql
DESCRIBE outbox_events;
```

**预期输出**（包含新字段）：
```
+----------------+--------------+------+-----+---------+-------+
| Field          | Type         | Null | Key | Default | Extra |
+----------------+--------------+------+-----+---------+-------+
| ...            | ...          | ...  | ... | ...     | ...   |
| trace_id       | varchar(64)  | YES  | MUL |         |       |
| correlation_id | varchar(64)  | YES  | MUL |         |       |
| version        | bigint       | NO   |     | 1       |       |
+----------------+--------------+------+-----+---------+-------+
```

### 2. 检查索引

```sql
SHOW INDEX FROM outbox_events;
```

**预期输出**（包含新索引）：
```
+---------------+------------+---------------------+
| Table         | Key_name   | Column_name         |
+---------------+------------+---------------------+
| outbox_events | PRIMARY    | id                  |
| outbox_events | idx_trace_id | trace_id          |
| outbox_events | idx_correlation_id | correlation_id |
| ...           | ...        | ...                 |
+---------------+------------+---------------------+
```

### 3. 测试查询

```sql
-- 按 TraceID 查询
SELECT * FROM outbox_events WHERE trace_id = 'trace-123';

-- 按 CorrelationID 查询
SELECT * FROM outbox_events WHERE correlation_id = 'corr-456';

-- 查询同一链路的所有事件
SELECT id, event_type, trace_id, created_at 
FROM outbox_events 
WHERE trace_id = 'trace-123' 
ORDER BY created_at;

-- 查询同一业务流程的所有事件
SELECT id, event_type, correlation_id, created_at 
FROM outbox_events 
WHERE correlation_id = 'corr-456' 
ORDER BY created_at;
```

---

## 📝 最佳实践

### 1. 生产环境迁移

1. **备份数据库**
   ```bash
   mysqldump -u username -p database_name > backup_before_migration.sql
   ```

2. **在测试环境先执行**
   - 验证迁移脚本
   - 测试应用程序兼容性

3. **选择低峰期执行**
   - 修改 `version` 字段类型可能需要锁表
   - 建议在业务低峰期执行

4. **监控执行过程**
   ```sql
   -- 查看正在执行的 SQL
   SHOW PROCESSLIST;
   ```

### 2. 开发环境迁移

使用 GORM AutoMigrate 即可：

```go
db.AutoMigrate(&gorm.OutboxEventModel{})
```

### 3. 迁移后验证

```go
// 测试新字段
event, _ := outbox.NewOutboxEvent("tenant-1", "agg-1", "User", "UserCreated", payload)
event.WithTraceID("trace-123").WithCorrelationID("corr-456")

// 保存到数据库
repo.Save(ctx, event)

// 查询验证
events, _ := repo.FindByAggregateID(ctx, "agg-1", "tenant-1")
fmt.Printf("TraceID: %s, CorrelationID: %s\n", events[0].TraceID, events[0].CorrelationID)
```

---

## 🔧 故障排除

### 问题 1：索引创建失败

**错误**：`Duplicate key name 'idx_trace_id'`

**解决**：索引已存在，跳过或先删除旧索引
```sql
DROP INDEX idx_trace_id ON outbox_events;
```

### 问题 2：字段已存在

**错误**：`Duplicate column name 'trace_id'`

**解决**：字段已存在，跳过此步骤或检查字段定义是否正确
```sql
SHOW COLUMNS FROM outbox_events LIKE 'trace_id';
```

### 问题 3：修改 version 字段类型超时

**错误**：`Lock wait timeout exceeded`

**解决**：
1. 选择业务低峰期执行
2. 增加锁等待超时时间
   ```sql
   SET SESSION innodb_lock_wait_timeout = 300;
   ```
3. 或者跳过此步骤（INT 也可以工作，只是不够优雅）

---

## 📚 相关文档

- [Outbox 模式设计文档](../../../docs/outbox-pattern-design.md)
- [Outbox 使用文档](../README.md)
- [OutboxEvent 到 Envelope 映射分析](../../../OUTBOX_TO_ENVELOPE_MAPPING_ANALYSIS.md)

---

**版本**：v1.1
**最后更新**：2026-07-27

