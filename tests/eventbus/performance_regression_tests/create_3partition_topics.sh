#!/bin/bash

# 创建 3 分区的 Kafka Topics
# 用于测试多分区环境下的顺序保证

echo "🚀 开始创建 3 分区的 Kafka Topics..."

# Kafka 容器名称（根据 docker-compose-nats.yml）
KAFKA_CONTAINER="benchmark-kafka"
BOOTSTRAP_SERVER="localhost:29094"

# 检查 Kafka 容器是否运行
if ! docker ps | grep -q "$KAFKA_CONTAINER"; then
    echo "❌ Kafka 容器未运行，请先启动 docker-compose-nats.yml"
    exit 1
fi

# 压力级别
PRESSURES=("low" "medium" "high" "extreme")

# 为每个压力级别创建 5 个 topic，每个 topic 3 个分区
for pressure in "${PRESSURES[@]}"; do
    echo ""
    echo "📋 创建 $pressure 压力级别的 topics..."
    
    for i in {1..5}; do
        topic="kafka.perf.$pressure.topic$i"
        
        # 删除已存在的 topic（如果存在）
        docker exec $KAFKA_CONTAINER kafka-topics.sh \
            --delete \
            --topic "$topic" \
            --bootstrap-server $BOOTSTRAP_SERVER \
            2>/dev/null || true
        
        # 创建新的 topic（3 个分区）
        docker exec $KAFKA_CONTAINER kafka-topics.sh \
            --create \
            --topic "$topic" \
            --partitions 3 \
            --replication-factor 1 \
            --bootstrap-server $BOOTSTRAP_SERVER
        
        if [ $? -eq 0 ]; then
            echo "  ✅ 创建成功: $topic (3 partitions)"
        else
            echo "  ❌ 创建失败: $topic"
        fi
    done
done

echo ""
echo "🎉 所有 topics 创建完成！"
echo ""
echo "📊 验证创建的 topics:"
docker exec $KAFKA_CONTAINER kafka-topics.sh \
    --list \
    --bootstrap-server $BOOTSTRAP_SERVER | grep "kafka.perf"

echo ""
echo "📋 查看 topic 详情（示例）:"
docker exec $KAFKA_CONTAINER kafka-topics.sh \
    --describe \
    --topic "kafka.perf.low.topic1" \
    --bootstrap-server $BOOTSTRAP_SERVER

