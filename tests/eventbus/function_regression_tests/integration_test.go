package function_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 端到端集成测试 (来自 e2e_integration_test.go)
// ============================================================================

// TestE2E_MemoryEventBus_WithEnvelope 测试 Memory eventbus.EventBus 端到端流程（使用 eventbus.Envelope）
func TestE2E_MemoryEventBus_WithEnvelope(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateMemoryEventBus()
	defer helper.CloseEventBus(bus)

	ctx := context.Background()
	topic := "orders.events"

	// 订阅 eventbus.Envelope 消息
	var received int64
	var receivedEnvelopes []*eventbus.Envelope
	var mu sync.Mutex

	handler := func(ctx context.Context, env *eventbus.Envelope) error {
		mu.Lock()
		receivedEnvelopes = append(receivedEnvelopes, env)
		mu.Unlock()
		atomic.AddInt64(&received, 1)
		return nil
	}

	err := bus.SubscribeEnvelope(ctx, topic, handler)
	helper.AssertNoError(err, "SubscribeEnvelope should not return error")

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布多个 eventbus.Envelope 消息
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		envelope := eventbus.NewEnvelope(
			fmt.Sprintf("event-%d", i+1), // eventID
			"order-123",                  // aggregateID
			"OrderCreated",               // eventType
			int64(i+1),                   // eventVersion
			[]byte(`{"orderId":"order-123","amount":100}`), // payload
		)

		data, err := json.Marshal(envelope)
		helper.AssertNoError(err, "Marshal should not return error")

		err = bus.Publish(ctx, topic, data)
		helper.AssertNoError(err, "Publish should not return error")
	}

	// 等待所有消息被接收
	success := helper.WaitForMessages(&received, int64(messageCount), 1*time.Second)
	helper.AssertTrue(success, "Should receive all messages within timeout")

	// 验证接收
	helper.AssertEqual(int64(messageCount), atomic.LoadInt64(&received), "Should receive all messages")
	helper.AssertEqual(messageCount, len(receivedEnvelopes), "Should have all envelopes")

	// 验证所有消息都被接收（不验证顺序，因为是并发处理）
	versions := make(map[int64]bool)
	for _, env := range receivedEnvelopes {
		assert.Equal(t, "order-123", env.AggregateID)
		assert.Equal(t, "OrderCreated", env.EventType)
		versions[env.EventVersion] = true
	}

	// 验证所有版本都存在
	for i := 1; i <= messageCount; i++ {
		assert.True(t, versions[int64(i)], "Version %d should exist", i)
	}
}

// TestE2E_MemoryEventBus_MultipleTopics 测试多主题端到端流程
func TestE2E_MemoryEventBus_MultipleTopics(t *testing.T) {
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}

	bus, err := eventbus.NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()

	// 订阅多个主题
	topics := []string{"topic1", "topic2", "topic3"}
	counters := make(map[string]*atomic.Int32)
	var mu sync.Mutex

	for _, topic := range topics {
		counter := &atomic.Int32{}
		counters[topic] = counter

		handler := func(t string, c *atomic.Int32) eventbus.MessageHandler {
			return func(ctx context.Context, data []byte) error {
				c.Add(1)
				return nil
			}
		}(topic, counter)

		err = bus.Subscribe(ctx, topic, handler)
		require.NoError(t, err)
	}

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 向每个主题发布消息
	messagesPerTopic := 3
	for _, topic := range topics {
		for i := 0; i < messagesPerTopic; i++ {
			message := []byte("message " + string(rune('0'+i)))
			err = bus.Publish(ctx, topic, message)
			require.NoError(t, err)
		}
	}

	// 等待所有消息被接收
	time.Sleep(500 * time.Millisecond)

	// 验证每个主题都收到了正确数量的消息
	mu.Lock()
	defer mu.Unlock()
	for topic, counter := range counters {
		assert.Equal(t, int32(messagesPerTopic), counter.Load(), "Topic: %s", topic)
	}
}

// TestE2E_MemoryEventBus_ConcurrentPublishSubscribe 测试并发发布订阅
func TestE2E_MemoryEventBus_ConcurrentPublishSubscribe(t *testing.T) {
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}

	bus, err := eventbus.NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "concurrent.test"

	// 订阅消息
	var received atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		received.Add(1)
		// 模拟处理时间
		time.Sleep(1 * time.Millisecond)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 并发发布消息
	goroutines := 10
	messagesPerGoroutine := 10
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				message := []byte("message")
				err := bus.Publish(ctx, topic, message)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// 等待所有消息被接收
	time.Sleep(2 * time.Second)

	// 验证接收到所有消息
	expectedCount := int32(goroutines * messagesPerGoroutine)
	assert.Equal(t, expectedCount, received.Load())
}

