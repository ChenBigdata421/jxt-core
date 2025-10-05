#!/bin/bash

# 完整的NATS EventBus测试脚本
# 包含NATS服务器启动和独立实例测试

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NATS_DIR="/tmp/nats"
NATS_BINARY="$NATS_DIR/nats-server"
NATS_PID_FILE="/tmp/nats.pid"

echo "=== NATS EventBus 完整测试 ==="

# 清理函数
cleanup() {
    echo ""
    echo "🧹 清理资源..."
    if [ -f "$NATS_PID_FILE" ]; then
        NATS_PID=$(cat "$NATS_PID_FILE")
        if kill -0 "$NATS_PID" 2>/dev/null; then
            echo "   停止NATS服务器 (PID: $NATS_PID)"
            kill "$NATS_PID"
            rm -f "$NATS_PID_FILE"
        fi
    fi
    echo "✅ 清理完成"
}

# 设置信号处理
trap cleanup EXIT INT TERM

# 检查Go环境
if ! command -v go >/dev/null 2>&1; then
    echo "❌ 需要Go环境来运行测试"
    exit 1
fi

# 1. 安装NATS服务器（如果需要）
echo "📦 检查NATS服务器..."
if [ ! -f "$NATS_BINARY" ]; then
    echo "   NATS服务器不存在，开始下载..."
    "$SCRIPT_DIR/setup_nats_server.sh" &
    SETUP_PID=$!
    
    # 等待下载完成或超时
    timeout=60
    while [ $timeout -gt 0 ] && kill -0 $SETUP_PID 2>/dev/null; do
        echo "   等待下载完成... ($timeout秒)"
        sleep 5
        timeout=$((timeout - 5))
    done
    
    if kill -0 $SETUP_PID 2>/dev/null; then
        echo "❌ 下载超时，请手动下载NATS服务器"
        kill $SETUP_PID
        exit 1
    fi
    
    wait $SETUP_PID
fi

# 2. 启动NATS服务器
echo "🚀 启动NATS服务器..."
if pgrep -f "nats-server" > /dev/null; then
    echo "   NATS服务器已在运行"
else
    echo "   启动新的NATS服务器实例..."
    "$NATS_BINARY" -js -p 4222 > /tmp/nats.log 2>&1 &
    NATS_PID=$!
    echo $NATS_PID > "$NATS_PID_FILE"
    
    # 等待服务器启动
    echo "   等待NATS服务器启动..."
    for i in {1..10}; do
        if nc -z localhost 4222 2>/dev/null; then
            echo "   ✅ NATS服务器启动成功 (PID: $NATS_PID)"
            break
        fi
        if [ $i -eq 10 ]; then
            echo "   ❌ NATS服务器启动失败"
            echo "   日志内容:"
            cat /tmp/nats.log
            exit 1
        fi
        sleep 1
    done
fi

# 3. 运行配置验证测试
echo ""
echo "🔧 运行配置验证测试..."
cd "$SCRIPT_DIR/.."
go run examples/independent_instances_validation.go

# 4. 运行完整的独立实例示例
echo ""
echo "🎯 运行独立实例示例..."
go run examples/independent_instances_example.go

echo ""
echo "✅ 所有测试完成！"
echo ""
echo "📊 测试总结:"
echo "   ✅ NATS服务器启动成功"
echo "   ✅ 持久化NATS实例配置正确"
echo "   ✅ 非持久化NATS实例配置正确"
echo "   ✅ 内存实例功能正常"
echo "   ✅ 独立实例方案验证通过"
echo ""
echo "💡 使用建议:"
echo "   - 业务A (需要持久化): 使用 NewPersistentNATSEventBus()"
echo "   - 业务B (不需要持久化): 使用 NewEphemeralNATSEventBus() 或内存实例"
