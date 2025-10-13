# PowerShell 脚本：连续运行多次性能测试并分析结果

param(
    [int]$Count = 20
)

$ErrorActionPreference = "Continue"

Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "开始连续运行 $Count 次性能测试" -ForegroundColor Cyan
Write-Host "测试文件: kafka_nats_comparison_test.go" -ForegroundColor Cyan
Write-Host "开始时间: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan

# 创建结果目录
$timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
$resultDir = "test_results_$timestamp"
New-Item -ItemType Directory -Path $resultDir -Force | Out-Null

# 统计变量
$passCount = 0
$failCount = 0
$allMetrics = @()

# 运行测试
for ($i = 1; $i -le $Count; $i++) {
    Write-Host ""
    Write-Host "==========================================" -ForegroundColor Yellow
    Write-Host "第 $i 次测试 ($(Get-Date -Format 'HH:mm:ss'))" -ForegroundColor Yellow
    Write-Host "==========================================" -ForegroundColor Yellow
    
    $logFile = "$resultDir\test_run_$i.log"
    $metricsFile = "$resultDir\metrics_$i.txt"
    
    # 运行测试
    $startTime = Get-Date
    go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 30m 2>&1 | Tee-Object -FilePath $logFile
    $exitCode = $LASTEXITCODE
    $duration = (Get-Date) - $startTime
    
    if ($exitCode -eq 0) {
        Write-Host "✅ 第 $i 次测试: PASS (耗时: $($duration.ToString('mm\:ss')))" -ForegroundColor Green
        $passCount++
    } else {
        Write-Host "❌ 第 $i 次测试: FAIL (耗时: $($duration.ToString('mm\:ss')))" -ForegroundColor Red
        $failCount++
    }
    
    # 提取关键指标
    $content = Get-Content $logFile
    $metrics = @{
        RunNumber = $i
        Passed = ($exitCode -eq 0)
        Duration = $duration
        KafkaOrderViolations = 0
        NATSOrderViolations = 0
        KafkaGoroutineLeak = 0
        NATSGoroutineLeak = 0
        KafkaSuccessRate = 0.0
        NATSSuccessRate = 0.0
    }
    
    foreach ($line in $content) {
        # 提取顺序违反
        if ($line -match '顺序违反.*Kafka:\s*(\d+).*NATS:\s*(\d+)') {
            $metrics.KafkaOrderViolations = [int]$matches[1]
            $metrics.NATSOrderViolations = [int]$matches[2]
        }
        
        # 提取协程泄漏
        if ($line -match '协程泄漏.*Kafka:\s*(\d+).*NATS:\s*(\d+)') {
            $metrics.KafkaGoroutineLeak = [int]$matches[1]
            $metrics.NATSGoroutineLeak = [int]$matches[2]
        }
        
        # 提取成功率
        if ($line -match '成功率.*Kafka:\s*([\d.]+)%.*NATS:\s*([\d.]+)%') {
            $metrics.KafkaSuccessRate = [double]$matches[1]
            $metrics.NATSSuccessRate = [double]$matches[2]
        }
    }
    
    $allMetrics += $metrics
    
    # 保存指标
    $metrics | ConvertTo-Json | Out-File $metricsFile
    
    # 打印当前测试的关键指标
    Write-Host "  Kafka 顺序违反: $($metrics.KafkaOrderViolations)" -ForegroundColor $(if ($metrics.KafkaOrderViolations -eq 0) { "Green" } else { "Red" })
    Write-Host "  NATS 顺序违反: $($metrics.NATSOrderViolations)" -ForegroundColor $(if ($metrics.NATSOrderViolations -eq 0) { "Green" } else { "Red" })
    Write-Host "  Kafka 协程泄漏: $($metrics.KafkaGoroutineLeak)" -ForegroundColor $(if ($metrics.KafkaGoroutineLeak -lt 100) { "Green" } else { "Red" })
    Write-Host "  NATS 协程泄漏: $($metrics.NATSGoroutineLeak)" -ForegroundColor $(if ($metrics.NATSGoroutineLeak -lt 100) { "Green" } else { "Red" })
    Write-Host "  Kafka 成功率: $($metrics.KafkaSuccessRate)%" -ForegroundColor $(if ($metrics.KafkaSuccessRate -ge 99) { "Green" } else { "Yellow" })
    Write-Host "  NATS 成功率: $($metrics.NATSSuccessRate)%" -ForegroundColor $(if ($metrics.NATSSuccessRate -ge 90) { "Green" } else { "Red" })
    
    # 等待清理资源
    if ($i -lt $Count) {
        Write-Host "等待 10 秒，清理资源..." -ForegroundColor Gray
        Start-Sleep -Seconds 10
    }
}

# 生成汇总报告
Write-Host ""
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "测试完成！生成汇总报告..." -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan

$successRate = [math]::Round($passCount * 100 / $Count, 2)

