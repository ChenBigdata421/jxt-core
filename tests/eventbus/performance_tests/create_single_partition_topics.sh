#!/bin/bash

# 创建单分区 Kafka Topics 用于顺序验证测试
# 用途：验证 Keyed-Worker Pool 的顺序保证是否正确

KAFKA_BROKER="localhost:29094"

echo "🔧 创建单分区 Kafka Topics..."
echo "================================"

# 定义压力级别
PRESSURES=("low" "medium" "high" "extreme")

for pressure in "${PRESSURES[@]}"; do
    echo ""
    echo "📋 创建 ${pressure} 压力级别的 topics..."
    
    for i in {1..5}; do
        TOPIC="kafka.perf.${pressure}.topic${i}"
        
        # 检查 topic 是否已存在
        if kafka-topics.sh --describe --topic "$TOPIC" --bootstrap-server "$KAFKA_BROKER" &>/dev/null; then
            echo "  ⚠️  Topic $TOPIC 已存在，跳过创建"
        else
            # 创建单分区 topic
            kafka-topics.sh --create \
                --topic "$TOPIC" \
                --partitions 1 \
                --replication-factor 1 \
                --bootstrap-server "$KAFKA_BROKER" \
                --config retention.ms=3600000 \
                --config segment.ms=3600000
            
            if [ $? -eq 0 ]; then
                echo "  ✅ 成功创建 topic: $TOPIC (1 partition)"
            else
                echo "  ❌ 创建 topic 失败: $TOPIC"
            fi
        fi
    done
done

echo ""
echo "================================"
echo "✅ 单分区 Topics 创建完成！"
echo ""
echo "📊 验证创建的 topics:"
echo ""

for pressure in "${PRESSURES[@]}"; do
    echo "🔍 ${pressure} 压力级别:"
    for i in {1..5}; do
        TOPIC="kafka.perf.${pressure}.topic${i}"
        kafka-topics.sh --describe --topic "$TOPIC" --bootstrap-server "$KAFKA_BROKER" 2>/dev/null | grep "PartitionCount"
    done
    echo ""
done

echo "💡 提示："
echo "  - 所有 topics 都配置为 1 个分区"
echo "  - 这将保证 100% 的全局顺序"
echo "  - 如果测试仍有顺序违反，说明问题在 Keyed-Worker Pool 或检测逻辑"
echo "  - 如果测试顺序违反为 0，说明问题在多分区并发消费"
echo ""
echo "🚀 现在可以运行测试："
echo "  cd tests/eventbus/performance_tests"
echo "  go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 15m"

