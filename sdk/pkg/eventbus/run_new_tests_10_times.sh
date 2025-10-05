#!/bin/bash

# 运行新增的测试10遍（这些测试运行很快）
# 新增测试：Metrics, HotPath Advanced, WrappedHandler, EdgeCases, Performance

set -e

echo "=========================================="
echo "运行新增测试 10 遍"
echo "=========================================="
echo ""

# 新增测试的模式
NEW_TESTS="TestEventBusManager_UpdateMetrics|TestEventBusManager_Metrics|Advanced|WrappedHandler|TestEventBusManager_Publish_Closed|TestEventBusManager_Subscribe_Closed|TestEventBusManager_EmptyMessage|TestEventBusManager_ConcurrentPublish"

TOTAL_RUNS=10
PASSED_RUNS=0
FAILED_RUNS=0

# 创建结果目录
mkdir -p test_results_new

for i in $(seq 1 $TOTAL_RUNS); do
    echo "========== 第 $i 轮测试 =========="
    
    START_TIME=$(date +%s)
    
    # 运行新增测试
    if go test -v -run "$NEW_TESTS" . > test_results_new/run_${i}.log 2>&1; then
        END_TIME=$(date +%s)
        DURATION=$((END_TIME - START_TIME))
        
        # 统计测试数量
        PASS_COUNT=$(grep -c "^--- PASS:" test_results_new/run_${i}.log || echo "0")
        FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_new/run_${i}.log || echo "0")
        
        echo "✅ 第 $i 轮: PASS (${DURATION}s) - $PASS_COUNT 个测试通过, $FAIL_COUNT 个测试失败"
        PASSED_RUNS=$((PASSED_RUNS + 1))
    else
        END_TIME=$(date +%s)
        DURATION=$((END_TIME - START_TIME))
        
        # 统计测试数量
        PASS_COUNT=$(grep -c "^--- PASS:" test_results_new/run_${i}.log || echo "0")
        FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_new/run_${i}.log || echo "0")
        
        echo "❌ 第 $i 轮: FAIL (${DURATION}s) - $PASS_COUNT 个测试通过, $FAIL_COUNT 个测试失败"
        FAILED_RUNS=$((FAILED_RUNS + 1))
        
        # 显示失败的测试
        echo "失败的测试:"
        grep "^--- FAIL:" test_results_new/run_${i}.log | head -5
    fi
    
    echo ""
done

echo "=========================================="
echo "新增测试总结"
echo "=========================================="
echo "总运行次数: $TOTAL_RUNS"
echo "通过次数: $PASSED_RUNS"
echo "失败次数: $FAILED_RUNS"
echo ""

if [ $FAILED_RUNS -eq 0 ]; then
    SUCCESS_RATE=100
    echo "🎉 所有新增测试都通过了！100% 成功率！"
else
    SUCCESS_RATE=$((PASSED_RUNS * 100 / TOTAL_RUNS))
    echo "⚠️  成功率: $SUCCESS_RATE%"
fi

echo ""
echo "=========================================="
echo "运行完整测试套件 1 遍（包括长时间运行的测试）"
echo "=========================================="
echo ""

START_TIME=$(date +%s)

if timeout 1200 go test -v . > test_results_new/full_test.log 2>&1; then
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    
    PASS_COUNT=$(grep -c "^--- PASS:" test_results_new/full_test.log || echo "0")
    FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_new/full_test.log || echo "0")
    
    echo "✅ 完整测试: PASS (${DURATION}s)"
    echo "   通过: $PASS_COUNT 个测试"
    echo "   失败: $FAIL_COUNT 个测试"
else
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    
    PASS_COUNT=$(grep -c "^--- PASS:" test_results_new/full_test.log || echo "0")
    FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_new/full_test.log || echo "0")
    
    echo "⚠️  完整测试: 超时或失败 (${DURATION}s)"
    echo "   通过: $PASS_COUNT 个测试"
    echo "   失败: $FAIL_COUNT 个测试"
    
    # 显示失败的测试
    echo ""
    echo "失败的测试:"
    grep "^--- FAIL:" test_results_new/full_test.log | head -10
fi

echo ""
echo "=========================================="
echo "最终总结"
echo "=========================================="
echo "新增测试 10 轮成功率: $SUCCESS_RATE%"
echo "完整测试执行: 已完成"
echo ""
echo "详细日志保存在 test_results_new/ 目录中"
echo "  - run_1.log ~ run_10.log: 新增测试的10轮执行日志"
echo "  - full_test.log: 完整测试套件执行日志"
echo ""

if [ $FAILED_RUNS -eq 0 ]; then
    echo "🎉 新增测试 100% 通过！"
    exit 0
else
    echo "⚠️  部分测试失败，请查看日志"
    exit 1
fi

