package postgres

import "gorm.io/gorm"

// CreateTableSQL 建 four tables（PostgreSQL 方言）。与 mysql 语义等价，类型用 PG 原生 + partial index。
const CreateTableSQL = `
CREATE TABLE IF NOT EXISTS event_consumption (
  id             BIGSERIAL    PRIMARY KEY,
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
  claimed_at     TIMESTAMP(3),
  lease_expires_at TIMESTAMP(3),
  last_attempt_at TIMESTAMP(3),
  error_class    VARCHAR(16),
  error_code     VARCHAR(64),
  error_fingerprint CHAR(64),
  error_message  TEXT,
  next_attempt_at TIMESTAMP(3),
  replay_mode    VARCHAR(8),
  replay_requested_by VARCHAR(100),
  replay_approved_by  VARCHAR(100),
  replay_reason  TEXT,
  replay_auth_id CHAR(36),
  replay_auth_consumed_at TIMESTAMP(3),
  payload        BYTEA,
  raw_key        BYTEA,
  headers        JSONB,
  src_partition  INT,
  src_offset     BIGINT,
  raw_payload_hash CHAR(64),
  broker_timestamp TIMESTAMP(3),
  resolved_at    TIMESTAMP(3),
  resolved_by    VARCHAR(100),
  discard_reason TEXT,
  first_seen_at  TIMESTAMP(3) NOT NULL,
  created_at     TIMESTAMP(3) NOT NULL,
  updated_at     TIMESTAMP(3) NOT NULL,
  CONSTRAINT uk_event_handler UNIQUE (event_id, handler_id, item_key),
  CONSTRAINT chk_consumption_status CHECK (status IN ('PROCESSING','SUCCEEDED','RETRY_SCHEDULED','DEAD_LETTER','DISCARDED')),
  CONSTRAINT chk_consumption_attempt CHECK (attempt >= 1),
  CONSTRAINT chk_processing_owner CHECK (status <> 'PROCESSING' OR (claim_id IS NOT NULL AND claimed_at IS NOT NULL AND lease_expires_at IS NOT NULL)),
  CONSTRAINT chk_retry_due CHECK (status <> 'RETRY_SCHEDULED' OR (payload IS NOT NULL AND next_attempt_at IS NOT NULL AND error_class IS NOT NULL)),
  CONSTRAINT chk_dead_payload CHECK (status <> 'DEAD_LETTER' OR (payload IS NOT NULL AND error_class IS NOT NULL))
);
-- D22（本轮评审）：idx_due 保留 partial（只索 RETRY_SCHEDULED，索引最小），但 FindEligibleHeads 的
-- SQL 已把 status 写成字面量——参数化 status = $1 在 generic plan 下无法蕴含 partial 谓词，
-- 会间歇退化为 Seq Scan（执行 5 次后才切 generic plan，所以早期测试往往看不出来）。
-- 两者必须成对修：字面量查询 + partial 索引，否则 EXPLAIN 门禁无法是确定性的。
CREATE INDEX IF NOT EXISTS idx_due      ON event_consumption (next_attempt_at) WHERE status = 'RETRY_SCHEDULED';
CREATE INDEX IF NOT EXISTS idx_lease    ON event_consumption (lease_expires_at) WHERE status = 'PROCESSING';
CREATE INDEX IF NOT EXISTS idx_ops      ON event_consumption (tenant_id, status, first_seen_at);
CREATE INDEX IF NOT EXISTS idx_handler  ON event_consumption (handler_id, status);
-- D22：尾部加 first_seen_at，与 MySQL 逐字对齐（NOT EXISTS 在无 causal_seq 时比 first_seen_at）。
CREATE INDEX IF NOT EXISTS idx_aggregate ON event_consumption (tenant_id, aggregate_type, aggregate_id, status, causal_seq, src_partition, src_offset, first_seen_at);

CREATE TABLE IF NOT EXISTS consumption_anomalies (
  id BIGSERIAL PRIMARY KEY, kind VARCHAR(32) NOT NULL, event_id VARCHAR(64),
  handler_id VARCHAR(100),
  -- B8：与 AnomalyModel.TenantID int / MySQL DDL 对齐
  tenant_id INT NOT NULL DEFAULT 0,
  claim_id VARCHAR(36) NOT NULL DEFAULT '',
  detail TEXT, created_at TIMESTAMP(3) NOT NULL,
  -- 幂等键（本轮评审）：ObserveExpiredLeases 每 tick 反复扫到同一孤儿行，靠此唯一键 +
  -- ON CONFLICT DO NOTHING 保证同一次占位只记一条，避免 LEASE_ORPHAN 告警自噪。
  CONSTRAINT uk_anomaly_once UNIQUE (kind, event_id, handler_id, claim_id)
);
CREATE INDEX IF NOT EXISTS idx_kind_time ON consumption_anomalies (kind, created_at);

CREATE TABLE IF NOT EXISTS raw_message_quarantine (
  id BIGSERIAL PRIMARY KEY, handler_id VARCHAR(100) NOT NULL, topic VARCHAR(100) NOT NULL,
  src_partition INT NOT NULL, src_offset BIGINT NOT NULL, raw_value BYTEA NOT NULL, raw_key BYTEA,
  headers JSONB NOT NULL, raw_payload_hash CHAR(64) NOT NULL, broker_timestamp TIMESTAMP(3),
  error_message TEXT, status VARCHAR(16) NOT NULL, row_version BIGINT NOT NULL DEFAULT 1,
  resolved_at TIMESTAMP(3), resolved_by VARCHAR(100), created_at TIMESTAMP(3) NOT NULL,
  CONSTRAINT uk_raw_delivery UNIQUE (topic, src_partition, src_offset, handler_id)
);
CREATE INDEX IF NOT EXISTS idx_raw_status ON raw_message_quarantine (status, created_at);

CREATE TABLE IF NOT EXISTS consumption_aggregate_leases (
  tenant_id INT NOT NULL, aggregate_type VARCHAR(64) NOT NULL, aggregate_id VARCHAR(100) NOT NULL,
  holder_id VARCHAR(100) NOT NULL, acquired_at TIMESTAMP(3) NOT NULL, expires_at TIMESTAMP(3) NOT NULL,
  PRIMARY KEY (tenant_id, aggregate_type, aggregate_id)
);
-- D3（本轮评审）：ReleaseAggregateGate 按 holder_id 删，ReclaimExpiredAggregateGates 按 expires_at 扫；
-- 无索引时两者都是全表扇，而 gate 在重放热路径上。与 MySQL DDL 对齐。
CREATE INDEX IF NOT EXISTS idx_holder ON consumption_aggregate_leases (holder_id);
CREATE INDEX IF NOT EXISTS idx_gate_expires ON consumption_aggregate_leases (expires_at);
`

const DropTableSQL = `
DROP TABLE IF EXISTS consumption_aggregate_leases;
DROP TABLE IF EXISTS raw_message_quarantine;
DROP TABLE IF EXISTS consumption_anomalies;
DROP TABLE IF EXISTS event_consumption;
`

func Migration() func(*gorm.DB) error {
	return func(db *gorm.DB) error { return db.Exec(CreateTableSQL).Error }
}
