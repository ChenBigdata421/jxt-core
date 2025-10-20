#!/bin/bash

# Kafka vs NATS JetStream 性能对比测试运行脚本

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_message() {
    local color=$1
    shift
    echo -e "${color}$@${NC}"
}

# 打印标题
print_header() {
    echo ""
    print_message $PURPLE "========================================"
    print_message $PURPLE "$1"
    print_message $PURPLE "========================================"
    echo ""
}

# 检查服务是否运行
check_service() {
    local service=$1
    local port=$2
    
    if nc -z localhost $port 2>/dev/null; then
        print_message $GREEN "✅ $service is running on port $port"
        return 0
    else
        print_message $RED "❌ $service is NOT running on port $port"
        return 1
    fi
}

# 主函数
main() {
    print_header "🚀 Kafka vs NATS JetStream 性能对比测试"
    
    # 检查前置条件
    print_message $CYAN "📋 检查前置条件..."
    
    # 检查 Kafka
    if ! check_service "Kafka" 29092; then
        print_message $YELLOW "⚠️  请先启动 Kafka: docker-compose up -d kafka"
        exit 1
    fi
    
    # 检查 NATS
    if ! check_service "NATS" 4223; then
        print_message $YELLOW "⚠️  请先启动 NATS: docker-compose up -d nats"
        exit 1
    fi
    
    print_message $GREEN "✅ 所有服务已就绪"
    echo ""
    
    # 显示测试信息
    print_message $CYAN "📊 测试配置:"
    print_message $CYAN "   • 低压测试: 500 条消息"
    print_message $CYAN "   • 中压测试: 2000 条消息"
    print_message $CYAN "   • 高压测试: 5000 条消息"
    print_message $CYAN "   • 极限测试: 10000 条消息"
    echo ""
    print_message $CYAN "   • 发布方法: PublishEnvelope"
    print_message $CYAN "   • 订阅方法: SubscribeEnvelope"
    print_message $CYAN "   • NATS 持久化: 磁盘 (file)"
    print_message $CYAN "   • Kafka 持久化: 启用 (RequiredAcks=-1)"
    echo ""
    
    # 询问是否继续
    print_message $YELLOW "⏱️  预计测试时间: 15-20 分钟"
    read -p "是否继续运行测试？(y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_message $YELLOW "⏭️  测试已取消"
        exit 0
    fi
    
    # 运行测试
    print_header "🧪 开始运行性能对比测试"
    
    # 创建日志目录
    LOG_DIR="test_results"
    mkdir -p $LOG_DIR
    
    # 生成日志文件名
    TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
    LOG_FILE="$LOG_DIR/comparison_test_$TIMESTAMP.log"
    
    print_message $CYAN "📝 测试日志将保存到: $LOG_FILE"
    echo ""
    
    # 运行测试并保存日志
    if go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 30m 2>&1 | tee $LOG_FILE; then
        print_header "✅ 测试完成"
        print_message $GREEN "📊 测试报告已保存到: $LOG_FILE"
        
        # 提取关键指标
        print_message $CYAN "\n📈 快速摘要:"
        grep -E "🎉 总体获胜者:|平均吞吐量|平均延迟|平均内存增量" $LOG_FILE || true
        
    else
        print_header "❌ 测试失败"
        print_message $RED "请查看日志文件了解详情: $LOG_FILE"
        exit 1
    fi
    
    echo ""
    print_message $PURPLE "🎉 感谢使用 Kafka vs NATS 性能对比测试！"
}

# 运行主函数
main "$@"

