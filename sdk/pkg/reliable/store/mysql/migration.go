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
  KEY idx_due      (status, next_attempt_at),
  KEY idx_lease    (status, lease_expires_at),
  KEY idx_ops      (tenant_id, status, first_seen_at),
  KEY idx_handler  (handler_id, status),
  -- D22：尾部加 first_seen_at——FindEligibleHeads 的 NOT EXISTS 在事件不带 causal_seq 时按 first_seen_at
  -- 比较（准入 ⑩），无此列则子查询逐行回表，10K 行规模下退化为 O(N²)。
  KEY idx_aggregate (tenant_id, aggregate_type, aggregate_id, status, causal_seq, src_partition, src_offset, first_seen_at),
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
  resolved_at DATETIME(3),
  resolved_by VARCHAR(100),
  created_at DATETIME(3) NOT NULL,
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
