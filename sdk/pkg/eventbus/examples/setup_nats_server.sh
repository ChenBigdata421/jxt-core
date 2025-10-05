#!/bin/bash

# NATS服务器安装和启动脚本
# 用于测试独立EventBus实例

set -e

NATS_VERSION="v2.10.7"
NATS_DIR="/tmp/nats"
NATS_BINARY="$NATS_DIR/nats-server"

echo "=== NATS服务器安装和启动脚本 ==="

# 检查是否已经有NATS服务器在运行
if pgrep -f "nats-server" > /dev/null; then
    echo "✅ NATS服务器已在运行"
    echo "   进程信息:"
    ps aux | grep nats-server | grep -v grep
    exit 0
fi

# 检查是否已经下载了NATS二进制文件
if [ -f "$NATS_BINARY" ]; then
    echo "✅ NATS二进制文件已存在: $NATS_BINARY"
else
    echo "📥 下载NATS服务器..."
    
    # 创建目录
    mkdir -p "$NATS_DIR"
    cd "$NATS_DIR"
    
    # 检测系统架构
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)
            NATS_ARCH="amd64"
            ;;
        aarch64|arm64)
            NATS_ARCH="arm64"
            ;;
        *)
            echo "❌ 不支持的架构: $ARCH"
            exit 1
            ;;
    esac
    
    # 下载NATS服务器
    NATS_URL="https://github.com/nats-io/nats-server/releases/download/${NATS_VERSION}/nats-server-${NATS_VERSION}-linux-${NATS_ARCH}.zip"
    echo "   下载地址: $NATS_URL"
    
    if command -v curl >/dev/null 2>&1; then
        curl -L -o "nats-server.zip" "$NATS_URL"
    elif command -v wget >/dev/null 2>&1; then
        wget -O "nats-server.zip" "$NATS_URL"
    else
        echo "❌ 需要curl或wget来下载文件"
        exit 1
    fi
    
    # 解压
    echo "📦 解压NATS服务器..."
    unzip -q "nats-server.zip"
    
    # 移动二进制文件
    mv "nats-server-${NATS_VERSION}-linux-${NATS_ARCH}/nats-server" ./
    chmod +x nats-server
    
    # 清理
    rm -rf "nats-server.zip" "nats-server-${NATS_VERSION}-linux-${NATS_ARCH}"
    
    echo "✅ NATS服务器下载完成"
fi

# 启动NATS服务器
echo "🚀 启动NATS服务器（支持JetStream）..."
echo "   二进制文件: $NATS_BINARY"
echo "   启动命令: $NATS_BINARY -js -p 4222"
echo ""
echo "📝 注意："
echo "   - NATS服务器将在前台运行"
echo "   - 按 Ctrl+C 停止服务器"
echo "   - 服务器地址: nats://localhost:4222"
echo "   - JetStream已启用"
echo ""

# 启动服务器
exec "$NATS_BINARY" -js -p 4222
