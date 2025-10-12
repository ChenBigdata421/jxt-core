#!/bin/bash

# 🎯 NATS & Kafka 基准测试环境验证脚本

echo "🔍 Verifying NATS & Kafka Benchmark Environment..."

# 检查Docker容器状态
echo ""
echo "📦 Checking Docker containers..."
docker ps | grep benchmark

echo ""
echo "🔵 Checking NATS JetStream..."
# 检查NATS健康状态
NATS_HEALTH=$(curl -s http://localhost:8223/healthz 2>/dev/null)
if [ $? -eq 0 ]; then
    echo "✅ NATS JetStream is healthy"
    echo "🔗 NATS URL: nats://localhost:4223"
    echo "📊 NATS Monitoring: http://localhost:8223"
else
    echo "❌ NATS JetStream is not accessible"
fi

echo ""
echo "🟠 Checking Kafka..."
# 检查Kafka状态
KAFKA_STATUS=$(docker exec benchmark-kafka kafka-topics.sh --bootstrap-server localhost:9092 --list 2>/dev/null)
if [ $? -eq 0 ]; then
    echo "✅ Kafka is healthy"
    echo "🔗 Kafka URL: localhost:29093"
    echo "📋 Available topics:"
    echo "$KAFKA_STATUS"
else
    echo "❌ Kafka is not accessible or still starting"
    echo "📋 Checking Kafka logs..."
    docker logs benchmark-kafka --tail 5
fi

echo ""
echo "🌐 Checking network connectivity..."
# 检查网络
NETWORK_INFO=$(docker network ls | grep benchmark)
if [ ! -z "$NETWORK_INFO" ]; then
    echo "✅ Benchmark network exists"
    echo "$NETWORK_INFO"
else
    echo "❌ Benchmark network not found"
fi

echo ""
echo "💾 Checking volumes..."
# 检查数据卷
VOLUME_INFO=$(docker volume ls | grep benchmark)
if [ ! -z "$VOLUME_INFO" ]; then
    echo "✅ Benchmark volumes exist"
    echo "$VOLUME_INFO"
else
    echo "❌ Benchmark volumes not found"
fi

echo ""
echo "🎯 Environment Summary:"
echo "- NATS JetStream: localhost:4223"
echo "- NATS Monitoring: http://localhost:8223"
echo "- Kafka: localhost:29093"
echo "- Network: benchmark-network"

echo ""
echo "🚀 Ready to run performance benchmarks!"
echo ""
echo "📋 Available test commands:"
echo "  go test -v -run TestNATSSimpleBenchmark ./sdk/pkg/eventbus/"
echo "  go test -v -run TestFairComparisonNATSvsKafka ./sdk/pkg/eventbus/"
echo ""
