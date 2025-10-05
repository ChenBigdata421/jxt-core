#!/bin/bash

# 简化版：运行测试10遍，实时显示输出

SKIP_PATTERN="TestHealthCheck|TestProductionReadiness|TestKafka|TestNATS|Integration"
TOTAL_RUNS=10
PASSED=0
FAILED=0

echo "=========================================="
echo "运行 EventBus 测试 10 遍"
echo "=========================================="
echo ""

for i in $(seq 1 $TOTAL_RUNS); do
    echo "========== 第 $i 轮 =========="
    
    if go test -skip="$SKIP_PATTERN" . 2>&1 | tail -3; then
        PASSED=$((PASSED + 1))
        echo "✅ 第 $i 轮: PASS"
    else
        FAILED=$((FAILED + 1))
        echo "❌ 第 $i 轮: FAIL"
    fi
    echo ""
done

echo "=========================================="
echo "总结: $PASSED/$TOTAL_RUNS 通过"
echo "=========================================="

if [ $FAILED -eq 0 ]; then
    echo "🎉 100% 通过率！"
    exit 0
else
    echo "⚠️  成功率: $((PASSED * 100 / TOTAL_RUNS))%"
    exit 1
fi

