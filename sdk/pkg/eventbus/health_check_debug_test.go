package eventbus

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// TestHealthCheckMessageDebug 调试健康检查消息问题
func TestHealthCheckMessageDebug(t *testing.T) {
	// 初始化logger
	if logger.DefaultLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		logger.Logger = zapLogger
		logger.DefaultLogger = zapLogger.Sugar()
	}

	// 测试1: 使用标准库JSON
	t.Run("StandardJSON", func(t *testing.T) {
		msg := &HealthCheckMessage{
			MessageID:    "test-123",
			Timestamp:    time.Now(),
			Source:       "test-service",
			EventBusType: "memory",
			Version:      "1.0.0",
			Metadata:     make(map[string]string),
		}

		msg.Metadata["testKey"] = "testValue"

		// 使用标准库序列化
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Standard JSON marshal failed: %v", err)
		}

		t.Logf("Standard JSON serialization successful, size: %d bytes", len(data))
	})

	// 测试2: 使用jsoniter
	t.Run("JsoniterJSON", func(t *testing.T) {
		msg := &HealthCheckMessage{
			MessageID:    "test-123",
			Timestamp:    time.Now(),
			Source:       "test-service",
			EventBusType: "memory",
			Version:      "1.0.0",
			Metadata:     make(map[string]string),
		}

		msg.Metadata["testKey"] = "testValue"

		// 使用jsoniter序列化
		data, err := Marshal(msg)
		if err != nil {
			t.Fatalf("Jsoniter marshal failed: %v", err)
		}

		t.Logf("Jsoniter serialization successful, size: %d bytes", len(data))
	})

	// 测试3: 测试CreateHealthCheckMessage
	t.Run("CreateHealthCheckMessage", func(t *testing.T) {
		msg := CreateHealthCheckMessage("test-service", "memory")
		
		t.Logf("Created message: %+v", msg)
		t.Logf("Metadata: %+v", msg.Metadata)

		// 直接序列化，不添加额外元数据
		data, err := Marshal(msg)
		if err != nil {
			t.Fatalf("Failed to marshal created message: %v", err)
		}

		t.Logf("Created message serialization successful, size: %d bytes", len(data))
	})

	// 测试4: 测试SetMetadata
	t.Run("SetMetadata", func(t *testing.T) {
		msg := CreateHealthCheckMessage("test-service", "memory")
		
		t.Logf("Before SetMetadata: %+v", msg.Metadata)

		// 添加元数据
		msg.SetMetadata("testKey", "testValue")
		
		t.Logf("After SetMetadata: %+v", msg.Metadata)

		// 序列化
		data, err := Marshal(msg)
		if err != nil {
			t.Fatalf("Failed to marshal message with metadata: %v", err)
		}

		t.Logf("Message with metadata serialization successful, size: %d bytes", len(data))
	})

	// 测试5: 测试Builder
	t.Run("MessageBuilder", func(t *testing.T) {
		builder := NewHealthCheckMessageBuilder("test-service", "memory")
		
		t.Logf("Builder created")

		msg := builder.Build()
		
		t.Logf("Builder message: %+v", msg)
		t.Logf("Builder metadata: %+v", msg.Metadata)

		// 序列化
		data, err := Marshal(msg)
		if err != nil {
			t.Fatalf("Failed to marshal builder message: %v", err)
		}

		t.Logf("Builder message serialization successful, size: %d bytes", len(data))
	})

	// 测试6: 测试Builder with metadata
	t.Run("BuilderWithMetadata", func(t *testing.T) {
		builder := NewHealthCheckMessageBuilder("test-service", "memory")
		
		msg := builder.
			WithCheckType("periodic").
			WithInstanceID("test-instance").
			Build()
		
		t.Logf("Builder with metadata message: %+v", msg)
		t.Logf("Builder with metadata: %+v", msg.Metadata)

		// 序列化
		data, err := Marshal(msg)
		if err != nil {
			t.Fatalf("Failed to marshal builder message with metadata: %v", err)
		}

		t.Logf("Builder message with metadata serialization successful, size: %d bytes", len(data))
	})
}

// TestMapSerialization 测试map序列化问题
func TestMapSerialization(t *testing.T) {
	// 测试1: 空map
	t.Run("EmptyMap", func(t *testing.T) {
		m := make(map[string]string)
		data, err := Marshal(m)
		if err != nil {
			t.Fatalf("Failed to marshal empty map: %v", err)
		}
		t.Logf("Empty map serialization successful: %s", string(data))
	})

	// 测试2: 有数据的map
	t.Run("MapWithData", func(t *testing.T) {
		m := make(map[string]string)
		m["key1"] = "value1"
		m["key2"] = "value2"
		
		data, err := Marshal(m)
		if err != nil {
			t.Fatalf("Failed to marshal map with data: %v", err)
		}
		t.Logf("Map with data serialization successful: %s", string(data))
	})

	// 测试3: nil map
	t.Run("NilMap", func(t *testing.T) {
		var m map[string]string
		data, err := Marshal(m)
		if err != nil {
			t.Fatalf("Failed to marshal nil map: %v", err)
		}
		t.Logf("Nil map serialization successful: %s", string(data))
	})
}

// TestStructWithMap 测试包含map的结构体
func TestStructWithMap(t *testing.T) {
	type TestStruct struct {
		Name     string            `json:"name"`
		Metadata map[string]string `json:"metadata"`
	}

	// 测试1: 初始化的map
	t.Run("InitializedMap", func(t *testing.T) {
		s := TestStruct{
			Name:     "test",
			Metadata: make(map[string]string),
		}
		s.Metadata["key"] = "value"

		data, err := Marshal(s)
		if err != nil {
			t.Fatalf("Failed to marshal struct with initialized map: %v", err)
		}
		t.Logf("Struct with initialized map serialization successful: %s", string(data))
	})

	// 测试2: nil map
	t.Run("NilMapInStruct", func(t *testing.T) {
		s := TestStruct{
			Name:     "test",
			Metadata: nil,
		}

		data, err := Marshal(s)
		if err != nil {
			t.Fatalf("Failed to marshal struct with nil map: %v", err)
		}
		t.Logf("Struct with nil map serialization successful: %s", string(data))
	})
}
