@echo off
REM 创建 3 分区的 Kafka Topics
REM 用于测试多分区环境下的顺序保证

echo 🚀 开始创建 3 分区的 Kafka Topics...

REM Kafka 容器名称
set KAFKA_CONTAINER=benchmark-kafka
set BOOTSTRAP_SERVER=localhost:29094

REM 检查 Kafka 容器是否运行
docker ps | findstr /C:"%KAFKA_CONTAINER%" >nul
if errorlevel 1 (
    echo ❌ Kafka 容器未运行，请先启动 docker-compose-nats.yml
    exit /b 1
)

REM 压力级别
set PRESSURES=low medium high extreme

for %%p in (%PRESSURES%) do (
    echo.
    echo 📋 创建 %%p 压力级别的 topics...
    
    for /L %%i in (1,1,5) do (
        set TOPIC=kafka.perf.%%p.topic%%i
        
        REM 删除已存在的 topic（如果存在）
        docker exec %KAFKA_CONTAINER% kafka-topics.sh --delete --topic !TOPIC! --bootstrap-server %BOOTSTRAP_SERVER% 2>nul
        
        REM 创建新的 topic（3 个分区）
        docker exec %KAFKA_CONTAINER% kafka-topics.sh --create --topic !TOPIC! --partitions 3 --replication-factor 1 --bootstrap-server %BOOTSTRAP_SERVER%
        
        if !errorlevel! equ 0 (
            echo   ✅ 创建成功: !TOPIC! (3 partitions^)
        ) else (
            echo   ❌ 创建失败: !TOPIC!
        )
    )
)

echo.
echo 🎉 所有 topics 创建完成！
echo.
echo 📊 验证创建的 topics:
docker exec %KAFKA_CONTAINER% kafka-topics.sh --list --bootstrap-server %BOOTSTRAP_SERVER% | findstr "kafka.perf"

echo.
echo 📋 查看 topic 详情（示例）:
docker exec %KAFKA_CONTAINER% kafka-topics.sh --describe --topic "kafka.perf.low.topic1" --bootstrap-server %BOOTSTRAP_SERVER%

pause

