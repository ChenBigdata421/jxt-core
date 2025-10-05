#!/bin/bash

# 最终优化策略：
# 1. 新增测试（快速）运行10遍 - 验证稳定性
# 2. 所有测试运行1遍 - 完整验证

set -e

echo "=========================================="
echo "EventBus 测试套件 - 最终执行策略"
echo "=========================================="
echo ""
echo "策略说明："
echo "  1. 新增测试（30个）运行 10 遍 - 验证新代码稳定性"
echo "  2. 所有测试（540个）运行 1 遍 - 完整回归测试"
echo "  3. 预计总时间: 5-10 分钟"
echo ""
echo "=========================================="
echo ""

# 创建结果目录
mkdir -p test_results_final
rm -f test_results_final/*.log 2>/dev/null || true

# ========== 阶段1：新增测试运行10遍 ==========
echo "=========================================="
echo "阶段1: 新增测试运行 10 遍"
echo "=========================================="
echo ""

# 新增测试模式
NEW_TESTS="TestEventBusManager_UpdateMetrics|TestEventBusManager_Metrics|Advanced|WrappedHandler"

NEW_PASSED=0
NEW_FAILED=0
TOTAL_NEW_RUNS=10

for i in $(seq 1 $TOTAL_NEW_RUNS); do
    echo "---------- 新增测试 第 $i 轮 ----------"
    
    START_TIME=$(date +%s)
    
    if go test -timeout 30s -run="$NEW_TESTS" . > test_results_final/new_run_${i}.log 2>&1; then
        END_TIME=$(date +%s)
        DURATION=$((END_TIME - START_TIME))
        
        PASS_COUNT=$(grep -c "^--- PASS:" test_results_final/new_run_${i}.log 2>/dev/null || echo "0")
        FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_final/new_run_${i}.log 2>/dev/null || echo "0")
        
        echo "✅ 第 $i 轮: PASS (${DURATION}s) - $PASS_COUNT 通过, $FAIL_COUNT 失败"
        NEW_PASSED=$((NEW_PASSED + 1))
    else
        END_TIME=$(date +%s)
        DURATION=$((END_TIME - START_TIME))
        
        PASS_COUNT=$(grep -c "^--- PASS:" test_results_final/new_run_${i}.log 2>/dev/null || echo "0")
        FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_final/new_run_${i}.log 2>/dev/null || echo "0")
        
        echo "❌ 第 $i 轮: FAIL (${DURATION}s) - $PASS_COUNT 通过, $FAIL_COUNT 失败"
        NEW_FAILED=$((NEW_FAILED + 1))
        
        # 显示失败的测试
        echo "   失败的测试:"
        grep "^--- FAIL:" test_results_final/new_run_${i}.log 2>/dev/null | head -3 | sed 's/^/   /'
    fi
done

echo ""
echo "新增测试总结: $NEW_PASSED/$TOTAL_NEW_RUNS 通过 (成功率: $((NEW_PASSED * 100 / TOTAL_NEW_RUNS))%)"
echo ""

# ========== 阶段2：所有测试运行1遍（分批执行） ==========
echo "=========================================="
echo "阶段2: 所有测试运行 1 遍（分批执行）"
echo "=========================================="
echo ""

# 批次1：快速测试（跳过慢速测试）
echo "---------- 批次1: 快速测试 ----------"
SKIP_SLOW="TestHealthCheck|TestProductionReadiness|TestKafka|TestNATS|Integration|TestBacklog|TestPublisher.*Backlog|TestConfig.*HealthCheck|TestFailure|TestRecovery|TestAlert"

START_TIME=$(date +%s)

if timeout 300 go test -timeout 250s -skip="$SKIP_SLOW" . > test_results_final/batch1_fast.log 2>&1; then
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    
    PASS_COUNT=$(grep -c "^--- PASS:" test_results_final/batch1_fast.log 2>/dev/null || echo "0")
    FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_final/batch1_fast.log 2>/dev/null || echo "0")
    
    echo "✅ 批次1: PASS (${DURATION}s) - $PASS_COUNT 通过, $FAIL_COUNT 失败"
    BATCH1_RESULT="PASS"
else
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    
    PASS_COUNT=$(grep -c "^--- PASS:" test_results_final/batch1_fast.log 2>/dev/null || echo "0")
    FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_final/batch1_fast.log 2>/dev/null || echo "0")
    
    echo "❌ 批次1: FAIL/TIMEOUT (${DURATION}s) - $PASS_COUNT 通过, $FAIL_COUNT 失败"
    BATCH1_RESULT="FAIL"
    
    # 显示失败的测试
    echo "   失败的测试:"
    grep "^--- FAIL:" test_results_final/batch1_fast.log 2>/dev/null | head -5 | sed 's/^/   /'
fi

echo ""

# 批次2：慢速测试（仅运行慢速测试）
echo "---------- 批次2: 慢速测试 ----------"
RUN_SLOW="TestHealthCheck|TestProductionReadiness|TestKafka|TestNATS|Integration|TestBacklog|TestPublisher.*Backlog|TestConfig.*HealthCheck|TestFailure|TestRecovery|TestAlert"

START_TIME=$(date +%s)

if timeout 1800 go test -timeout 1700s -run="$RUN_SLOW" . > test_results_final/batch2_slow.log 2>&1; then
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    
    PASS_COUNT=$(grep -c "^--- PASS:" test_results_final/batch2_slow.log 2>/dev/null || echo "0")
    FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_final/batch2_slow.log 2>/dev/null || echo "0")
    
    echo "✅ 批次2: PASS (${DURATION}s) - $PASS_COUNT 通过, $FAIL_COUNT 失败"
    BATCH2_RESULT="PASS"
else
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    
    PASS_COUNT=$(grep -c "^--- PASS:" test_results_final/batch2_slow.log 2>/dev/null || echo "0")
    FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_final/batch2_slow.log 2>/dev/null || echo "0")
    
    echo "❌ 批次2: FAIL/TIMEOUT (${DURATION}s) - $PASS_COUNT 通过, $FAIL_COUNT 失败"
    BATCH2_RESULT="FAIL"
    
    # 显示失败的测试
    echo "   失败的测试:"
    grep "^--- FAIL:" test_results_final/batch2_slow.log 2>/dev/null | head -5 | sed 's/^/   /'
fi

echo ""

# ========== 阶段3：生成覆盖率报告 ==========
echo "=========================================="
echo "阶段3: 生成覆盖率报告"
echo "=========================================="
echo ""

if timeout 300 go test -timeout 250s -coverprofile=test_results_final/coverage.out -covermode=atomic -skip="$SKIP_SLOW" . > test_results_final/coverage_run.log 2>&1; then
    COVERAGE=$(go tool cover -func=test_results_final/coverage.out 2>/dev/null | grep "^total:" | awk '{print $3}')
    echo "✅ 覆盖率: $COVERAGE"
    
    # 生成HTML报告
    go tool cover -html=test_results_final/coverage.out -o test_results_final/coverage.html 2>/dev/null
    echo "✅ HTML报告: test_results_final/coverage.html"
    
    # 显示热路径方法覆盖率
    echo ""
    echo "热路径方法覆盖率:"
    go tool cover -func=test_results_final/coverage.out 2>/dev/null | grep -E "(Publish|Subscribe|updateMetrics|checkConnection|checkMessageTransport)" | grep -v "100.0%" | head -10 | sed 's/^/  /'
else
    echo "⚠️  覆盖率生成失败或超时"
fi

echo ""

# ========== 最终总结 ==========
echo "=========================================="
echo "最终总结"
echo "=========================================="
echo ""
echo "新增测试 (30个):"
echo "  - 运行次数: $TOTAL_NEW_RUNS"
echo "  - 通过次数: $NEW_PASSED"
echo "  - 失败次数: $NEW_FAILED"
echo "  - 成功率: $((NEW_PASSED * 100 / TOTAL_NEW_RUNS))%"
echo ""
echo "所有测试 (540个):"
echo "  - 批次1 (快速测试): $BATCH1_RESULT"
echo "  - 批次2 (慢速测试): $BATCH2_RESULT"
echo ""

# 计算总体结果
TOTAL_SUCCESS=0
if [ $NEW_PASSED -eq $TOTAL_NEW_RUNS ]; then
    TOTAL_SUCCESS=$((TOTAL_SUCCESS + 1))
fi
if [ "$BATCH1_RESULT" = "PASS" ]; then
    TOTAL_SUCCESS=$((TOTAL_SUCCESS + 1))
fi
if [ "$BATCH2_RESULT" = "PASS" ]; then
    TOTAL_SUCCESS=$((TOTAL_SUCCESS + 1))
fi

echo "总体结果: $TOTAL_SUCCESS/3 阶段通过"
echo ""
echo "详细日志保存在 test_results_final/ 目录:"
echo "  - new_run_1.log ~ new_run_10.log: 新增测试10轮日志"
echo "  - batch1_fast.log: 快速测试日志"
echo "  - batch2_slow.log: 慢速测试日志"
echo "  - coverage.out: 覆盖率数据"
echo "  - coverage.html: 覆盖率HTML报告"
echo ""

# 最终结果
if [ $TOTAL_SUCCESS -eq 3 ]; then
    echo "🎉 所有测试阶段都通过了！"
    exit 0
else
    echo "⚠️  部分测试阶段失败，请查看日志"
    exit 1
fi

