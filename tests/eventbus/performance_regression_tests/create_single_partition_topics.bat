@echo off
REM 创建单分区 Kafka Topics 用于顺序验证测试

echo 🔧 创建单分区 Kafka Topics...
echo ================================
echo.

REM 低压级别 topics
echo 📋 创建 low 压力级别的 topics...
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.low.topic1 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.low.topic2 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.low.topic3 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.low.topic4 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.low.topic5 --bootstrap-server localhost:29094 2>nul

timeout /t 2 /nobreak >nul

docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.low.topic1 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.low.topic2 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.low.topic3 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.low.topic4 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.low.topic5 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094

echo.
echo 📋 创建 medium 压力级别的 topics...
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.medium.topic1 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.medium.topic2 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.medium.topic3 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.medium.topic4 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.medium.topic5 --bootstrap-server localhost:29094 2>nul

timeout /t 2 /nobreak >nul

docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.medium.topic1 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.medium.topic2 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.medium.topic3 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.medium.topic4 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.medium.topic5 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094

echo.
echo 📋 创建 high 压力级别的 topics...
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.high.topic1 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.high.topic2 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.high.topic3 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.high.topic4 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.high.topic5 --bootstrap-server localhost:29094 2>nul

timeout /t 2 /nobreak >nul

docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.high.topic1 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.high.topic2 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.high.topic3 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.high.topic4 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.high.topic5 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094

echo.
echo 📋 创建 extreme 压力级别的 topics...
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.extreme.topic1 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.extreme.topic2 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.extreme.topic3 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.extreme.topic4 --bootstrap-server localhost:29094 2>nul
docker exec benchmark-kafka kafka-topics.sh --delete --topic kafka.perf.extreme.topic5 --bootstrap-server localhost:29094 2>nul

timeout /t 2 /nobreak >nul

docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.extreme.topic1 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.extreme.topic2 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.extreme.topic3 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.extreme.topic4 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094
docker exec benchmark-kafka kafka-topics.sh --create --topic kafka.perf.extreme.topic5 --partitions 1 --replication-factor 1 --bootstrap-server localhost:29094

echo.
echo ================================
echo ✅ 单分区 Topics 创建完成！
echo.
echo 📊 验证创建的 topics:
echo.
docker exec benchmark-kafka kafka-topics.sh --list --bootstrap-server localhost:29094 | findstr "kafka.perf"
echo.
echo 💡 提示：
echo   - 所有 topics 都配置为 1 个分区
echo   - 这将保证 100%% 的全局顺序
echo   - 如果测试仍有顺序违反，说明问题在 Keyed-Worker Pool 或检测逻辑
echo   - 如果测试顺序违反为 0，说明问题在多分区并发消费
echo.
echo 🚀 现在可以运行测试：
echo   go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 15m
echo.