# 计算统计数据
$kafkaOrderViolations = $allMetrics | ForEach-Object { $_.KafkaOrderViolations }
$natsOrderViolations = $allMetrics | ForEach-Object { $_.NATSOrderViolations }
$kafkaGoroutineLeaks = $allMetrics | ForEach-Object { $_.KafkaGoroutineLeak }
$natsGoroutineLeaks = $allMetrics | ForEach-Object { $_.NATSGoroutineLeak }
$kafkaSuccessRates = $allMetrics | ForEach-Object { $_.KafkaSuccessRate }
$natsSuccessRates = $allMetrics | ForEach-Object { $_.NATSSuccessRate }

$stats = @{
    TotalRuns = $Count
    PassedRuns = $passCount
    FailedRuns = $failCount
    SuccessRate = $successRate
    
    KafkaOrderViolationsMin = ($kafkaOrderViolations | Measure-Object -Minimum).Minimum
    KafkaOrderViolationsMax = ($kafkaOrderViolations | Measure-Object -Maximum).Maximum
    KafkaOrderViolationsAvg = [math]::Round(($kafkaOrderViolations | Measure-Object -Average).Average, 2)
    KafkaOrderViolationRuns = ($kafkaOrderViolations | Where-Object { $_ -gt 0 }).Count
    
    NATSOrderViolationsMin = ($natsOrderViolations | Measure-Object -Minimum).Minimum
    NATSOrderViolationsMax = ($natsOrderViolations | Measure-Object -Maximum).Maximum
    NATSOrderViolationsAvg = [math]::Round(($natsOrderViolations | Measure-Object -Average).Average, 2)
    NATSOrderViolationRuns = ($natsOrderViolations | Where-Object { $_ -gt 0 }).Count
    
    KafkaGoroutineLeakMin = ($kafkaGoroutineLeaks | Measure-Object -Minimum).Minimum
    KafkaGoroutineLeakMax = ($kafkaGoroutineLeaks | Measure-Object -Maximum).Maximum
    KafkaGoroutineLeakAvg = [math]::Round(($kafkaGoroutineLeaks | Measure-Object -Average).Average, 2)
    KafkaGoroutineLeakRuns = ($kafkaGoroutineLeaks | Where-Object { $_ -gt 100 }).Count
    
    NATSGoroutineLeakMin = ($natsGoroutineLeaks | Measure-Object -Minimum).Minimum
    NATSGoroutineLeakMax = ($natsGoroutineLeaks | Measure-Object -Maximum).Maximum
    NATSGoroutineLeakAvg = [math]::Round(($natsGoroutineLeaks | Measure-Object -Average).Average, 2)
    NATSGoroutineLeakRuns = ($natsGoroutineLeaks | Where-Object { $_ -gt 100 }).Count
    
    KafkaSuccessRateMin = [math]::Round(($kafkaSuccessRates | Measure-Object -Minimum).Minimum, 2)
    KafkaSuccessRateMax = [math]::Round(($kafkaSuccessRates | Measure-Object -Maximum).Maximum, 2)
    KafkaSuccessRateAvg = [math]::Round(($kafkaSuccessRates | Measure-Object -Average).Average, 2)
    
    NATSSuccessRateMin = [math]::Round(($natsSuccessRates | Measure-Object -Minimum).Minimum, 2)
    NATSSuccessRateMax = [math]::Round(($natsSuccessRates | Measure-Object -Maximum).Maximum, 2)
    NATSSuccessRateAvg = [math]::Round(($natsSuccessRates | Measure-Object -Average).Average, 2)
}