// TestE2E_MemoryEventBus_ErrorRecovery 测试错误恢复
func TestE2E_MemoryEventBus_ErrorRecovery(t *testing.T) {
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}

	bus, err := eventbus.NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "error.recovery"

	// 订阅消息 - 前几次返回错误，然后成功
	var attemptCount atomic.Int32
	var successCount atomic.Int32

	handler := func(ctx context.Context, data []byte) error {
		count := attemptCount.Add(1)
		if count <= 3 {
			// 前3次返回错误
			return assert.AnError
		}
		// 之后成功
		successCount.Add(1)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		message := []byte("message " + string(rune('0'+i)))
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 等待所有消息被处理
	time.Sleep(500 * time.Millisecond)

	// 验证处理器被调用
	assert.Greater(t, attemptCount.Load(), int32(0))
	// 验证有成功的处理
	assert.Greater(t, successCount.Load(), int32(0))
}

// TestE2E_MemoryEventBus_ContextCancellation 测试上下文取消
func TestE2E_MemoryEventBus_ContextCancellation(t *testing.T) {
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}

	bus, err := eventbus.NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	topic := "context.cancel"

	// 订阅消息
	var received atomic.Int32
	handler := func(ctx context.Context, data []byte) error {
		received.Add(1)
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布一些消息
	for i := 0; i < 3; i++ {
		message := []byte("message " + string(rune('0'+i)))
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 等待消息被接收
	time.Sleep(200 * time.Millisecond)

	// 取消上下文
	cancel()

	// 尝试发布更多消息（应该失败或被取消）
	message := []byte("message after cancel")
	err = bus.Publish(ctx, topic, message)
	// 上下文已取消，发布可能失败
	if err != nil {
		assert.Contains(t, err.Error(), "context")
	}

	// 验证之前的消息被接收
	assert.Greater(t, received.Load(), int32(0))
}

// TestE2E_MemoryEventBus_Metrics 测试指标收集
func TestE2E_MemoryEventBus_Metrics(t *testing.T) {
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}

	bus, err := eventbus.NewEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	topic := "metrics.test"

	// 订阅消息
	handler := func(ctx context.Context, data []byte) error {
		return nil
	}

	err = bus.Subscribe(ctx, topic, handler)
	require.NoError(t, err)

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 发布消息
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		message := []byte("message " + string(rune('0'+i)))
		err = bus.Publish(ctx, topic, message)
		require.NoError(t, err)
	}

	// 等待消息被处理
	time.Sleep(500 * time.Millisecond)

	// 获取指标
	metrics := bus.GetMetrics()
	require.NotNil(t, metrics)

	// 验证指标
	assert.Greater(t, metrics.MessagesPublished, int64(0))
	assert.Greater(t, metrics.MessagesConsumed, int64(0))
}

// ============================================================================
// JSON 配置测试 (来自 json_config_test.go)
// ============================================================================

// TestMarshalToString 测试序列化为字符串
func TestMarshalToString(t *testing.T) {
	// 使用简单的结构体避免 jsoniter 的 map 序列化问题
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	str, err := eventbus.MarshalToString(data)
	require.NoError(t, err)
	assert.Contains(t, str, "test")
	assert.Contains(t, str, "30")
}

// TestUnmarshalFromString 测试从字符串反序列化
func TestUnmarshalFromString(t *testing.T) {
	str := `{"name":"test","age":30}`

	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	var result TestData
	err := eventbus.UnmarshalFromString(str, &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 30, result.Age)
}

// TestMarshal 测试序列化
func TestMarshal(t *testing.T) {
	data := map[string]interface{}{
		"name": "test",
		"age":  30,
	}

	bytes, err := eventbus.Marshal(data)
	require.NoError(t, err)
	assert.NotEmpty(t, bytes)
	assert.Contains(t, string(bytes), "test")
}

// TestUnmarshal 测试反序列化
func TestUnmarshal(t *testing.T) {
	bytes := []byte(`{"name":"test","age":30}`)

	var result map[string]interface{}
	err := eventbus.Unmarshal(bytes, &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["name"])
	assert.Equal(t, float64(30), result["age"])
}

// TestMarshalFast 测试快速序列化
func TestMarshalFast(t *testing.T) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	bytes, err := eventbus.MarshalFast(data)
	require.NoError(t, err)
	assert.NotEmpty(t, bytes)
}

// TestUnmarshalFast 测试快速反序列化
func TestUnmarshalFast(t *testing.T) {
	bytes := []byte(`{"name":"test","age":30}`)

	var result map[string]interface{}
	err := eventbus.UnmarshalFast(bytes, &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["name"])
}

// TestJSON_RoundTrip 测试完整的序列化反序列化流程
func TestJSON_RoundTrip(t *testing.T) {
	original := map[string]interface{}{
		"string": "value",
		"number": 42,
		"bool":   true,
		"array":  []int{1, 2, 3},
		"nested": map[string]string{
			"key": "value",
		},
	}

	// 序列化
	bytes, err := eventbus.Marshal(original)
	require.NoError(t, err)

	// 反序列化
	var result map[string]interface{}
	err = eventbus.Unmarshal(bytes, &result)
	require.NoError(t, err)

	// 验证
	assert.Equal(t, "value", result["string"])
	assert.Equal(t, float64(42), result["number"])
	assert.Equal(t, true, result["bool"])
}

