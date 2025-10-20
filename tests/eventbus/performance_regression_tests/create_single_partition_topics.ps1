# 创建单分区 Kafka Topics 用于顺序验证测试
# 用途：验证 Keyed-Worker Pool 的顺序保证是否正确

$KAFKA_CONTAINER = "benchmark-kafka"
$KAFKA_BROKER = "localhost:29094"

Write-Host "🔧 创建单分区 Kafka Topics..." -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""

# 定义压力级别
$PRESSURES = @("low", "medium", "high", "extreme")

foreach ($pressure in $PRESSURES) {
    Write-Host ""
    Write-Host "📋 创建 $pressure 压力级别的 topics..." -ForegroundColor Yellow
    
    for ($i = 1; $i -le 5; $i++) {
        $TOPIC = "kafka.perf.$pressure.topic$i"
        
        # 检查 topic 是否已存在
        $checkCmd = "kafka-topics.sh --describe --topic $TOPIC --bootstrap-server $KAFKA_BROKER"
        $checkResult = docker exec $KAFKA_CONTAINER bash -c $checkCmd 2>&1
        
        if ($LASTEXITCODE -eq 0 -and $checkResult -notmatch "does not exist") {
            Write-Host "  ⚠️  Topic $TOPIC 已存在，先删除..." -ForegroundColor Yellow
            $deleteCmd = "kafka-topics.sh --delete --topic $TOPIC --bootstrap-server $KAFKA_BROKER"
            docker exec $KAFKA_CONTAINER bash -c $deleteCmd | Out-Null
            Start-Sleep -Seconds 1
        }
        
        # 创建单分区 topic
        $createCmd = "kafka-topics.sh --create --topic $TOPIC --partitions 1 --replication-factor 1 --bootstrap-server $KAFKA_BROKER --config retention.ms=3600000 --config segment.ms=3600000"
        $createResult = docker exec $KAFKA_CONTAINER bash -c $createCmd 2>&1
        
        if ($LASTEXITCODE -eq 0) {
            Write-Host "  ✅ 成功创建 topic: $TOPIC (1 partition)" -ForegroundColor Green
        } else {
            Write-Host "  ❌ 创建 topic 失败: $TOPIC" -ForegroundColor Red
            Write-Host "     错误: $createResult" -ForegroundColor Red
        }
    }
}

Write-Host ""
Write-Host "================================" -ForegroundColor Cyan
Write-Host "✅ 单分区 Topics 创建完成！" -ForegroundColor Green
Write-Host ""
Write-Host "📊 验证创建的 topics:" -ForegroundColor Cyan
Write-Host ""

foreach ($pressure in $PRESSURES) {
    Write-Host "🔍 $pressure 压力级别:" -ForegroundColor Yellow
    for ($i = 1; $i -le 5; $i++) {
        $TOPIC = "kafka.perf.$pressure.topic$i"
        $describeCmd = "kafka-topics.sh --describe --topic $TOPIC --bootstrap-server $KAFKA_BROKER"
        $describeResult = docker exec $KAFKA_CONTAINER bash -c $describeCmd 2>&1
        
        if ($describeResult -match "PartitionCount: (\d+)") {
            $partitionCount = $matches[1]
            Write-Host "  $TOPIC : $partitionCount partition(s)" -ForegroundColor White
        }
    }
    Write-Host ""
}

Write-Host "💡 提示：" -ForegroundColor Cyan
Write-Host "  - 所有 topics 都配置为 1 个分区" -ForegroundColor White
Write-Host "  - 这将保证 100% 的全局顺序" -ForegroundColor White
Write-Host "  - 如果测试仍有顺序违反，说明问题在 Keyed-Worker Pool 或检测逻辑" -ForegroundColor White
Write-Host "  - 如果测试顺序违反为 0，说明问题在多分区并发消费" -ForegroundColor White
Write-Host ""
Write-Host "🚀 现在可以运行测试：" -ForegroundColor Cyan
Write-Host "  cd tests/eventbus/performance_tests" -ForegroundColor Yellow
Write-Host "  go test -v -run TestKafkaVsNATSPerformanceComparison -timeout 15m" -ForegroundColor Yellow

