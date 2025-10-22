-- 添加幂等性键字段
-- 用于防止重复发布相同的事件

-- MySQL
ALTER TABLE outbox_events 
ADD COLUMN idempotency_key VARCHAR(512) DEFAULT '' COMMENT '幂等性键';

-- 创建唯一索引（防止重复）
CREATE UNIQUE INDEX idx_idempotency_key ON outbox_events(idempotency_key);

-- PostgreSQL
-- ALTER TABLE outbox_events 
-- ADD COLUMN idempotency_key VARCHAR(512) DEFAULT '';
-- 
-- COMMENT ON COLUMN outbox_events.idempotency_key IS '幂等性键';
-- 
-- CREATE UNIQUE INDEX idx_idempotency_key ON outbox_events(idempotency_key);

-- SQLite
-- ALTER TABLE outbox_events 
-- ADD COLUMN idempotency_key TEXT DEFAULT '';
-- 
-- CREATE UNIQUE INDEX idx_idempotency_key ON outbox_events(idempotency_key);

