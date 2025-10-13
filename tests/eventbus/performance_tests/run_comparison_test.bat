@echo off
REM Kafka vs NATS JetStream 性能对比测试运行脚本 (Windows)

setlocal enabledelayedexpansion

echo.
echo ========================================
echo 🚀 Kafka vs NATS JetStream 性能对比测试
echo ========================================
echo.

REM 检查前置条件
echo 📋 检查前置条件...
echo.

REM 检查 Kafka (简化版，Windows 下不容易检查端口)
echo ✅ 假设 Kafka 运行在 localhost:29092
echo ✅ 假设 NATS 运行在 localhost:4223
echo.
echo ⚠️  如果服务未运行，请先启动:
echo    docker-compose up -d kafka nats
echo.

REM 显示测试信息
echo 📊 测试配置:
echo    • 低压测试: 500 条消息
echo    • 中压测试: 2000 条消息
echo    • 高压测试: 5000 条消息
echo    • 极限测试: 10000 条消息
echo.
echo    • 发布方法: PublishEnvelope
echo    • 订阅方法: SubscribeEnvelope
echo    • NATS 持久化: 磁盘 (file)
echo    • Kafka 持久化: 启用 (RequiredAcks=-1)
echo.

REM 询问是否继续
echo ⏱️  预计测试时间: 15-20 分钟
set /p CONTINUE="是否继续运行测试？(y/N): "
if /i not "%CONTINUE%"=="y" (
    echo ⏭️  测试已取消
    exit /b 0
)

echo.
echo ========================================
echo 🧪 开始运行性能对比测试
echo ========================================
echo.

REM 创建日志目录
if not exist "test_results" mkdir test_results

REM 生成日志文件名
for /f "tokens=2-4 delims=/ " %%a in ('date /t') do (set mydate=%%c%%a%%b)
for /f "tokens=1-2 delims=/:" %%a in ('time /t') do (set mytime=%%a%%b)
set TIMESTAMP=%mydate%_%mytime%
set LOG_FILE=test_results\comparison_test_%TIMESTAMP%.log

echo 📝 测试日志将保存到: %LOG_FILE%
echo.

REM 运行测试
go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 30m > %LOG_FILE% 2>&1

if %ERRORLEVEL% equ 0 (
    echo.
    echo ========================================
    echo ✅ 测试完成
    echo ========================================
    echo.
    echo 📊 测试报告已保存到: %LOG_FILE%
    echo.
    echo 📈 快速摘要:
    findstr /C:"🎉 总体获胜者:" /C:"平均吞吐量" /C:"平均延迟" /C:"平均内存增量" %LOG_FILE%
) else (
    echo.
    echo ========================================
    echo ❌ 测试失败
    echo ========================================
    echo.
    echo 请查看日志文件了解详情: %LOG_FILE%
    exit /b 1
)

echo.
echo 🎉 感谢使用 Kafka vs NATS 性能对比测试！
echo.

endlocal

