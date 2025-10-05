#!/bin/bash

# 运行测试10遍，跳过长时间运行的测试
# 跳过：健康检查测试、生产就绪测试、Kafka集成测试、NATS集成测试

set -e

echo "=========================================="
echo "运行 EventBus 测试套件 10 遍"
echo "跳过：HealthCheck, ProductionReadiness, Kafka, NATS, Integration"
echo "=========================================="
echo ""

SKIP_PATTERN="TestHealthCheck|TestProductionReadiness|TestKafka|TestNATS|Integration"
TOTAL_RUNS=10
PASSED_RUNS=0
FAILED_RUNS=0

# 创建结果目录
mkdir -p test_results

for i in $(seq 1 $TOTAL_RUNS); do
    echo "=========================================="
    echo "第 $i 轮测试 (共 $TOTAL_RUNS 轮)"
    echo "=========================================="
    
    START_TIME=$(date +%s)
    
    # 运行测试
    if go test -skip="$SKIP_PATTERN" . > test_results/run_${i}.log 2>&1; then
        END_TIME=$(date +%s)
        DURATION=$((END_TIME - START_TIME))
        
        echo "✅ 第 $i 轮测试: PASS (${DURATION}s)"
        PASSED_RUNS=$((PASSED_RUNS + 1))
        
        # 显示执行时间
        tail -1 test_results/run_${i}.log
    else
        END_TIME=$(date +%s)
        DURATION=$((END_TIME - START_TIME))
        
        echo "❌ 第 $i 轮测试: FAIL (${DURATION}s)"
        FAILED_RUNS=$((FAILED_RUNS + 1))
        
        # 显示失败的测试
        echo "失败的测试:"
        grep "FAIL" test_results/run_${i}.log | head -10
    fi
    
    echo ""
done

echo "=========================================="
echo "测试总结"
echo "=========================================="
echo "总运行次数: $TOTAL_RUNS"
echo "通过次数: $PASSED_RUNS"
echo "失败次数: $FAILED_RUNS"
echo ""

if [ $FAILED_RUNS -eq 0 ]; then
    SUCCESS_RATE=100
    echo "🎉 所有测试都通过了！100% 成功率！"
    echo ""
    echo "详细日志保存在 test_results/ 目录中"
    exit 0
else
    SUCCESS_RATE=$((PASSED_RUNS * 100 / TOTAL_RUNS))
    echo "⚠️  成功率: $SUCCESS_RATE%"
    echo ""
    echo "失败的测试详情请查看 test_results/run_*.log 文件"
    exit 1
fi

