#!/bin/bash

# 连续运行 20 次性能测试脚本

echo "=========================================="
echo "开始连续运行 20 次性能测试"
echo "测试文件: kafka_nats_comparison_test.go"
echo "开始时间: $(date)"
echo "=========================================="

# 创建结果目录
RESULT_DIR="test_results_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$RESULT_DIR"

# 统计变量
PASS_COUNT=0
FAIL_COUNT=0

# 运行 20 次测试
for i in {1..20}; do
    echo ""
    echo "=========================================="
    echo "第 $i 次测试 ($(date))"
    echo "=========================================="
    
    # 运行测试并保存结果
    LOG_FILE="$RESULT_DIR/test_run_${i}.log"
    
    if go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 30m 2>&1 | tee "$LOG_FILE"; then
        echo "✅ 第 $i 次测试: PASS"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        echo "❌ 第 $i 次测试: FAIL"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
    
    # 提取关键指标
    echo "提取关键指标..."
    grep -E "协程泄漏|顺序违反|成功率|吞吐量|延迟|内存" "$LOG_FILE" > "$RESULT_DIR/metrics_${i}.txt"
    
    # 等待一段时间，让系统清理资源
    if [ $i -lt 20 ]; then
        echo "等待 10 秒，清理资源..."
        sleep 10
    fi
done

echo ""
echo "=========================================="
echo "测试完成！"
echo "结束时间: $(date)"
echo "=========================================="
echo "总测试次数: 20"
echo "通过次数: $PASS_COUNT"
echo "失败次数: $FAIL_COUNT"
echo "成功率: $(echo "scale=2; $PASS_COUNT * 100 / 20" | bc)%"
echo ""
echo "结果保存在: $RESULT_DIR"
echo "=========================================="

# 生成汇总报告
echo "生成汇总报告..."
cat > "$RESULT_DIR/summary.txt" << EOF
性能测试 20 次运行汇总报告
========================================
测试时间: $(date)
测试文件: kafka_nats_comparison_test.go
总测试次数: 20
通过次数: $PASS_COUNT
失败次数: $FAIL_COUNT
成功率: $(echo "scale=2; $PASS_COUNT * 100 / 20" | bc)%
========================================

详细结果请查看各个日志文件。
EOF

echo "汇总报告已生成: $RESULT_DIR/summary.txt"

