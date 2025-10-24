#!/bin/bash

# 多租户 ACK Channel 测试运行脚本

set -e

echo "=========================================="
echo "多租户 ACK Channel 测试套件"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 测试结果统计
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
SKIPPED_TESTS=0

# 运行单个测试
run_test() {
    local test_name=$1
    local test_pattern=$2
    local requires_service=$3
    
    echo "----------------------------------------"
    echo "测试: $test_name"
    echo "----------------------------------------"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    # 如果需要外部服务，检查是否可用
    if [ -n "$requires_service" ]; then
        echo "检查 $requires_service 服务..."
        if [ "$requires_service" == "NATS" ]; then
            if ! nc -z localhost 4223 2>/dev/null; then
                echo -e "${YELLOW}⚠️  NATS 服务未运行，跳过测试${NC}"
                echo "提示: 运行 'docker-compose up -d nats' 启动 NATS"
                SKIPPED_TESTS=$((SKIPPED_TESTS + 1))
                echo ""
                return
            fi
        elif [ "$requires_service" == "Kafka" ]; then
            if ! nc -z localhost 29094 2>/dev/null; then
                echo -e "${YELLOW}⚠️  Kafka 服务未运行，跳过测试${NC}"
                echo "提示: 运行 'docker-compose up -d kafka' 启动 Kafka"
                SKIPPED_TESTS=$((SKIPPED_TESTS + 1))
                echo ""
                return
            fi
        fi
    fi
    
    # 运行测试
    if go test -v -run "$test_pattern" -timeout 60s 2>&1 | tee /tmp/test_output.log; then
        if grep -q "PASS" /tmp/test_output.log; then
            echo -e "${GREEN}✅ 测试通过${NC}"
            PASSED_TESTS=$((PASSED_TESTS + 1))
        else
            echo -e "${RED}❌ 测试失败${NC}"
            FAILED_TESTS=$((FAILED_TESTS + 1))
        fi
    else
        echo -e "${RED}❌ 测试失败${NC}"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    
    echo ""
}

# 切换到测试目录
cd "$(dirname "$0")"

echo "当前目录: $(pwd)"
echo ""

# 1. 运行 Memory EventBus 路由测试（不需要外部服务）
run_test "Memory EventBus - ACK 路由测试" "^TestMultiTenantACKRouting$" ""

# 2. 运行 NATS EventBus 测试（需要 NATS 服务）
run_test "NATS EventBus - 基础多租户 ACK 测试" "^TestMultiTenantACKChannel_NATS$" "NATS"
run_test "NATS EventBus - 租户隔离性测试" "^TestMultiTenantACKChannel_NATS_Isolation$" "NATS"

# 3. 运行 Kafka EventBus 测试（需要 Kafka 服务）
run_test "Kafka EventBus - 基础多租户 ACK 测试" "^TestMultiTenantACKChannel_Kafka$" "Kafka"
run_test "Kafka EventBus - 并发发布测试" "^TestMultiTenantACKChannel_Kafka_ConcurrentPublish$" "Kafka"

# 打印测试结果摘要
echo "=========================================="
echo "测试结果摘要"
echo "=========================================="
echo "总测试数: $TOTAL_TESTS"
echo -e "${GREEN}通过: $PASSED_TESTS${NC}"
echo -e "${RED}失败: $FAILED_TESTS${NC}"
echo -e "${YELLOW}跳过: $SKIPPED_TESTS${NC}"
echo "=========================================="

# 根据测试结果返回退出码
if [ $FAILED_TESTS -gt 0 ]; then
    exit 1
else
    exit 0
fi

