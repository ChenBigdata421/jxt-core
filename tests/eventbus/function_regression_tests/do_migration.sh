#!/bin/bash

# 实际可迁移的测试文件列表
# 这些文件不使用内部类型，可以安全迁移

set -e

SOURCE_DIR="../../../sdk/pkg/eventbus"
TARGET_DIR="."

# 实际可迁移的文件（经过验证）
FILES=(
    "e2e_integration_test.go"
    "envelope_advanced_test.go"
    "health_check_comprehensive_test.go"
    "health_check_config_test.go"
    "health_check_simple_test.go"
    "backlog_detector_test.go"
    "topic_config_manager_test.go"
)

echo "========================================="
echo "EventBus 测试迁移脚本"
echo "========================================="
echo ""

migrate_file() {
    local file=$1
    local source="$SOURCE_DIR/$file"
    local target="$TARGET_DIR/$file"
    
    echo "处理: $file"
    
    # 检查源文件是否存在
    if [ ! -f "$source" ]; then
        echo "  ✗ 源文件不存在，跳过"
        return 1
    fi
    
    # 检查目标文件是否已存在
    if [ -f "$target" ] && [ "$file" != "e2e_integration_test.go" ]; then
        echo "  ⚠ 目标文件已存在，跳过"
        return 1
    fi
    
    # 复制文件
    cp "$source" "$target"
    echo "  ✓ 文件已复制"
    
    # 修改包名
    sed -i 's/^package eventbus$/package function_tests/' "$target"
    echo "  ✓ 包名已修改"
    
    # 添加导入
    if ! grep -q 'github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus' "$target"; then
        sed -i '/^import ($/a\	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"' "$target"
        echo "  ✓ 导入已添加"
    fi
    
    # 替换类型引用
    sed -i 's/\bEventBusConfig\b/eventbus.EventBusConfig/g' "$target"
    sed -i 's/\bEventBusType/eventbus.EventBusType/g' "$target"
    sed -i 's/\bEventBus\b/eventbus.EventBus/g' "$target"
    sed -i 's/\bEnvelope\b/eventbus.Envelope/g' "$target"
    sed -i 's/\bMessageHandler\b/eventbus.MessageHandler/g' "$target"
    sed -i 's/\bEnvelopeHandler\b/eventbus.EnvelopeHandler/g' "$target"
    sed -i 's/\bHealthStatus\b/eventbus.HealthStatus/g' "$target"
    sed -i 's/\bMetrics\b/eventbus.Metrics/g' "$target"
    sed -i 's/\bNewEventBus\b/eventbus.NewEventBus/g' "$target"
    sed -i 's/\bNewEnvelope\b/eventbus.NewEnvelope/g' "$target"
    sed -i 's/\bTopicConfig\b/eventbus.TopicConfig/g' "$target"
    sed -i 's/\bBacklogDetector\b/eventbus.BacklogDetector/g' "$target"
    sed -i 's/\bNewBacklogDetector\b/eventbus.NewBacklogDetector/g' "$target"
    
    # 避免重复前缀
    sed -i 's/eventbus\.eventbus\./eventbus./g' "$target"
    
    echo "  ✓ 类型引用已更新"
    echo "  ✓ 迁移完成"
    echo ""
    
    return 0
}

# 执行迁移
SUCCESS=0
SKIP=0

for file in "${FILES[@]}"; do
    if migrate_file "$file"; then
        SUCCESS=$((SUCCESS + 1))
    else
        SKIP=$((SKIP + 1))
    fi
done

echo "========================================="
echo "迁移统计"
echo "========================================="
echo "成功迁移: $SUCCESS"
echo "跳过文件: $SKIP"
echo ""

if [ $SUCCESS -gt 0 ]; then
    echo "运行测试验证..."
    go test -v -run "TestE2E|TestEnvelope|TestHealth|TestBacklog|TestTopic" -timeout 2m
fi

