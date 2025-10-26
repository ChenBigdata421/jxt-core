#!/bin/bash

# 修复 SetPayload 调用
sed -i '233s/event.SetPayload(payload)/event.SetPayload(createTestDomainEvent("UserCreated", "aggregate-1", "User", payload))/' event_test.go

# 修复 json.RawMessage
sed -i 's/json\.RawMessage/jxtjson.RawMessage/g' event_test.go

# 修复剩余的 NewOutboxEvent 调用
sed -i '674s/map\[string\]string{"name": "test"}/createTestDomainEvent("UserCreated", "aggregate-1", "User", map[string]interface{}{"name": "test"})/' event_test.go
sed -i '703s/map\[string\]string{"name": "test"}/createTestDomainEvent("ArchiveCreated", "aggregate-123", "Archive", map[string]interface{}{"name": "test"})/' event_test.go
sed -i '731s/map\[string\]string{"name": "test"}/createTestDomainEvent("ArchiveCreated", "aggregate-123", "Archive", map[string]interface{}{"name": "test"})/' event_test.go
sed -i '751s/map\[string\]string{"name": "test"}/createTestDomainEvent("UserCreated", "aggregate-1", "User", map[string]interface{}{"name": "test"})/' event_test.go
sed -i '777s/map\[string\]string{"name": "test"}/createTestDomainEvent("UserCreated", "aggregate-1", "User", map[string]interface{}{"name": "test"})/' event_test.go

# 添加 jxtjson 导入
sed -i '/jxtevent "github.com\/ChenBigdata421\/jxt-core\/sdk\/pkg\/domain\/event"/a\jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"' event_test.go

# 删除 encoding/json 导入
sed -i '/^[[:space:]]*"encoding\/json"$/d' event_test.go

echo "修复完成！"
