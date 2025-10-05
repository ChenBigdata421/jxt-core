package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	loggerPkg "github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// OrderCreatedEvent 订单创建事件
type OrderCreatedEvent struct {
	OrderID    string  `json:"order_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
}

// OrderPaidEvent 订单支付事件
type OrderPaidEvent struct {
	OrderID   string  `json:"order_id"`
	PaymentID string  `json:"payment_id"`
	Amount    float64 `json:"amount"`
	Method    string  `json:"method"`
}

func main() {
	// 初始化全局logger（避免nil pointer）
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 设置全局logger
	zap.ReplaceGlobals(logger)

	// 初始化logger包的全局变量
	loggerPkg.Logger = logger
	loggerPkg.DefaultLogger = logger.Sugar()

	// 配置EventBus（使用内存实现进行演示）
	cfg := &eventbus.EventBusConfig{
		Type: "memory",
	}

	// 创建EventBus
	bus, err := eventbus.NewEventBus(cfg)
	if err != nil {
		log.Fatalf("Failed to create EventBus: %v", err)
	}
	defer bus.Close()

	ctx := context.Background()

	// 演示方案A：Envelope消息发布与订阅
	fmt.Println("=== 方案A：Envelope 消息示例 ===")

	// 1. 订阅Envelope消息
	err = bus.(eventbus.EnvelopeEventBus).SubscribeEnvelope(ctx, "orders", func(ctx context.Context, envelope *eventbus.Envelope) error {
		fmt.Printf("收到Envelope消息:\n")
		fmt.Printf("  AggregateID: %s\n", envelope.AggregateID)
		fmt.Printf("  EventType: %s\n", envelope.EventType)
		fmt.Printf("  EventVersion: %d\n", envelope.EventVersion)
		fmt.Printf("  Timestamp: %s\n", envelope.Timestamp.Format(time.RFC3339))

		// 根据事件类型处理不同的业务逻辑
		switch envelope.EventType {
		case "OrderCreated":
			var event OrderCreatedEvent
			if err := json.Unmarshal(envelope.Payload, &event); err != nil {
				return fmt.Errorf("failed to unmarshal OrderCreated event: %w", err)
			}
			fmt.Printf("  订单创建: OrderID=%s, CustomerID=%s, Amount=%.2f %s\n",
				event.OrderID, event.CustomerID, event.Amount, event.Currency)

		case "OrderPaid":
			var event OrderPaidEvent
			if err := json.Unmarshal(envelope.Payload, &event); err != nil {
				return fmt.Errorf("failed to unmarshal OrderPaid event: %w", err)
			}
			fmt.Printf("  订单支付: OrderID=%s, PaymentID=%s, Amount=%.2f, Method=%s\n",
				event.OrderID, event.PaymentID, event.Amount, event.Method)
		}

		fmt.Println()
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// 等待订阅生效
	time.Sleep(100 * time.Millisecond)

	// 2. 发布订单创建事件
	orderCreated := OrderCreatedEvent{
		OrderID:    "order-12345",
		CustomerID: "customer-67890",
		Amount:     99.99,
		Currency:   "USD",
	}

	payload, _ := json.Marshal(orderCreated)
	envelope1 := eventbus.NewEnvelope("order-12345", "OrderCreated", 1, payload)
	envelope1.TraceID = "trace-abc123"
	envelope1.CorrelationID = "corr-def456"

	err = bus.(eventbus.EnvelopeEventBus).PublishEnvelope(ctx, "orders", envelope1)
	if err != nil {
		log.Fatalf("Failed to publish OrderCreated envelope: %v", err)
	}

	// 3. 发布订单支付事件
	orderPaid := OrderPaidEvent{
		OrderID:   "order-12345",
		PaymentID: "payment-98765",
		Amount:    99.99,
		Method:    "credit_card",
	}

	payload2, _ := json.Marshal(orderPaid)
	envelope2 := eventbus.NewEnvelope("order-12345", "OrderPaid", 2, payload2)
	envelope2.TraceID = "trace-abc123"
	envelope2.CorrelationID = "corr-def456"

	err = bus.(eventbus.EnvelopeEventBus).PublishEnvelope(ctx, "orders", envelope2)
	if err != nil {
		log.Fatalf("Failed to publish OrderPaid envelope: %v", err)
	}

	// 等待消息处理
	time.Sleep(500 * time.Millisecond)

	// 演示aggregateID提取
	fmt.Println("=== AggregateID 提取示例 ===")

	// 创建测试Envelope
	testPayload, _ := json.Marshal(map[string]interface{}{"test": "data"})
	testEnvelope := eventbus.NewEnvelope("test-aggregate-123", "TestEvent", 1, testPayload)
	envelopeBytes, _ := testEnvelope.ToBytes()

	// 测试不同优先级的提取
	testCases := []struct {
		name        string
		msgBytes    []byte
		headers     map[string]string
		kafkaKey    []byte
		natsSubject string
	}{
		{
			name:     "从Envelope提取（优先级1）",
			msgBytes: envelopeBytes,
			headers:  map[string]string{"X-Aggregate-ID": "header-id"},
			kafkaKey: []byte("kafka-key-id"),
		},
		{
			name:        "从Header提取（优先级2）",
			msgBytes:    []byte(`{"invalid": "json"}`),
			headers:     map[string]string{"X-Aggregate-ID": "header-id"},
			kafkaKey:    []byte("kafka-key-id"),
			natsSubject: "events.order.subject-id",
		},
		{
			name:        "从Kafka Key提取（优先级3）",
			msgBytes:    []byte(`{"invalid": "json"}`),
			headers:     map[string]string{},
			kafkaKey:    []byte("kafka-key-id"),
			natsSubject: "events.order.subject-id",
		},
		{
			name:        "从NATS Subject提取（优先级4）",
			msgBytes:    []byte(`{"invalid": "json"}`),
			headers:     map[string]string{},
			kafkaKey:    []byte(""),
			natsSubject: "events.order.subject-id",
		},
	}

	for _, tc := range testCases {
		aggregateID, err := eventbus.ExtractAggregateID(tc.msgBytes, tc.headers, tc.kafkaKey, tc.natsSubject)
		if err != nil {
			fmt.Printf("%s: 提取失败 - %v\n", tc.name, err)
		} else {
			fmt.Printf("%s: %s\n", tc.name, aggregateID)
		}
	}

	fmt.Println("\n=== 示例完成 ===")
}
