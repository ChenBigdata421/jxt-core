#!/bin/bash

# 验证测试文件是否可以编译

set -e

echo "🔍 验证测试文件..."
echo ""

# 进入测试目录
cd "$(dirname "$0")"

# 编译测试文件
echo "📦 编译测试文件..."
if go build kafka_nats_comparison_test.go; then
    echo "✅ 测试文件编译成功！"
    
    # 清理编译产物
    if [ -f "kafka_nats_comparison_test" ]; then
        rm kafka_nats_comparison_test
    fi
    if [ -f "kafka_nats_comparison_test.exe" ]; then
        rm kafka_nats_comparison_test.exe
    fi
    
    echo ""
    echo "🎉 验证完成！测试文件可以正常编译。"
    echo ""
    echo "📝 下一步："
    echo "   1. 启动 Kafka: docker-compose up -d kafka"
    echo "   2. 启动 NATS: docker-compose up -d nats"
    echo "   3. 运行测试: ./run_comparison_test.sh"
    echo ""
    
    exit 0
else
    echo "❌ 测试文件编译失败！"
    exit 1
fi

