#!/bin/bash

# EventBus 测试迁移脚本
# 用途: 将测试文件从 sdk/pkg/eventbus/ 迁移到 tests/eventbus/function_tests/
# 作者: Augment Agent
# 日期: 2025-10-14

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 目录定义
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
SOURCE_DIR="$PROJECT_ROOT/sdk/pkg/eventbus"
TARGET_DIR="$PROJECT_ROOT/tests/eventbus/function_tests"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}EventBus 测试迁移脚本${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "项目根目录: ${GREEN}$PROJECT_ROOT${NC}"
echo -e "源目录: ${GREEN}$SOURCE_DIR${NC}"
echo -e "目标目录: ${GREEN}$TARGET_DIR${NC}"
echo ""

# P0 测试文件列表
declare -a P0_FILES=(
    "eventbus_core_test.go"
    "eventbus_types_test.go"
    "factory_test.go"
    "envelope_advanced_test.go"
    "message_formatter_test.go"
    "backlog_detector_test.go"
    "publisher_backlog_detector_test.go"
    "topic_config_manager_test.go"
    "health_check_comprehensive_test.go"
    "health_check_config_test.go"
    "health_check_simple_test.go"
    "health_check_deadlock_test.go"
    "e2e_integration_test.go"
    "eventbus_integration_test.go"
    "health_check_failure_test.go"
)

# P1 测试文件列表
declare -a P1_FILES=(
    "json_config_test.go"
    "config_conversion_test.go"
    "extract_aggregate_id_test.go"
    "naming_verification_test.go"
    "health_checker_advanced_test.go"
    "health_check_message_coverage_test.go"
    "health_check_message_extended_test.go"
    "health_check_subscriber_extended_test.go"
    "eventbus_health_check_coverage_test.go"
    "topic_config_manager_coverage_test.go"
    "eventbus_topic_config_coverage_test.go"
    "keyed_worker_pool_test.go"
    "unified_worker_pool_test.go"
    "rate_limiter_test.go"
    "dynamic_subscription_test.go"
)

# 统计变量
TOTAL_FILES=0
SUCCESS_COUNT=0
SKIP_COUNT=0
FAIL_COUNT=0

# 迁移单个文件
migrate_file() {
    local file=$1
    local source_file="$SOURCE_DIR/$file"
    local target_file="$TARGET_DIR/$file"
    
    TOTAL_FILES=$((TOTAL_FILES + 1))
    
    echo -e "${YELLOW}[$TOTAL_FILES] 处理: $file${NC}"
    
    # 检查源文件是否存在
    if [ ! -f "$source_file" ]; then
        echo -e "  ${RED}✗ 源文件不存在，跳过${NC}"
        SKIP_COUNT=$((SKIP_COUNT + 1))
        return 1
    fi
    
    # 检查目标文件是否已存在
    if [ -f "$target_file" ]; then
        echo -e "  ${YELLOW}⚠ 目标文件已存在，跳过${NC}"
        SKIP_COUNT=$((SKIP_COUNT + 1))
        return 1
    fi
    
    # 复制文件
    cp "$source_file" "$target_file"
    echo -e "  ${GREEN}✓ 文件已复制${NC}"
    
    # 修改包名
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        sed -i '' 's/^package eventbus$/package function_tests/' "$target_file"
    else
        # Linux
        sed -i 's/^package eventbus$/package function_tests/' "$target_file"
    fi
    echo -e "  ${GREEN}✓ 包名已修改${NC}"
    
    # 添加导入（如果不存在）
    if ! grep -q 'github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus' "$target_file"; then
        # 查找 import 块
        if grep -q '^import ($' "$target_file"; then
            # 在 import 块中添加
            if [[ "$OSTYPE" == "darwin"* ]]; then
                sed -i '' '/^import ($/a\
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
' "$target_file"
            else
                sed -i '/^import ($/a\	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"' "$target_file"
            fi
            echo -e "  ${GREEN}✓ 导入已添加${NC}"
        else
            echo -e "  ${YELLOW}⚠ 未找到 import 块，需要手动添加导入${NC}"
        fi
    else
        echo -e "  ${GREEN}✓ 导入已存在${NC}"
    fi
    
    # 更新类型引用（基本的替换，可能需要手动调整）
    # 注意：这是一个简化的替换，实际可能需要更复杂的逻辑
    echo -e "  ${BLUE}ℹ 正在更新类型引用...${NC}"
    
    # 创建临时文件进行替换
    local temp_file="${target_file}.tmp"
    
    # 使用 awk 进行更智能的替换
    awk '
    /^package function_tests/ { print; next }
    /^import/ { in_import=1 }
    /^\)/ && in_import { in_import=0; print; next }
    in_import { print; next }
    {
        # 替换常见的类型引用
        gsub(/\bEventBusConfig\b/, "eventbus.EventBusConfig")
        gsub(/\bEventBusType/, "eventbus.EventBusType")
        gsub(/\bEventBus\b/, "eventbus.EventBus")
        gsub(/\bEnvelope\b/, "eventbus.Envelope")
        gsub(/\bMessageHandler\b/, "eventbus.MessageHandler")
        gsub(/\bHealthStatus\b/, "eventbus.HealthStatus")
        gsub(/\bMetrics\b/, "eventbus.Metrics")
        gsub(/\bNewEventBus\b/, "eventbus.NewEventBus")
        gsub(/\bNewFactory\b/, "eventbus.NewFactory")
        
        # 避免重复添加 eventbus. 前缀
        gsub(/eventbus\.eventbus\./, "eventbus.")
        
        print
    }
    ' "$target_file" > "$temp_file"
    
    mv "$temp_file" "$target_file"
    echo -e "  ${GREEN}✓ 类型引用已更新${NC}"
    
    SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    echo -e "  ${GREEN}✓ 迁移完成${NC}"
    echo ""
    
    return 0
}

