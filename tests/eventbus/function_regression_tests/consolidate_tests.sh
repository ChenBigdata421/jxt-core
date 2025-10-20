#!/bin/bash

set -e

echo "��� 开始整合测试文件..."

# 备份原文件
echo "��� 备份原文件..."
mkdir -p backup
cp *_test.go backup/ 2>/dev/null || true

# 1. 创建 kafka_nats_test.go (已存在，追加内容)
echo "��� 整合 Kafka/NATS 测试..."

# 追加 envelope_test.go 的内容 (跳过package和import)
echo "" >> kafka_nats_test.go
echo "// ============================================================================" >> kafka_nats_test.go
echo "// Envelope 测试 (来自 envelope_test.go)" >> kafka_nats_test.go
echo "// ============================================================================" >> kafka_nats_test.go
echo "" >> kafka_nats_test.go
sed -n '/^\/\/ TestKafkaEnvelopePublishSubscribe/,$p' envelope_test.go >> kafka_nats_test.go

# 追加 lifecycle_test.go 的内容
echo "" >> kafka_nats_test.go
echo "// ============================================================================" >> kafka_nats_test.go
echo "// 生命周期测试 (来自 lifecycle_test.go)" >> kafka_nats_test.go
echo "// ============================================================================" >> kafka_nats_test.go
echo "" >> kafka_nats_test.go
sed -n '/^\/\/ TestKafkaClose/,$p' lifecycle_test.go >> kafka_nats_test.go

# 追加 topic_config_test.go 的内容
echo "" >> kafka_nats_test.go
echo "// ============================================================================" >> kafka_nats_test.go
echo "// 主题配置测试 (来自 topic_config_test.go)" >> kafka_nats_test.go
echo "// ============================================================================" >> kafka_nats_test.go
echo "" >> kafka_nats_test.go
sed -n '/^\/\/ TestKafkaTopicConfiguration/,$p' topic_config_test.go >> kafka_nats_test.go

echo "✅ kafka_nats_test.go 整合完成"

# 2. 创建 monitoring_test.go
echo "��� 整合监控测试..."
cat > monitoring_test.go << 'EOF'
package function_tests

import (
"context"
"sync/atomic"
"testing"
"time"

"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// ============================================================================
// 积压监控测试 (来自 backlog_test.go)
// ============================================================================

EOF

sed -n '/^\/\/ TestKafkaSubscriberBacklogMonitoring/,$p' backlog_test.go >> monitoring_test.go

echo "" >> monitoring_test.go
echo "// ============================================================================" >> monitoring_test.go
echo "// 健康检查测试 (来自 healthcheck_test.go)" >> monitoring_test.go
echo "// ============================================================================" >> monitoring_test.go
echo "" >> monitoring_test.go

sed -n '/^\/\/ TestKafkaHealthCheckPublisher/,$p' healthcheck_test.go >> monitoring_test.go

echo "✅ monitoring_test.go 整合完成"

# 3. 创建 integration_test.go
echo "��� 整合集成测试..."
cat > integration_test.go << 'EOF'
package function_tests

import (
"context"
"sync/atomic"
"testing"
"time"

"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
"github.com/stretchr/testify/assert"
"github.com/stretchr/testify/require"
)

// ============================================================================
// 端到端集成测试 (来自 e2e_integration_test.go)
// ============================================================================

EOF

sed -n '/^\/\/ TestE2E_MemoryEventBus_WithEnvelope/,$p' e2e_integration_test.go >> integration_test.go

echo "" >> integration_test.go
echo "// ============================================================================" >> integration_test.go
echo "// JSON 配置测试 (来自 json_config_test.go)" >> integration_test.go
echo "// ============================================================================" >> integration_test.go
echo "" >> integration_test.go

sed -n '/^\/\/ TestMarshalToString/,$p' json_config_test.go >> integration_test.go

echo "✅ integration_test.go 整合完成"

echo ""
echo "��� 测试文件整合完成！"
echo ""
echo "��� 整合结果:"
echo "  - kafka_nats_test.go: Kafka/NATS 基础功能测试"
echo "  - monitoring_test.go: 监控相关测试"
echo "  - integration_test.go: 集成和工具测试"
echo ""
echo "��� 原文件已备份到 backup/ 目录"

