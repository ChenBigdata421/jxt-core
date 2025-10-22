-- Migration: 添加 TraceID 和 CorrelationID 字段
-- Version: 001
-- Date: 2025-10-20
-- Description: 为 outbox_events 表添加分布式追踪和事件关联字段

-- ============================================================
-- 1. 添加 TraceID 和 CorrelationID 字段
-- ============================================================

ALTER TABLE outbox_events 
ADD COLUMN trace_id VARCHAR(64) DEFAULT '' COMMENT '链路追踪ID',
ADD COLUMN correlation_id VARCHAR(64) DEFAULT '' COMMENT '关联ID';

-- ============================================================
-- 2. 添加索引（用于查询和追踪）
-- ============================================================

-- TraceID 索引：用于按链路追踪ID查询事件
CREATE INDEX idx_trace_id ON outbox_events(trace_id);

-- CorrelationID 索引：用于按关联ID查询相关事件
CREATE INDEX idx_correlation_id ON outbox_events(correlation_id);

-- ============================================================
-- 3. 修改 Version 字段类型（可选）
-- ============================================================

-- 将 Version 从 INT 改为 BIGINT，与 EventBus Envelope 保持一致
-- 注意：如果表中已有数据，这个操作可能需要一些时间
ALTER TABLE outbox_events 
MODIFY COLUMN version BIGINT NOT NULL DEFAULT 1 COMMENT '事件版本';

-- ============================================================
-- 4. 验证迁移
-- ============================================================

-- 查看表结构
-- DESCRIBE outbox_events;

-- 查看索引
-- SHOW INDEX FROM outbox_events;

-- ============================================================
-- 回滚脚本（如果需要）
-- ============================================================

-- 删除索引
-- DROP INDEX idx_trace_id ON outbox_events;
-- DROP INDEX idx_correlation_id ON outbox_events;

-- 删除字段
-- ALTER TABLE outbox_events 
-- DROP COLUMN trace_id,
-- DROP COLUMN correlation_id;

-- 恢复 Version 字段类型
-- ALTER TABLE outbox_events 
-- MODIFY COLUMN version INT NOT NULL DEFAULT 1 COMMENT '事件版本';

