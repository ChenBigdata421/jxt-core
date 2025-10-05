package eventbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultMessageFormatter_FormatMessage 测试默认消息格式化器
func TestDefaultMessageFormatter_FormatMessage(t *testing.T) {
	formatter := &DefaultMessageFormatter{}

	msg, err := formatter.FormatMessage("uuid-123", "agg-456", []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "uuid-123", msg.UUID)
	assert.Equal(t, []byte("payload"), msg.Payload)
	assert.Equal(t, "agg-456", msg.Metadata["aggregateID"])
	assert.NotEmpty(t, msg.Metadata["timestamp"])
}

// TestDefaultMessageFormatter_ExtractAggregateID 测试提取聚合ID
func TestDefaultMessageFormatter_ExtractAggregateID(t *testing.T) {
	formatter := &DefaultMessageFormatter{}

	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"nil", nil, ""},
		{"string", "test-id", "test-id"},
		{"int", 123, "123"},
		{"int32", int32(456), "456"},
		{"int64", int64(789), "789"},
		{"uint", uint(111), "111"},
		{"uint32", uint32(222), "222"},
		{"uint64", uint64(333), "333"},
		{"other", struct{ ID int }{ID: 999}, "{999}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.ExtractAggregateID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDefaultMessageFormatter_SetMetadata 测试设置元数据
func TestDefaultMessageFormatter_SetMetadata(t *testing.T) {
	formatter := &DefaultMessageFormatter{}
	msg := NewMessage("uuid", []byte("payload"))

	metadata := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	err := formatter.SetMetadata(msg, metadata)
	require.NoError(t, err)
	assert.Equal(t, "value1", msg.Metadata["key1"])
	assert.Equal(t, "value2", msg.Metadata["key2"])
}

// TestEvidenceMessageFormatter_FormatMessage 测试业务消息格式化器
func TestEvidenceMessageFormatter_FormatMessage(t *testing.T) {
	formatter := &EvidenceMessageFormatter{}

	msg, err := formatter.FormatMessage("uuid-123", "evidence-456", []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "evidence-456", msg.Metadata["aggregate_id"])
	assert.Equal(t, "evidence-456", msg.Metadata["aggregateID"])
}

// TestJSONMessageFormatter_FormatMessage 测试JSON消息格式化器
func TestJSONMessageFormatter_FormatMessage(t *testing.T) {
	formatter := &JSONMessageFormatter{IncludeHeaders: true}

	msg, err := formatter.FormatMessage("uuid-123", "agg-456", []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "application/json", msg.Metadata["contentType"])
	assert.Equal(t, "utf-8", msg.Metadata["encoding"])
	assert.Equal(t, "json", msg.Metadata["messageFormat"])
	assert.Equal(t, "1.0", msg.Metadata["schemaVersion"])
}

// TestJSONMessageFormatter_WithoutHeaders 测试不包含头部的JSON格式化器
func TestJSONMessageFormatter_WithoutHeaders(t *testing.T) {
	formatter := &JSONMessageFormatter{IncludeHeaders: false}

	msg, err := formatter.FormatMessage("uuid-123", "agg-456", []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "application/json", msg.Metadata["contentType"])
	assert.Equal(t, "utf-8", msg.Metadata["encoding"])
	assert.Empty(t, msg.Metadata["messageFormat"])
	assert.Empty(t, msg.Metadata["schemaVersion"])
}

// TestProtobufMessageFormatter_FormatMessage 测试Protobuf消息格式化器
func TestProtobufMessageFormatter_FormatMessage(t *testing.T) {
	formatter := &ProtobufMessageFormatter{SchemaRegistry: "http://registry:8081"}

	msg, err := formatter.FormatMessage("uuid-123", "agg-456", []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "application/x-protobuf", msg.Metadata["contentType"])
	assert.Equal(t, "protobuf", msg.Metadata["messageFormat"])
	assert.Equal(t, "http://registry:8081", msg.Metadata["schemaRegistry"])
}

// TestProtobufMessageFormatter_WithoutRegistry 测试没有注册中心的Protobuf格式化器
func TestProtobufMessageFormatter_WithoutRegistry(t *testing.T) {
	formatter := &ProtobufMessageFormatter{}

	msg, err := formatter.FormatMessage("uuid-123", "agg-456", []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Empty(t, msg.Metadata["schemaRegistry"])
}

// TestAvroMessageFormatter_FormatMessage 测试Avro消息格式化器
func TestAvroMessageFormatter_FormatMessage(t *testing.T) {
	formatter := &AvroMessageFormatter{
		SchemaID:       "schema-123",
		SchemaRegistry: "http://registry:8081",
	}

	msg, err := formatter.FormatMessage("uuid-123", "agg-456", []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "application/avro", msg.Metadata["contentType"])
	assert.Equal(t, "avro", msg.Metadata["messageFormat"])
	assert.Equal(t, "schema-123", msg.Metadata["schemaId"])
	assert.Equal(t, "http://registry:8081", msg.Metadata["schemaRegistry"])
}

// TestCloudEventMessageFormatter_FormatMessage 测试CloudEvents消息格式化器
func TestCloudEventMessageFormatter_FormatMessage(t *testing.T) {
	formatter := &CloudEventMessageFormatter{
		Source:      "test-source",
		EventType:   "test.event",
		SpecVersion: "1.0",
	}

	msg, err := formatter.FormatMessage("uuid-123", "agg-456", []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "1.0", msg.Metadata["ce-specversion"])
	assert.Equal(t, "test.event", msg.Metadata["ce-type"])
	assert.Equal(t, "test-source", msg.Metadata["ce-source"])
	assert.Equal(t, "uuid-123", msg.Metadata["ce-id"])
	assert.Equal(t, "agg-456", msg.Metadata["ce-subject"])
	assert.NotEmpty(t, msg.Metadata["ce-time"])
}

// TestCloudEventMessageFormatter_Defaults 测试CloudEvents默认值
func TestCloudEventMessageFormatter_Defaults(t *testing.T) {
	formatter := &CloudEventMessageFormatter{}

	msg, err := formatter.FormatMessage("uuid-123", nil, []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "1.0", msg.Metadata["ce-specversion"])
	assert.Equal(t, "com.jxt.event", msg.Metadata["ce-type"])
	assert.Equal(t, "jxt-core", msg.Metadata["ce-source"])
	assert.Empty(t, msg.Metadata["ce-subject"])
}

// TestMessageFormatterChain_FormatMessage 测试消息格式化器链
func TestMessageFormatterChain_FormatMessage(t *testing.T) {
	chain := NewMessageFormatterChain(
		&DefaultMessageFormatter{},
		&JSONMessageFormatter{IncludeHeaders: true},
	)

	msg, err := chain.FormatMessage("uuid-123", "agg-456", []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "agg-456", msg.Metadata["aggregateID"])
	assert.Equal(t, "application/json", msg.Metadata["contentType"])
}

// TestMessageFormatterChain_Empty 测试空格式化器链
func TestMessageFormatterChain_Empty(t *testing.T) {
	chain := NewMessageFormatterChain()

	msg, err := chain.FormatMessage("uuid-123", "agg-456", []byte("payload"))
	require.NoError(t, err)
	assert.NotNil(t, msg)
	assert.Equal(t, "uuid-123", msg.UUID)
}

// TestMessageFormatterChain_ExtractAggregateID 测试链提取聚合ID
func TestMessageFormatterChain_ExtractAggregateID(t *testing.T) {
	chain := NewMessageFormatterChain(&DefaultMessageFormatter{})
	result := chain.ExtractAggregateID("test-id")
	assert.Equal(t, "test-id", result)

	emptyChain := NewMessageFormatterChain()
	result = emptyChain.ExtractAggregateID("test-id")
	assert.Equal(t, "", result)
}

// TestMessageFormatterChain_SetMetadata 测试链设置元数据
func TestMessageFormatterChain_SetMetadata(t *testing.T) {
	chain := NewMessageFormatterChain(&DefaultMessageFormatter{})
	msg := NewMessage("uuid", []byte("payload"))

	err := chain.SetMetadata(msg, map[string]string{"key": "value"})
	require.NoError(t, err)
	assert.Equal(t, "value", msg.Metadata["key"])

	emptyChain := NewMessageFormatterChain()
	err = emptyChain.SetMetadata(msg, map[string]string{"key": "value"})
	assert.Error(t, err)
}

// TestMessageFormatterRegistry_NewRegistry 测试创建注册表
func TestMessageFormatterRegistry_NewRegistry(t *testing.T) {
	registry := NewMessageFormatterRegistry()
	assert.NotNil(t, registry)

	// 检查默认注册的格式化器
	names := registry.List()
	assert.Contains(t, names, "default")
	assert.Contains(t, names, "evidence")
	assert.Contains(t, names, "json")
	assert.Contains(t, names, "protobuf")
	assert.Contains(t, names, "avro")
	assert.Contains(t, names, "cloudevents")
}

// TestMessageFormatterRegistry_Register 测试注册格式化器
func TestMessageFormatterRegistry_Register(t *testing.T) {
	registry := NewMessageFormatterRegistry()
	custom := &DefaultMessageFormatter{}

	registry.Register("custom", custom)

	formatter, exists := registry.Get("custom")
	assert.True(t, exists)
	assert.NotNil(t, formatter)
}

// TestMessageFormatterRegistry_Get 测试获取格式化器
func TestMessageFormatterRegistry_Get(t *testing.T) {
	registry := NewMessageFormatterRegistry()

	formatter, exists := registry.Get("default")
	assert.True(t, exists)
	assert.NotNil(t, formatter)

	formatter, exists = registry.Get("non-existent")
	assert.False(t, exists)
	assert.Nil(t, formatter)
}

// TestMessageFormatterRegistry_GetOrDefault 测试获取或默认
func TestMessageFormatterRegistry_GetOrDefault(t *testing.T) {
	registry := NewMessageFormatterRegistry()

	formatter := registry.GetOrDefault("default")
	assert.NotNil(t, formatter)

	formatter = registry.GetOrDefault("non-existent")
	assert.NotNil(t, formatter)
	assert.IsType(t, &DefaultMessageFormatter{}, formatter)
}

// TestGlobalFormatterRegistry 测试全局注册表
func TestGlobalFormatterRegistry(t *testing.T) {
	assert.NotNil(t, GlobalFormatterRegistry)

	formatter, exists := GlobalFormatterRegistry.Get("default")
	assert.True(t, exists)
	assert.NotNil(t, formatter)
}

