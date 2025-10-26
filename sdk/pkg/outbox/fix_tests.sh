#!/bin/bash

# 修复 outbox 测试文件的脚本
# 将所有使用 map[string]string 的 NewOutboxEvent 调用改为使用 createTestDomainEvent

cd "$(dirname "$0")"

# 备份原文件
cp event_test.go event_test.go.bak

# 修复模式：NewOutboxEvent("tenant-1", "agg-1", "User", "UserCreated", map[string]string{"name": "test"})
# 改为：NewOutboxEvent("tenant-1", "agg-1", "User", "UserCreated", createTestDomainEvent("UserCreated", "agg-1", "User", map[string]interface{}{"name": "test"}))

sed -i 's/NewOutboxEvent("\([^"]*\)", "\([^"]*\)", "\([^"]*\)", "\([^"]*\)", map\[string\]string{"name": "test"})/NewOutboxEvent("\1", "\2", "\3", "\4", createTestDomainEvent("\4", "\2", "\3", map[string]interface{}{"name": "test"}))/g' event_test.go

echo "修复完成！"
echo "原文件备份为: event_test.go.bak"
echo "请运行测试验证: go test -v ./..."

