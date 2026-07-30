-- 003_add_dead_lettered.sql
-- 为 outbox_events 增加发布侧死信终态字段（opus5-RCC-v2 §2.2 M2 / C1）。
-- status 枚举新增终态值 'dead_lettered'（varchar(20) 足够，无需改列类型）。
-- 「终态事实」与「通知是否已发」拆成两个独立事实，避免引入会孤儿化的 dead_lettering 中间态。

-- ============================================================
-- MySQL
-- ============================================================
ALTER TABLE outbox_events
  ADD COLUMN dead_lettered_at DATETIME(3) NULL COMMENT '发布侧死信终态时间（status=dead_lettered 时填充）',
  ADD COLUMN dlq_notified_at  DATETIME(3) NULL COMMENT '死信通知成功时间；NULL=待补发';

CREATE INDEX idx_outbox_dlq_notify ON outbox_events (status, dlq_notified_at, id);

-- ============================================================
-- PostgreSQL
-- ============================================================
-- ALTER TABLE outbox_events
--   ADD COLUMN dead_lettered_at TIMESTAMPTZ,
--   ADD COLUMN dlq_notified_at  TIMESTAMPTZ;
-- CREATE INDEX idx_outbox_dlq_notify ON outbox_events (status, dlq_notified_at, id)
--   WHERE status = 'dead_lettered' AND dlq_notified_at IS NULL;

-- ============================================================
-- 回滚（MySQL）
-- ============================================================
-- DROP INDEX idx_outbox_dlq_notify ON outbox_events;
-- ALTER TABLE outbox_events
--   DROP COLUMN dead_lettered_at,
--   DROP COLUMN dlq_notified_at;
