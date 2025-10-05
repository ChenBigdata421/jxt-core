#!/bin/bash

# 运行测试并生成覆盖率报告，跳过长时间运行的测试

echo "Running tests with coverage..."

# 跳过以下测试：
# - TestHealthCheck* (所有健康检查测试)
# - TestProductionReadiness (生产就绪测试)
# - TestKafka* (Kafka 集成测试)
# - TestNATS* (NATS 集成测试)
# - *Integration* (所有集成测试)

timeout 60 go test -coverprofile=coverage_round14.out -covermode=atomic \
  -skip="TestHealthCheck|TestProductionReadiness|TestKafka|TestNATS|Integration" \
  . 2>&1 | tee test_output.log

# 检查测试是否成功
if [ $? -eq 0 ]; then
  echo ""
  echo "Tests completed successfully!"
  echo ""
  echo "Coverage report:"
  go tool cover -func=coverage_round14.out | grep "^total:"
  echo ""
  echo "Generating HTML coverage report..."
  go tool cover -html=coverage_round14.out -o coverage_round14.html
  echo "HTML report generated: coverage_round14.html"
else
  echo ""
  echo "Tests failed or timed out!"
  echo "Check test_output.log for details"
fi

