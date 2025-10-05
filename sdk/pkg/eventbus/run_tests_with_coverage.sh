#!/bin/bash

# EventBus 测试覆盖率运行脚本
# 用法: ./run_tests_with_coverage.sh

set -e

echo "=========================================="
echo "EventBus 组件测试覆盖率分析"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 1. 运行测试并生成覆盖率
echo "📋 步骤 1: 运行测试并生成覆盖率数据..."
echo ""

go test -v -coverprofile=coverage.out -covermode=atomic . 2>&1 | tee test_output.log

# 检查测试是否通过
if [ ${PIPESTATUS[0]} -eq 0 ]; then
    echo ""
    echo -e "${GREEN}✅ 所有测试通过！${NC}"
else
    echo ""
    echo -e "${RED}❌ 部分测试失败，请查看 test_output.log${NC}"
fi

echo ""
echo "=========================================="

# 2. 生成 HTML 报告
echo "📊 步骤 2: 生成 HTML 覆盖率报告..."
go tool cover -html=coverage.out -o coverage.html
echo -e "${GREEN}✅ HTML 报告已生成: coverage.html${NC}"
echo ""

# 3. 显示总体覆盖率
echo "=========================================="
echo "📈 步骤 3: 总体覆盖率"
echo "=========================================="
TOTAL_COVERAGE=$(go tool cover -func=coverage.out | grep "total:" | awk '{print $3}')
echo ""
echo -e "   总体覆盖率: ${GREEN}${TOTAL_COVERAGE}${NC}"
echo ""

# 4. 显示各文件覆盖率
echo "=========================================="
echo "📁 步骤 4: 各文件覆盖率详情"
echo "=========================================="
echo ""

# 使用 Python 生成详细报告
python3 generate_coverage_report.py

echo ""
echo "=========================================="
echo "📚 查看详细报告"
echo "=========================================="
echo ""
echo "1. HTML 可视化报告:"
echo "   file://$(pwd)/coverage.html"
echo ""
echo "2. 详细文字报告:"
echo "   cat COVERAGE_REPORT_2025-10-01.md"
echo ""
echo "3. 测试输出日志:"
echo "   cat test_output.log"
echo ""
echo "=========================================="
echo -e "${GREEN}✅ 测试覆盖率分析完成！${NC}"
echo "=========================================="
echo ""

# 5. 询问是否打开 HTML 报告
read -p "是否在浏览器中打开 HTML 报告? (y/n) " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    if command -v xdg-open &> /dev/null; then
        xdg-open coverage.html
    elif command -v open &> /dev/null; then
        open coverage.html
    else
        echo "请手动打开: file://$(pwd)/coverage.html"
    fi
fi

