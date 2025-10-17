#!/bin/bash

# go-admin-core v1.6.0 Migration Tool
# 自动更新导入路径从 sdk/pkg/* 到根目录

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查是否在 Go 项目根目录
check_go_project() {
    if [ ! -f "go.mod" ]; then
        print_error "未找到 go.mod 文件，请在 Go 项目根目录运行此脚本"
        exit 1
    fi
    print_info "检测到 Go 项目: $(grep "^module" go.mod | awk '{print $2}')"
}

# 检查是否使用了 go-admin-core
check_dependency() {
    if ! grep -q "github.com/go-admin-team/go-admin-core" go.mod; then
        print_error "项目未依赖 go-admin-core，无需迁移"
        exit 1
    fi
    print_success "检测到 go-admin-core 依赖"
}

# 备份项目
backup_project() {
    local backup_dir="backup_before_v1.8_$(date +%Y%m%d_%H%M%S)"
    print_info "创建备份: $backup_dir"
    
    # 创建备份目录（排除 vendor 和 .git）
    mkdir -p "../$backup_dir"
    rsync -av --exclude='vendor' --exclude='.git' --exclude='node_modules' ./ "../$backup_dir/" > /dev/null
    
    print_success "备份完成: ../$backup_dir"
}

# 查找需要更新的文件
find_files_to_update() {
    print_info "扫描需要更新的文件..."
    
    local files=$(grep -rl "github.com/go-admin-team/go-admin-core/\(sdk/pkg/\(captcha\|jwtauth\|response\|casbin\)\|observability/audit\|tools/gorm/logger\)" \
        --include="*.go" \
        --exclude-dir="vendor" \
        --exclude-dir=".git" \
        . 2>/dev/null || true)
    
    if [ -z "$files" ]; then
        print_warning "未找到需要更新的文件"
        return 1
    fi
    
    echo "$files"
}

# 执行迁移
migrate_imports() {
    local files="$1"
    local count=0
    
    print_info "开始更新导入路径..."
    
    # 使用数组存储文件列表
    local file_array=()
    while IFS= read -r file; do
        [ -n "$file" ] && file_array+=("$file")
    done <<< "$files"
    
    for file in "${file_array[@]}"; do
        print_info "处理文件: $file"
        
        # 创建临时文件
        local tmpfile="${file}.tmp"
        
        # 执行替换
        sed -e 's|github\.com/go-admin-team/go-admin-core/sdk/pkg/captcha|github.com/go-admin-team/go-admin-core/captcha|g' \
            -e 's|github\.com/go-admin-team/go-admin-core/sdk/pkg/jwtauth|github.com/go-admin-team/go-admin-core/jwtauth|g' \
            -e 's|github\.com/go-admin-team/go-admin-core/sdk/pkg/response|github.com/go-admin-team/go-admin-core/response|g' \
            -e 's|github\.com/go-admin-team/go-admin-core/sdk/pkg/casbin|github.com/go-admin-team/go-admin-core/casbin|g' \
            -e 's|github\.com/go-admin-team/go-admin-core/observability/audit|github.com/go-admin-team/go-admin-core/observe/audit|g' \
            -e 's|github\.com/go-admin-team/go-admin-core/tools/gorm/logger|github.com/go-admin-team/go-admin-core/tools/gorm/gormlog|g' \
            "$file" > "$tmpfile"
        
        # 替换原文件
        mv "$tmpfile" "$file"
        
        ((count++))
    done
    
    print_success "已更新 $count 个文件"
    return 0
}

# 显示变更摘要
show_summary() {
    print_info "变更摘要:"
    echo ""
    echo "已更新的导入路径:"
    echo "  sdk/pkg/captcha      → captcha"
    echo "  sdk/pkg/jwtauth      → jwtauth"
    echo "  sdk/pkg/response     → response"
    echo "  sdk/pkg/casbin       → casbin"
    echo "  observability/audit  → observe/audit"
    echo "  tools/gorm/logger    → tools/gorm/gormlog"
    echo ""
}

# 运行测试
run_tests() {
    print_info "运行 go mod tidy..."
    if go mod tidy; then
        print_success "go mod tidy 成功"
    else
        print_error "go mod tidy 失败，请手动检查"
        return 1
    fi
    
    print_info "尝试编译项目..."
    if go build ./...; then
        print_success "项目编译成功"
    else
        print_warning "编译失败，可能需要手动调整部分代码"
        return 1
    fi
}

# 生成迁移报告
generate_report() {
    local report_file="migration_report_$(date +%Y%m%d_%H%M%S).txt"
    
    print_info "生成迁移报告: $report_file"
    
    cat > "$report_file" << EOF
go-admin-core v1.6.0 迁移报告
====================================
迁移时间: $(date)
项目模块: $(grep "^module" go.mod | awk '{print $2}')

变更内容:
- sdk/pkg/captcha  → captcha
- sdk/pkg/jwtauth  → jwtauth
- sdk/pkg/response → response
- sdk/pkg/casbin   → casbin

注意事项:
1. 旧路径将在 v2.0.0 版本中移除
2. 新旧路径在 v1.6.0 - v1.9.x 期间都可用
3. 建议尽快迁移到新路径
4. 如遇到问题，可恢复备份目录

更多信息:
https://github.com/go-admin-team/go-admin-core/blob/dev/docs/migration/v1.6.0-plan.md
EOF
    
    print_success "报告已生成: $report_file"
}

# 主函数
main() {
    echo ""
    echo "================================================"
    echo "  go-admin-core v1.6.0 自动迁移工具"
    echo "================================================"
    echo ""
    
    # 1. 检查环境
    check_go_project
    check_dependency
    
    # 2. 询问是否继续
    echo ""
    read -p "$(echo -e ${YELLOW}是否继续迁移？这将修改您的代码文件 [y/N]:${NC} )" -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_info "取消迁移"
        exit 0
    fi
    
    # 3. 创建备份
    backup_project
    
    # 4. 查找需要更新的文件
    local files=$(find_files_to_update)
    if [ $? -ne 0 ]; then
        print_success "项目已是最新状态，无需迁移"
        exit 0
    fi
    
    echo ""
    print_info "找到以下文件需要更新:"
    echo "$files" | while read -r file; do
        echo "  - $file"
    done
    echo ""
    
    # 5. 执行迁移
    migrate_imports "$files"
    
    # 6. 显示摘要
    show_summary
    
    # 7. 运行测试
    run_tests
    
    # 8. 生成报告
    generate_report
    
    # 9. 完成
    echo ""
    echo "================================================"
    print_success "迁移完成！"
    echo "================================================"
    echo ""
    print_info "下一步:"
    echo "  1. 检查变更: git diff"
    echo "  2. 运行测试: go test ./..."
    echo "  3. 提交变更: git commit -am 'chore: migrate to go-admin-core v1.6.0'"
    echo ""
}

# 执行主函数
main "$@"
