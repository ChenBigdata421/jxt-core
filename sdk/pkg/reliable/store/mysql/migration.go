package mysql

import "gorm.io/gorm"

// CreateTableSQL 建四张表（MySQL 方言，严格对齐 §2.1/§2.3 DDL）。
const CreateTableSQL = `
CREATE TABLE IF NOT EXISTS event_consumption (
  id             BIGINT       NOT NULL AUTO_INCREMENT,
  event_id       VARCHAR(64)  NOT NULL,
  item_key       VARCHAR(100) NOT NULL DEFAULT '',
  handler_id     VARCHAR(100) NOT NULL,
  tenant_id      INT          NOT NULL,
  event_type     VARCHAR(64),
  aggregate_type VARCHAR(64),
  aggregate_id   VARCHAR(100),
  causal_seq     BIGINT,
  topic          VARCHAR(100) NOT NULL,
  status         VARCHAR(16)  NOT NULL,
  attempt        INT          NOT NULL DEFAULT 1,
  replay_generation INT       NOT NULL DEFAULT 0,
  row_version    BIGINT       NOT NULL DEFAULT 1,
  claim_id       CHAR(36),
  claimed_at     DATETIME(3),
  lease_expires_at DATETIME(3),
  last_attempt_at DATETIME(3),
  error_class    VARCHAR(16),
  error_code     VARCHAR(64),
  error_fingerprint CHAR(64),
  error_message  TEXT,
  next_attempt_at DATETIME(3),
  replay_mode    VARCHAR(8),
  replay_requested_by VARCHAR(100),
  replay_approved_by  VARCHAR(100),
  replay_reason  TEXT,
  replay_auth_id CHAR(36),
  replay_auth_consumed_at DATETIME(3),
  payload        LONGBLOB,
  raw_key        LONGBLOB,  -- 与 PG BYTEA 对等（曾 VARBINARY(512) 致 >512B Kafka key 在严格模式 INSERT 失败 → 卡分区；PG 无界不受影响）
  headers        JSON,
  src_partition  INT,
  src_offset     BIGINT,
  raw_payload_hash CHAR(64),
  broker_timestamp DATETIME(3),
  resolved_at    DATETIME(3),
  resolved_by    VARCHAR(100),
  discard_reason TEXT,
  first_seen_at  DATETIME(3) NOT NULL,
  created_at     DATETIME(3) NOT NULL,
  updated_at     DATETIME(3) NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_event_consumption (event_id, handler_id, item_key),
  KEY idx_lease    (status, lease_expires_at),
  -- §16.12 索引合并（2026-09-15）：将 5 个 status 前导索引合并为 3 个，减少写放大。
  -- MySQL 无 partial index，PROCESSING 行会进入所有 status 前导索引，而 idx_due/idx_unresolved/
  -- idx_retention 的消费者从不查询 PROCESSING 行——纯死重写放大（PG 侧用 partial index 规避）。
  --
  -- Tier 1 合并：idx_ops + idx_retention → idx_ops (status, first_seen_at, updated_at)
  --   DeleteSettledBefore (status='SUCCEEDED' AND updated_at<?)：updated_at 是第 3 列，
  --   无法 range 但索引覆盖（index-only scan on SUCCEEDED range）。List/Count 同级。
  --
  -- Tier 2 合并：idx_due + idx_unresolved → idx_replay (status, next_attempt_at, handler_id, first_seen_at)
  --   FindEligibleHeads (status='RETRY_SCHEDULED' AND next_attempt_at<=? ORDER BY next_attempt_at)：
  --   前两列完美 range + 无 filesort，与原 idx_due 等价。
  --   RetryAgeSeconds/PendingCounts (GROUP BY handler_id)：覆盖扫描，丢失松散索引扫描
  --   （handler_id 降至第 3 列），退化为覆盖扫描 + 临时表聚合。30s 探针，绝对开销毫秒级。
  --
  -- 合并后：INSERT 写 4 个索引项（3 status 前导 + idx_aggregate），
  -- UPDATE status→SUCCEEDED 触及 6 次（3 棵树 × 删+插），约 -40% 写放大。
  --
  -- 仅随 CREATE TABLE 生效（本迁移无 ALTER 机制，review OV⑧b）。
  --
  -- 附录 Z（jxt-benchmark/docs/analysis/HTTP多租户并发上传-死锁风暴与分区阻塞
  -- 根因分析_20260823 - v1.md，Z.4/Z.6，2026-09-01 内核侧落地）：
  --   1. idx_handler (handler_id, status) 不建——原 KEY 行整行移除。零专属消费者
  --      （探针全部 status 打头走 idx_replay；opsvc List/Stats 走 idx_ops；
  --      handler 定位由 uk_event_consumption 覆盖），且是 2026-08-23 死锁风暴 dump
  --      里 T2 手持 29 把锁的检索路径；状态迁移的二级索引维护 7 → 6 → 5 → 3（约 -57%）。
  --   2. idx_ops 去前导 tenant_id（3 列 → 2 列 → 3 列含 updated_at）——一库一租户下基数为 1
  --      （opsprobe 明确「正确性依赖一库一租户」），前导常量列纯属写放大浪费；opsvc 的 tenant
  --      等值退化为常量过滤，(status, first_seen_at, updated_at) 保住 status 等值 + 时间区间的
  --      干净 range + DeleteSettledBefore 的 index-only scan。
  KEY idx_ops      (status, first_seen_at, updated_at),
  -- PR-7 opsprobe（review D7=6A）：RetryAgeSeconds/PendingCounts/FrozenAggregates 外层过滤均为
  -- 「未解决两态」，无索引则每 30s 探针全表扫。MySQL 无 partial index → 普通复合；两态行罕见，索引小。
  -- §16.12 合并 idx_due + idx_unresolved → idx_replay：
  -- (status, next_attempt_at, handler_id, first_seen_at) 同时服务 FindEligibleHeads
  -- （status + next_attempt_at range + ORDER BY）和 opsprobe（status eq + handler_id 覆盖扫描）。
  KEY idx_replay   (status, next_attempt_at, handler_id, first_seen_at),
  -- D22：尾部加 first_seen_at——FindEligibleHeads 的 NOT EXISTS 在事件不带 causal_seq 时按 first_seen_at
  -- 比较（准入 ⑩），无此列则子查询逐行回表，10K 行规模下退化为 O(N²)。
  -- 附录 Z Tier 3：去前导 tenant_id（8 列 → 7 列）——一库一租户下基数为 1，前导常量列
  -- 纯属写放大浪费；新列序与 FindEligibleHeads NOT EXISTS 谓词逐列对齐。
  KEY idx_aggregate (aggregate_type, aggregate_id, status, causal_seq, src_partition, src_offset, first_seen_at),
  CONSTRAINT chk_consumption_status CHECK (status IN ('PROCESSING','SUCCEEDED','RETRY_SCHEDULED','DEAD_LETTER','DISCARDED')),
  CONSTRAINT chk_consumption_attempt CHECK (attempt >= 1),
  CONSTRAINT chk_processing_owner CHECK (status <> 'PROCESSING' OR (claim_id IS NOT NULL AND claimed_at IS NOT NULL AND lease_expires_at IS NOT NULL)),
  CONSTRAINT chk_retry_due CHECK (status <> 'RETRY_SCHEDULED' OR (payload IS NOT NULL AND next_attempt_at IS NOT NULL AND error_class IS NOT NULL)),
  CONSTRAINT chk_dead_payload CHECK (status <> 'DEAD_LETTER' OR (payload IS NOT NULL AND error_class IS NOT NULL))
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS consumption_anomalies (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  kind         VARCHAR(32) NOT NULL,
  -- review #11：列入 uk_anomaly_once 的列须 NOT NULL DEFAULT ''——NULL 在唯一索引里互不相等，
  -- 否则缺 event/handler 上下文的异常 kind 会让幂等失效、anomaly 成倍写入刷爆告警（与 claim_id 同处理）。
  event_id     VARCHAR(64) NOT NULL DEFAULT '',
  handler_id   VARCHAR(100) NOT NULL DEFAULT '',
  tenant_id    INT NOT NULL DEFAULT 0,  -- B8：与 AnomalyModel.TenantID int 对齐（D18#8 RecordAnomaly 必传）
  claim_id     VARCHAR(36) NOT NULL DEFAULT '',
  detail       TEXT,
  created_at   DATETIME(3) NOT NULL,
  KEY idx_kind_time (kind, created_at),
  -- 幂等键（本轮评审）：ObserveExpiredLeases 每 tick 反复扫到同一孤儿行，靠此唯一键 +
  -- ON CONFLICT DO NOTHING 保证「同一次占位的同类异常只记一条」，避免告警自噪。
  -- review #6：键含 tenant_id——纵然当前每租户独立库（库内 tenant_id 恒定），一旦 store 跨租户，
  -- 缺 tenant_id 会让第二租户的同 (event,handler,claim) 异常被 ON CONFLICT 静默丢弃、告警欠计。
  UNIQUE KEY uk_anomaly_once (kind, tenant_id, event_id, handler_id, claim_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS raw_message_quarantine (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  -- review #1：租户隔离——隔离区同样按租户隔离（与 event_consumption.tenant_id 对齐）。
  tenant_id INT NOT NULL,
  handler_id VARCHAR(100) NOT NULL,
  topic VARCHAR(100) NOT NULL,
  src_partition INT NOT NULL,
  src_offset BIGINT NOT NULL,
  raw_value LONGBLOB NOT NULL,
  raw_key LONGBLOB,  -- 与 PG BYTEA 对等（同 event_consumption.raw_key）
  headers JSON NOT NULL,
  raw_payload_hash CHAR(64) NOT NULL,
  broker_timestamp DATETIME(3),
  error_message TEXT,
  status VARCHAR(16) NOT NULL,
  row_version BIGINT NOT NULL DEFAULT 1,
  -- PR-7 Task 3（C①，review D1 + review M-3）：本块为存量库新增两列，缺一不可——
  --   (1) replay_attempts：QuarantineReplay 的失败重放计数（OV⑤③ 上限）。
  --   (2) updated_at：watchdog 谓词列（OV⑤④，REPLAYING 超时重claim / Task 14 sweep）。
  -- 两列都仅随 CREATE TABLE 生效——本迁移无 ALTER 机制（与 idx_unresolved 同一 OV⑧b 缺口类）：
  -- 存量 MySQL 库（evidence-command 租户库）由 evidence 侧迁移【同时补两列】
  -- command/cmd/migrate/migration/version/2026082300003_add_quarantine_replay_attempts.go
  -- （⚠ 同为 PR-7 服务侧待落地交付物，内核合入时尚未创建）
  -- （information_schema.COLUMNS 存在性守卫——MySQL 任何版本（5.7/8.0/8.4）都不支持
  -- ADD COLUMN IF NOT EXISTS（那是 MariaDB 语法；MySQL 8.0.29 加的是 INSTANT 加列性能特性，
  -- 非条件 DDL），版本升级也解除不了这条守卫）。
  -- ⚠ M-3 证据指针：只补 replay_attempts 不补 updated_at 的库，QuarantineReplay 首次点击
  -- 即在 casToReplaying 的 UPDATE（写 updated_at 列）上报 Error 1054 unknown column——
  -- D1 失败类。⚠ blast radius 大于首击点（review 复核）：QuarantineModel 的
  -- Record（隔离写入路径，adapters write-before-ACK）同样 INSERT 新列——未迁移库上
  -- 【首次毒消息隔离落库】即报 1054，错误上抛 → 分区阻塞（strategy-A fail-closed），
  -- 早于任何 QuarantineReplay 点击。evidence 侧迁移作者须照抄两列，不得只加计数列。
  -- file-storage 的租户库是 PostgreSQL only，内核侧 ADD COLUMN IF NOT EXISTS 已自愈（见
  -- postgres/migration.go 文末两条 ALTER），无需 evidence 式迁移。
  replay_attempts INT NOT NULL DEFAULT 0,
  resolved_at DATETIME(3),
  resolved_by VARCHAR(100),
  created_at DATETIME(3) NOT NULL,
  -- OV⑤④ watchdog 谓词列，可空（新行由 GORM autoUpdateTime 写创建时间——见 model.go M-1 注释；
  -- CAS 迁移显式置 now）。存量库补列同走 2026082300003 evidence 侧迁移（与 replay_attempts
  -- 同文件、同一次补齐，见上方 M-3 注释）。
  updated_at DATETIME(3),
  -- review #1（纵深防御）：键含 tenant_id——与 consumption_anomalies.uk_anomaly_once 同理。共享库下两租户
  -- 撞上相同 (topic,partition,offset,handler) 时，缺 tenant_id 会让第二租户的毒消息被 ON CONFLICT DO NOTHING
  -- 静默吞掉、回读还拿到第一租户 id（丢消息 + id 污染 + ACK 语义错误）。当前每租户独立库不触发，纵深对齐。
  UNIQUE KEY uk_raw_delivery (tenant_id, topic, src_partition, src_offset, handler_id),
  KEY idx_raw_status (tenant_id, status, created_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS consumption_aggregate_leases (
  tenant_id      INT NOT NULL,
  aggregate_type VARCHAR(64) NOT NULL,
  aggregate_id   VARCHAR(100) NOT NULL,
  holder_id      VARCHAR(100) NOT NULL,
  acquired_at    DATETIME(3) NOT NULL,
  expires_at     DATETIME(3) NOT NULL,
  PRIMARY KEY (tenant_id, aggregate_type, aggregate_id),
  -- D3（本轮评审）：ReleaseAggregateGate 按 holder_id 删除，无此索引则每次释放 gate 都是全表扇 + 行锁；
  -- gate 在重放热路径上，并发释放会互相阻塞。ReclaimExpiredAggregateGates 按 expires_at 扫，同理。
  KEY idx_holder (holder_id),
  KEY idx_gate_expires (expires_at)
) ENGINE=InnoDB;
`

// DropTableSQL 回滚（repotest migration down 用）。
const DropTableSQL = `
DROP TABLE IF EXISTS consumption_aggregate_leases;
DROP TABLE IF EXISTS raw_message_quarantine;
DROP TABLE IF EXISTS consumption_anomalies;
DROP TABLE IF EXISTS event_consumption;
`

// Migration 返回建表闭包（与 outbox/migrations 风格一致；DSN 需 multiStatements=true）。
func Migration() func(*gorm.DB) error {
	return func(db *gorm.DB) error { return db.Exec(CreateTableSQL).Error }
}
