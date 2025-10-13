@echo off
REM 验证测试文件是否可以编译

echo 🔍 验证测试文件...
echo.

REM 编译测试文件
echo 📦 编译测试文件...
go build kafka_nats_comparison_test.go

if %ERRORLEVEL% equ 0 (
    echo ✅ 测试文件编译成功！
    
    REM 清理编译产物
    if exist kafka_nats_comparison_test.exe del kafka_nats_comparison_test.exe
    
    echo.
    echo 🎉 验证完成！测试文件可以正常编译。
    echo.
    echo 📝 下一步：
    echo    1. 启动 Kafka: docker-compose up -d kafka
    echo    2. 启动 NATS: docker-compose up -d nats
    echo    3. 运行测试: run_comparison_test.bat
    echo.
    
    exit /b 0
) else (
    echo ❌ 测试文件编译失败！
    exit /b 1
)

