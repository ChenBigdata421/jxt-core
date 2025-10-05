#!/bin/bash

# 优化的测试执行策略：
# 1. 快速测试（366个）运行10遍
# 2. 慢速测试（174个）运行1遍
# 3. 总执行时间预计：10-15分钟

set -e

echo "=========================================="
echo "EventBus 测试套件 - 优化执行策略"
echo "=========================================="
echo ""
echo "测试统计："
echo "  - 总测试数: 540"
echo "  - 快速测试: 366 (运行10遍)"
echo "  - 慢速测试: 174 (运行1遍)"
echo ""
echo "=========================================="
echo ""

# 创建结果目录
mkdir -p test_results_optimized

# ========== 阶段1：快速测试运行10遍 ==========
echo "=========================================="
echo "阶段1: 快速测试运行 10 遍"
echo "=========================================="
echo ""

# 跳过所有长时间运行的测试（包括健康检查配置、失败测试等）
SKIP_PATTERN="TestHealthCheck|TestProductionReadiness|TestKafka|TestNATS|Integration|TestBacklog|TestPublisher.*Backlog|TestConfig.*HealthCheck|TestFailure|TestRecovery|TestAlert"
FAST_PASSED=0
FAST_FAILED=0
TOTAL_FAST_RUNS=10

for i in $(seq 1 $TOTAL_FAST_RUNS); do
    echo "---------- 快速测试 第 $i 轮 ----------"

    START_TIME=$(date +%s)

    if timeout 120 go test -timeout 100s -skip="$SKIP_PATTERN" . > test_results_optimized/fast_run_${i}.log 2>&1; then
        END_TIME=$(date +%s)
        DURATION=$((END_TIME - START_TIME))
        
        PASS_COUNT=$(grep -c "^--- PASS:" test_results_optimized/fast_run_${i}.log 2>/dev/null || echo "0")
        FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_optimized/fast_run_${i}.log 2>/dev/null || echo "0")
        
        echo "✅ 第 $i 轮: PASS (${DURATION}s) - $PASS_COUNT 通过, $FAIL_COUNT 失败"
        FAST_PASSED=$((FAST_PASSED + 1))
    else
        END_TIME=$(date +%s)
        DURATION=$((END_TIME - START_TIME))
        
        PASS_COUNT=$(grep -c "^--- PASS:" test_results_optimized/fast_run_${i}.log 2>/dev/null || echo "0")
        FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_optimized/fast_run_${i}.log 2>/dev/null || echo "0")
        
        echo "❌ 第 $i 轮: FAIL (${DURATION}s) - $PASS_COUNT 通过, $FAIL_COUNT 失败"
        FAST_FAILED=$((FAST_FAILED + 1))
        
        # 显示失败的测试
        echo "   失败的测试:"
        grep "^--- FAIL:" test_results_optimized/fast_run_${i}.log 2>/dev/null | head -3 | sed 's/^/   /'
    fi
done

echo ""
echo "快速测试总结: $FAST_PASSED/$TOTAL_FAST_RUNS 通过"
echo ""

# ========== 阶段2：慢速测试运行1遍 ==========
echo "=========================================="
echo "阶段2: 慢速测试运行 1 遍"
echo "=========================================="
echo ""

# 慢速测试模式（包括所有被跳过的测试）
SLOW_PATTERN="TestHealthCheck|TestProductionReadiness|TestKafka|TestNATS|Integration|TestBacklog|TestPublisher.*Backlog|TestConfig.*HealthCheck|TestFailure|TestRecovery|TestAlert"

echo "---------- 慢速测试执行中 ----------"

START_TIME=$(date +%s)

if timeout 1800 go test -timeout 1700s -run="$SLOW_PATTERN" . > test_results_optimized/slow_run.log 2>&1; then
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    
    PASS_COUNT=$(grep -c "^--- PASS:" test_results_optimized/slow_run.log 2>/dev/null || echo "0")
    FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_optimized/slow_run.log 2>/dev/null || echo "0")
    
    echo "✅ 慢速测试: PASS (${DURATION}s) - $PASS_COUNT 通过, $FAIL_COUNT 失败"
    SLOW_RESULT="PASS"
else
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))
    
    PASS_COUNT=$(grep -c "^--- PASS:" test_results_optimized/slow_run.log 2>/dev/null || echo "0")
    FAIL_COUNT=$(grep -c "^--- FAIL:" test_results_optimized/slow_run.log 2>/dev/null || echo "0")
    
    echo "❌ 慢速测试: FAIL (${DURATION}s) - $PASS_COUNT 通过, $FAIL_COUNT 失败"
    SLOW_RESULT="FAIL"
    
    # 显示失败的测试
    echo "   失败的测试:"
    grep "^--- FAIL:" test_results_optimized/slow_run.log 2>/dev/null | head -5 | sed 's/^/   /'
fi

echo ""

# ========== 最终总结 ==========
echo "=========================================="
echo "最终总结"
echo "=========================================="
echo ""
echo "快速测试 (366个):"
echo "  - 运行次数: $TOTAL_FAST_RUNS"
echo "  - 通过次数: $FAST_PASSED"
echo "  - 失败次数: $FAST_FAILED"
echo "  - 成功率: $((FAST_PASSED * 100 / TOTAL_FAST_RUNS))%"
echo ""
echo "慢速测试 (174个):"
echo "  - 运行次数: 1"
echo "  - 结果: $SLOW_RESULT"
echo ""

# 计算总体统计
TOTAL_TEST_RUNS=$((TOTAL_FAST_RUNS + 1))
TOTAL_PASSED=$FAST_PASSED
if [ "$SLOW_RESULT" = "PASS" ]; then
    TOTAL_PASSED=$((TOTAL_PASSED + 1))
fi

echo "总体统计:"
echo "  - 总测试轮数: $TOTAL_TEST_RUNS (快速10轮 + 慢速1轮)"
echo "  - 通过轮数: $TOTAL_PASSED"
echo "  - 失败轮数: $((TOTAL_TEST_RUNS - TOTAL_PASSED))"
echo ""

# 生成覆盖率报告
echo "=========================================="
echo "生成覆盖率报告"
echo "=========================================="
echo ""

if timeout 120 go test -timeout 100s -coverprofile=test_results_optimized/coverage.out -covermode=atomic -skip="$SKIP_PATTERN" . > test_results_optimized/coverage_run.log 2>&1; then
    COVERAGE=$(go tool cover -func=test_results_optimized/coverage.out 2>/dev/null | grep "^total:" | awk '{print $3}')
    echo "✅ 覆盖率: $COVERAGE"
    
    # 生成HTML报告
    go tool cover -html=test_results_optimized/coverage.out -o test_results_optimized/coverage.html 2>/dev/null
    echo "✅ HTML报告: test_results_optimized/coverage.html"
else
    echo "⚠️  覆盖率生成失败或超时"
fi

echo ""
echo "=========================================="
echo "详细日志"
echo "=========================================="
echo "所有日志保存在 test_results_optimized/ 目录:"
echo "  - fast_run_1.log ~ fast_run_10.log: 快速测试10轮日志"
echo "  - slow_run.log: 慢速测试日志"
echo "  - coverage.out: 覆盖率数据"
echo "  - coverage.html: 覆盖率HTML报告"
echo ""

# 最终结果
if [ $FAST_FAILED -eq 0 ] && [ "$SLOW_RESULT" = "PASS" ]; then
    echo "🎉 所有测试都通过了！"
    exit 0
else
    echo "⚠️  部分测试失败，请查看日志"
    exit 1
fi

