@echo off
setlocal enabledelayedexpansion

REM 连续运行 20 次性能测试脚本

echo ==========================================
echo 开始连续运行 20 次性能测试
echo 测试文件: kafka_nats_comparison_test.go
echo 开始时间: %date% %time%
echo ==========================================

REM 创建结果目录
set RESULT_DIR=test_results_%date:~0,4%%date:~5,2%%date:~8,2%_%time:~0,2%%time:~3,2%%time:~6,2%
set RESULT_DIR=%RESULT_DIR: =0%
mkdir "%RESULT_DIR%"

REM 统计变量
set PASS_COUNT=0
set FAIL_COUNT=0

REM 运行 20 次测试
for /l %%i in (1,1,20) do (
    echo.
    echo ==========================================
    echo 第 %%i 次测试 ^(%date% %time%^)
    echo ==========================================
    
    REM 运行测试并保存结果
    set LOG_FILE=%RESULT_DIR%\test_run_%%i.log
    
    go test -v -run "TestKafkaVsNATSPerformanceComparison" -timeout 30m > "!LOG_FILE!" 2>&1
    
    if !errorlevel! equ 0 (
        echo ✅ 第 %%i 次测试: PASS
        set /a PASS_COUNT+=1
    ) else (
        echo ❌ 第 %%i 次测试: FAIL
        set /a FAIL_COUNT+=1
    )
    
    REM 提取关键指标
    echo 提取关键指标...
    findstr /C:"协程泄漏" /C:"顺序违反" /C:"成功率" /C:"吞吐量" /C:"延迟" /C:"内存" "!LOG_FILE!" > "%RESULT_DIR%\metrics_%%i.txt"
    
    REM 等待一段时间，让系统清理资源
    if %%i lss 20 (
        echo 等待 10 秒，清理资源...
        timeout /t 10 /nobreak > nul
    )
)

echo.
echo ==========================================
echo 测试完成！
echo 结束时间: %date% %time%
echo ==========================================
echo 总测试次数: 20
echo 通过次数: %PASS_COUNT%
echo 失败次数: %FAIL_COUNT%
set /a SUCCESS_RATE=%PASS_COUNT% * 100 / 20
echo 成功率: !SUCCESS_RATE!%%
echo.
echo 结果保存在: %RESULT_DIR%
echo ==========================================

REM 生成汇总报告
echo 生成汇总报告...
(
echo 性能测试 20 次运行汇总报告
echo ========================================
echo 测试时间: %date% %time%
echo 测试文件: kafka_nats_comparison_test.go
echo 总测试次数: 20
echo 通过次数: %PASS_COUNT%
echo 失败次数: %FAIL_COUNT%
echo 成功率: !SUCCESS_RATE!%%
echo ========================================
echo.
echo 详细结果请查看各个日志文件。
) > "%RESULT_DIR%\summary.txt"

echo 汇总报告已生成: %RESULT_DIR%\summary.txt

pause

