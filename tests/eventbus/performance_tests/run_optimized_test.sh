#!/bin/bash

# Worker 池优化验证测试脚本
# 用途：验证 NATS Worker 池大小优化和 Kafka 协程泄漏修复的效果

set -e

echo "============================================================================"
echo "🚀 Worker 池优化验证测试"
echo "============================================================================"
echo ""
echo "优化内容："
echo "  1. NATS Worker 池大小: 48 → 256 (+433%)"
echo "  2. Kafka 协程泄漏修复: dispatcher goroutine 加入 WaitGroup"
echo ""
echo "============================================================================"
echo ""

# 记录开始时间
START_TIME=$(date +%s)

# 运行测试
echo "📊 运行内存持久化性能对比测试..."
echo ""

go test -v -run TestMemoryPersistenceComparison -timeout 30m 2>&1 | tee memory_test_optimized_output.log

# 记录结束时间
END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo ""
echo "============================================================================"
echo "✅ 测试完成"
echo "============================================================================"
echo ""
echo "测试时长: ${DURATION} 秒"
echo "日志文件: memory_test_optimized_output.log"
echo ""
echo "============================================================================"
echo "📊 关键指标对比（预期）"
echo "============================================================================"
echo ""
echo "NATS 吞吐量:"
echo "  优化前: 387.70 msg/s"
echo "  优化后: 2,000+ msg/s (预期)"
echo "  提升:   +400%+ (预期)"
echo ""
echo "Kafka 协程泄漏:"
echo "  优化前: 134 个"
echo "  优化后: ~133 个 (预期)"
echo "  改善:   -1 个 (预期)"
echo ""
echo "NATS Worker 池:"
echo "  优化前: 48 workers"
echo "  优化后: 256 workers"
echo "  提升:   +433%"
echo ""
echo "============================================================================"
echo ""
echo "请查看日志文件获取详细测试结果"
echo ""

