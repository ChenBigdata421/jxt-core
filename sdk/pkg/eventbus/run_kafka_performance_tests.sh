#!/bin/bash

# 🚀 Kafka UnifiedConsumer 性能测试运行脚本
# 用于执行各种性能基准测试和验证测试

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
    local message=$2
    echo -e "${color}${message}${NC}"
}

# 检查Kafka是否运行
check_kafka() {
    print_message $BLUE "🔍 检查Kafka服务状态..."
    
    # 检查Kafka端口是否开放
    if ! nc -z localhost 9092 2>/dev/null; then
        print_message $RED "❌ Kafka服务未运行！请先启动Kafka。"
        print_message $YELLOW "💡 启动命令示例："
        echo "   docker run -d --name kafka -p 9092:9092 \\"
        echo "     -e KAFKA_ZOOKEEPER_CONNECT=zookeeper:2181 \\"
        echo "     -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092 \\"
        echo "     -e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \\"
        echo "     confluentinc/cp-kafka:latest"
        exit 1
    fi
    
    print_message $GREEN "✅ Kafka服务正在运行"
}

# 检查Go环境
check_go() {
    print_message $BLUE "🔍 检查Go环境..."
    
    if ! command -v go &> /dev/null; then
        print_message $RED "❌ Go未安装或不在PATH中"
        exit 1
    fi
    
    GO_VERSION=$(go version | awk '{print $3}')
    print_message $GREEN "✅ Go环境正常: $GO_VERSION"
}

# 清理测试topic
cleanup_topics() {
    print_message $BLUE "🧹 清理测试topic..."
    
    # 这里可以添加清理Kafka topic的命令
    # 例如使用kafka-topics.sh删除测试topic
    print_message $YELLOW "⚠️  请手动清理测试topic（如果需要）"
}

# 运行快速验证测试
run_quick_test() {
    print_message $CYAN "🚀 运行快速验证测试..."
    print_message $YELLOW "📊 测试配置: 10 topics, 100 messages/topic, 20 aggregates"
    
    go test -v -run TestKafkaUnifiedConsumerQuickValidation -timeout 120s
    
    if [ $? -eq 0 ]; then
        print_message $GREEN "✅ 快速验证测试通过"
    else
        print_message $RED "❌ 快速验证测试失败"
        return 1
    fi
}

# 运行性能基准测试
run_benchmark_test() {
    print_message $CYAN "🎯 运行性能基准测试..."
    print_message $YELLOW "📊 测试配置: 10 topics, 1000 messages/topic, 100 aggregates, 30s duration"
    
    go test -v -run TestBenchmarkKafkaUnifiedConsumerPerformance -timeout 300s
    
    if [ $? -eq 0 ]; then
        print_message $GREEN "✅ 性能基准测试完成"
    else
        print_message $RED "❌ 性能基准测试失败"
        return 1
    fi
}

# 运行压力测试
run_stress_test() {
    print_message $CYAN "🔥 运行压力测试..."
    print_message $YELLOW "📊 测试配置: 10 topics, 5 producers, 20 consumers, 5000 msg/s, 60s duration"
    print_message $YELLOW "⚠️  这是一个高强度测试，请确保系统资源充足"
    
    read -p "是否继续运行压力测试？(y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_message $YELLOW "⏭️  跳过压力测试"
        return 0
    fi
    
    go test -v -run TestKafkaUnifiedConsumerStressTest -timeout 600s
    
    if [ $? -eq 0 ]; then
        print_message $GREEN "✅ 压力测试完成"
    else
        print_message $RED "❌ 压力测试失败"
        return 1
    fi
}

# 运行所有测试
run_all_tests() {
    print_message $PURPLE "🎯 运行所有Kafka UnifiedConsumer性能测试"
    
    run_quick_test
    echo
    
    run_benchmark_test
    echo
    
    run_stress_test
    echo
    
    print_message $GREEN "🎉 所有测试完成！"
}

# 显示帮助信息
show_help() {
    echo "Kafka UnifiedConsumer 性能测试运行脚本"
    echo
    echo "用法: $0 [选项]"
    echo
    echo "选项:"
    echo "  quick      运行快速验证测试"
    echo "  benchmark  运行性能基准测试"
    echo "  stress     运行压力测试"
    echo "  all        运行所有测试"
    echo "  clean      清理测试topic"
    echo "  help       显示此帮助信息"
    echo
    echo "示例:"
    echo "  $0 quick      # 运行快速验证"
    echo "  $0 benchmark  # 运行性能基准测试"
    echo "  $0 all        # 运行所有测试"
}

# 主函数
main() {
    print_message $PURPLE "🚀 Kafka UnifiedConsumer 性能测试套件"
    print_message $PURPLE "=========================================="
    
    # 检查环境
    check_go
    check_kafka
    echo
    
    # 根据参数执行相应操作
    case "${1:-help}" in
        "quick")
            run_quick_test
            ;;
        "benchmark")
            run_benchmark_test
            ;;
        "stress")
            run_stress_test
            ;;
        "all")
            run_all_tests
            ;;
        "clean")
            cleanup_topics
            ;;
        "help"|*)
            show_help
            ;;
    esac
}

# 执行主函数
main "$@"