# 打印汇总
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "$Count 次测试汇总报告" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "测试通过率: $passCount/$Count ($successRate%)" -ForegroundColor $(if ($successRate -ge 90) { "Green" } else { "Red" })
Write-Host ""
Write-Host "Kafka 顺序违反:" -ForegroundColor Yellow
Write-Host "  最小值: $($stats.KafkaOrderViolationsMin)"
Write-Host "  最大值: $($stats.KafkaOrderViolationsMax)"
Write-Host "  平均值: $($stats.KafkaOrderViolationsAvg)"
Write-Host "  有违反的运行: $($stats.KafkaOrderViolationRuns)/$Count" -ForegroundColor $(if ($stats.KafkaOrderViolationRuns -eq 0) { "Green" } else { "Red" })
Write-Host ""
Write-Host "NATS 顺序违反:" -ForegroundColor Yellow
Write-Host "  最小值: $($stats.NATSOrderViolationsMin)"
Write-Host "  最大值: $($stats.NATSOrderViolationsMax)"
Write-Host "  平均值: $($stats.NATSOrderViolationsAvg)"
Write-Host "  有违反的运行: $($stats.NATSOrderViolationRuns)/$Count" -ForegroundColor $(if ($stats.NATSOrderViolationRuns -eq 0) { "Green" } else { "Red" })
Write-Host ""
Write-Host "Kafka 协程泄漏:" -ForegroundColor Yellow
Write-Host "  最小值: $($stats.KafkaGoroutineLeakMin)"
Write-Host "  最大值: $($stats.KafkaGoroutineLeakMax)"
Write-Host "  平均值: $($stats.KafkaGoroutineLeakAvg)"
Write-Host "  有泄漏的运行: $($stats.KafkaGoroutineLeakRuns)/$Count" -ForegroundColor $(if ($stats.KafkaGoroutineLeakRuns -eq 0) { "Green" } else { "Red" })
Write-Host ""
Write-Host "NATS 协程泄漏:" -ForegroundColor Yellow
Write-Host "  最小值: $($stats.NATSGoroutineLeakMin)"
Write-Host "  最大值: $($stats.NATSGoroutineLeakMax)"
Write-Host "  平均值: $($stats.NATSGoroutineLeakAvg)"
Write-Host "  有泄漏的运行: $($stats.NATSGoroutineLeakRuns)/$Count" -ForegroundColor $(if ($stats.NATSGoroutineLeakRuns -eq 0) { "Green" } else { "Red" })
Write-Host ""
Write-Host "Kafka 成功率:" -ForegroundColor Yellow
Write-Host "  最小值: $($stats.KafkaSuccessRateMin)%"
Write-Host "  最大值: $($stats.KafkaSuccessRateMax)%"
Write-Host "  平均值: $($stats.KafkaSuccessRateAvg)%" -ForegroundColor $(if ($stats.KafkaSuccessRateAvg -ge 99) { "Green" } else { "Yellow" })
Write-Host ""
Write-Host "NATS 成功率:" -ForegroundColor Yellow
Write-Host "  最小值: $($stats.NATSSuccessRateMin)%"
Write-Host "  最大值: $($stats.NATSSuccessRateMax)%"
Write-Host "  平均值: $($stats.NATSSuccessRateAvg)%" -ForegroundColor $(if ($stats.NATSSuccessRateAvg -ge 90) { "Green" } else { "Red" })
Write-Host ""

# 保存详细报告
$reportFile = "$resultDir\ANALYSIS_REPORT.md"
@"
# $Count 次连续测试详细分析报告

测试时间: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')

## 总体统计

- 总测试次数: $Count
- 通过次数: $passCount
- 失败次数: $failCount
- 成功率: $successRate%

## Kafka 指标统计

### 顺序违反
- 最小值: $($stats.KafkaOrderViolationsMin)
- 最大值: $($stats.KafkaOrderViolationsMax)
- 平均值: $($stats.KafkaOrderViolationsAvg)
- 有违反的运行: $($stats.KafkaOrderViolationRuns)/$Count

### 协程泄漏
- 最小值: $($stats.KafkaGoroutineLeakMin)
- 最大值: $($stats.KafkaGoroutineLeakMax)
- 平均值: $($stats.KafkaGoroutineLeakAvg)
- 有泄漏的运行: $($stats.KafkaGoroutineLeakRuns)/$Count

### 成功率
- 最小值: $($stats.KafkaSuccessRateMin)%
- 最大值: $($stats.KafkaSuccessRateMax)%
- 平均值: $($stats.KafkaSuccessRateAvg)%

## NATS 指标统计

### 顺序违反
- 最小值: $($stats.NATSOrderViolationsMin)
- 最大值: $($stats.NATSOrderViolationsMax)
- 平均值: $($stats.NATSOrderViolationsAvg)
- 有违反的运行: $($stats.NATSOrderViolationRuns)/$Count

### 协程泄漏
- 最小值: $($stats.NATSGoroutineLeakMin)
- 最大值: $($stats.NATSGoroutineLeakMax)
- 平均值: $($stats.NATSGoroutineLeakAvg)
- 有泄漏的运行: $($stats.NATSGoroutineLeakRuns)/$Count

### 成功率
- 最小值: $($stats.NATSSuccessRateMin)%
- 最大值: $($stats.NATSSuccessRateMax)%
- 平均值: $($stats.NATSSuccessRateAvg)%

## 详细测试结果

| 运行 | 状态 | Kafka顺序违反 | NATS顺序违反 | Kafka协程泄漏 | NATS协程泄漏 | Kafka成功率 | NATS成功率 |
|------|------|--------------|-------------|--------------|-------------|------------|-----------|
"@ | Out-File $reportFile

foreach ($m in $allMetrics) {
    $status = if ($m.Passed) { "✅" } else { "❌" }
    "| $($m.RunNumber) | $status | $($m.KafkaOrderViolations) | $($m.NATSOrderViolations) | $($m.KafkaGoroutineLeak) | $($m.NATSGoroutineLeak) | $($m.KafkaSuccessRate)% | $($m.NATSSuccessRate)% |" | Out-File $reportFile -Append
}

Write-Host "结果保存在: $resultDir" -ForegroundColor Cyan
Write-Host "详细报告: $reportFile" -ForegroundColor Cyan