# 运行测试
run_tests() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}运行测试验证${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    
    cd "$TARGET_DIR"
    
    echo -e "${YELLOW}正在运行测试...${NC}"
    if go test -v ./... 2>&1 | tee test_migration.log; then
        echo -e "${GREEN}✓ 所有测试通过${NC}"
    else
        echo -e "${RED}✗ 部分测试失败，请查看 test_migration.log${NC}"
    fi
    
    echo ""
}

# 生成覆盖率报告
generate_coverage() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}生成覆盖率报告${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    
    cd "$TARGET_DIR"
    
    echo -e "${YELLOW}正在生成覆盖率报告...${NC}"
    if go test -coverprofile=coverage_migrated.out -covermode=atomic 2>&1; then
        echo -e "${GREEN}✓ 覆盖率数据已生成${NC}"
        
        # 生成文本报告
        go tool cover -func=coverage_migrated.out > coverage_migrated.txt
        echo -e "${GREEN}✓ 文本报告已生成: coverage_migrated.txt${NC}"
        
        # 生成 HTML 报告
        go tool cover -html=coverage_migrated.out -o coverage_migrated.html
        echo -e "${GREEN}✓ HTML 报告已生成: coverage_migrated.html${NC}"
        
        # 显示总体覆盖率
        echo ""
        echo -e "${BLUE}总体覆盖率:${NC}"
        grep "total:" coverage_migrated.txt || echo "无法获取覆盖率"
    else
        echo -e "${RED}✗ 覆盖率生成失败${NC}"
    fi
    
    echo ""
}

# 显示统计信息
show_statistics() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}迁移统计${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    echo -e "总文件数: ${BLUE}$TOTAL_FILES${NC}"
    echo -e "成功迁移: ${GREEN}$SUCCESS_COUNT${NC}"
    echo -e "跳过文件: ${YELLOW}$SKIP_COUNT${NC}"
    echo -e "失败文件: ${RED}$FAIL_COUNT${NC}"
    echo ""
    
    if [ $SUCCESS_COUNT -gt 0 ]; then
        echo -e "${GREEN}✓ 迁移完成！${NC}"
    else
        echo -e "${RED}✗ 没有文件被迁移${NC}"
    fi
}

# 主函数
main() {
    # 检查目标目录是否存在
    if [ ! -d "$TARGET_DIR" ]; then
        echo -e "${RED}错误: 目标目录不存在: $TARGET_DIR${NC}"
        exit 1
    fi
    
    # 询问用户要迁移哪一批
    echo -e "${YELLOW}请选择要迁移的批次:${NC}"
    echo "1) P0 - 高优先级 (15个文件)"
    echo "2) P1 - 中优先级 (15个文件)"
    echo "3) 全部 (30个文件)"
    echo "4) 退出"
    echo ""
    read -p "请输入选项 (1-4): " choice
    
    case $choice in
        1)
            echo -e "${GREEN}开始迁移 P0 测试文件...${NC}"
            echo ""
            for file in "${P0_FILES[@]}"; do
                migrate_file "$file"
            done
            ;;
        2)
            echo -e "${GREEN}开始迁移 P1 测试文件...${NC}"
            echo ""
            for file in "${P1_FILES[@]}"; do
                migrate_file "$file"
            done
            ;;
        3)
            echo -e "${GREEN}开始迁移所有测试文件...${NC}"
            echo ""
            for file in "${P0_FILES[@]}" "${P1_FILES[@]}"; do
                migrate_file "$file"
            done
            ;;
        4)
            echo -e "${YELLOW}退出${NC}"
            exit 0
            ;;
        *)
            echo -e "${RED}无效选项${NC}"
            exit 1
            ;;
    esac
    
    # 显示统计
    show_statistics
    
    # 询问是否运行测试
    echo ""
    read -p "是否运行测试验证? (y/n): " run_test
    if [ "$run_test" = "y" ] || [ "$run_test" = "Y" ]; then
        run_tests
    fi
    
    # 询问是否生成覆盖率报告
    echo ""
    read -p "是否生成覆盖率报告? (y/n): " gen_coverage
    if [ "$gen_coverage" = "y" ] || [ "$gen_coverage" = "Y" ]; then
        generate_coverage
    fi
    
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}迁移流程完成！${NC}"
    echo -e "${GREEN}========================================${NC}"
}

# 执行主函数
main