// TestJSONFast_RoundTrip 测试快速序列化反序列化流程
func TestJSONFast_RoundTrip(t *testing.T) {
	type TestData struct {
		String string `json:"string"`
		Number int    `json:"number"`
	}
	original := TestData{String: "value", Number: 42}

	// 快速序列化
	bytes, err := eventbus.MarshalFast(original)
	require.NoError(t, err)

	// 快速反序列化
	var result TestData
	err = eventbus.UnmarshalFast(bytes, &result)
	require.NoError(t, err)

	// 验证
	assert.Equal(t, "value", result.String)
	assert.Equal(t, 42, result.Number)
}

// TestMarshalToString_Error 测试序列化错误
func TestMarshalToString_Error(t *testing.T) {
	// 创建一个无法序列化的对象（channel 类型）
	invalidData := make(chan int)

	_, err := eventbus.MarshalToString(invalidData)
	assert.Error(t, err)
}

// TestUnmarshalFromString_Error 测试反序列化错误
func TestUnmarshalFromString_Error(t *testing.T) {
	invalidJSON := `{"name": invalid}`

	var result map[string]interface{}
	err := eventbus.UnmarshalFromString(invalidJSON, &result)
	assert.Error(t, err)
}

// TestMarshal_Struct 测试序列化结构体
func TestMarshal_Struct(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	person := Person{Name: "Alice", Age: 30}

	bytes, err := eventbus.Marshal(person)
	require.NoError(t, err)
	assert.Contains(t, string(bytes), "Alice")
	assert.Contains(t, string(bytes), "30")
}

// TestUnmarshal_Struct 测试反序列化到结构体
func TestUnmarshal_Struct(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	bytes := []byte(`{"name":"Bob","age":25}`)

	var person Person
	err := eventbus.Unmarshal(bytes, &person)
	require.NoError(t, err)
	assert.Equal(t, "Bob", person.Name)
	assert.Equal(t, 25, person.Age)
}

// TestJSON_Variables 测试JSON变量
func TestJSON_Variables(t *testing.T) {
	assert.NotNil(t, eventbus.JSON)
	assert.NotNil(t, eventbus.JSONFast)
	assert.NotNil(t, eventbus.JSONDefault)
}

// TestRawMessage 测试RawMessage类型
func TestRawMessage(t *testing.T) {
	type Message struct {
		Type string              `json:"type"`
		Data eventbus.RawMessage `json:"data"`
	}

	// 创建消息
	msg := Message{
		Type: "test",
		Data: eventbus.RawMessage(`{"key":"value"}`),
	}

	// 序列化
	bytes, err := eventbus.Marshal(msg)
	require.NoError(t, err)

	// 反序列化
	var result Message
	err = eventbus.Unmarshal(bytes, &result)
	require.NoError(t, err)

	assert.Equal(t, "test", result.Type)
	assert.Contains(t, string(result.Data), "key")
}

// TestMarshalToString_EmptyObject 测试序列化空对象
func TestMarshalToString_EmptyObject(t *testing.T) {
	type EmptyData struct{}
	data := EmptyData{}

	str, err := eventbus.MarshalToString(data)
	require.NoError(t, err)
	assert.Equal(t, "{}", str)
}

// TestUnmarshalFromString_EmptyObject 测试反序列化空对象
func TestUnmarshalFromString_EmptyObject(t *testing.T) {
	str := `{}`

	type EmptyData struct{}
	var result EmptyData
	err := eventbus.UnmarshalFromString(str, &result)
	require.NoError(t, err)
}

// TestMarshal_Array 测试序列化数组
func TestMarshal_Array(t *testing.T) {
	data := []int{1, 2, 3, 4, 5}

	bytes, err := eventbus.Marshal(data)
	require.NoError(t, err)
	assert.Contains(t, string(bytes), "1")
	assert.Contains(t, string(bytes), "5")
}

// TestUnmarshal_Array 测试反序列化数组
func TestUnmarshal_Array(t *testing.T) {
	bytes := []byte(`[1,2,3,4,5]`)

	var result []int
	err := eventbus.Unmarshal(bytes, &result)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, result)
}

// BenchmarkMarshal 基准测试：标准序列化
func BenchmarkMarshal(b *testing.B) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eventbus.Marshal(data)
	}
}

// BenchmarkMarshalFast 基准测试：快速序列化
func BenchmarkMarshalFast(b *testing.B) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eventbus.MarshalFast(data)
	}
}

// BenchmarkMarshalToString 基准测试：序列化为字符串
func BenchmarkMarshalToString(b *testing.B) {
	type TestData struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	data := TestData{Name: "test", Age: 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eventbus.MarshalToString(data)
	}
}
