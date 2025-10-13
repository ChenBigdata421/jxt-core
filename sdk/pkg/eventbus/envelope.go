package eventbus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Envelope 统一消息包络结构（方案A）
type Envelope struct {
	AggregateID   string     `json:"aggregate_id"`             // 聚合ID（必填）
	EventType     string     `json:"event_type"`               // 事件类型（必填）
	EventVersion  int64      `json:"event_version"`            // 事件版本（预留，为了将来可能实现事件溯源预留）
	Timestamp     time.Time  `json:"timestamp"`                // 时间戳
	TraceID       string     `json:"trace_id,omitempty"`       // 链路追踪ID（可选）
	CorrelationID string     `json:"correlation_id,omitempty"` // 关联ID（可选）
	Payload       RawMessage `json:"payload"`                  // 业务负载
}

// NewEnvelope 创建新的消息包络
func NewEnvelope(aggregateID, eventType string, eventVersion int64, payload []byte) *Envelope {
	return &Envelope{
		AggregateID:  aggregateID,
		EventType:    eventType,
		EventVersion: eventVersion,
		Timestamp:    time.Now(),
		Payload:      RawMessage(payload),
	}
}

// Validate 校验包络字段
func (e *Envelope) Validate() error {
	if strings.TrimSpace(e.AggregateID) == "" {
		return errors.New("aggregate_id is required")
	}
	if strings.TrimSpace(e.EventType) == "" {
		return errors.New("event_type is required")
	}
	if e.EventVersion <= 0 {
		return errors.New("event_version must be positive")
	}
	if len(e.Payload) == 0 {
		return errors.New("payload is required")
	}

	// 校验 aggregateID 格式
	if err := validateAggregateID(e.AggregateID); err != nil {
		return fmt.Errorf("invalid aggregate_id: %w", err)
	}

	return nil
}

// ToBytes 序列化为字节数组
func (e *Envelope) ToBytes() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return Marshal(e)
}

// FromBytes 从字节数组反序列化
func FromBytes(data []byte) (*Envelope, error) {
	var env Envelope
	if err := Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("failed to unmarshal envelope: %w", err)
	}
	if err := env.Validate(); err != nil {
		return nil, fmt.Errorf("invalid envelope: %w", err)
	}
	return &env, nil
}

// ExtractAggregateID 从消息中提取聚合ID（决定是否使用Keyed-Worker池的关键函数）
//
// 优先级：Envelope > Header > Kafka Key/NATS Subject
//
// 核心逻辑：
// - 如果能提取到聚合ID → 使用Keyed-Worker池进行顺序处理
// - 如果无法提取聚合ID → 直接并发处理，不使用Keyed-Worker池
//
// 这就是为什么：
// - SubscribeEnvelope 自动使用Keyed-Worker池（Envelope.AggregateID总是存在）
// - Subscribe 通常不使用Keyed-Worker池（原始消息通常无聚合ID）
//
// 参数说明：
// - msgBytes: 消息字节数据（优先尝试解析为Envelope）
// - headers: 消息头（兼容性支持）
// - kafkaKey: Kafka消息键（兼容性支持）
// - natsSubject: NATS主题（启发式提取）
//
// 返回值：
// - string: 聚合ID（空字符串表示无法提取）
// - error: 解析错误（通常忽略，回退到下一优先级）
func ExtractAggregateID(msgBytes []byte, headers map[string]string, kafkaKey []byte, natsSubject string) (string, error) {
	// 1. 优先从 Envelope 提取
	if len(msgBytes) > 0 {
		env, err := FromBytes(msgBytes)
		if err == nil && env.AggregateID != "" {
			return env.AggregateID, nil
		}
	}

	// 2. 从 Headers 提取（兼容方案B）
	if headers != nil {
		for _, key := range []string{"X-Aggregate-ID", "x-aggregate-id", "Aggregate-ID", "aggregate-id"} {
			if value, exists := headers[key]; exists && strings.TrimSpace(value) != "" {
				if err := validateAggregateID(value); err == nil {
					return strings.TrimSpace(value), nil
				}
			}
		}
	}

	// 3. 从 Kafka Key 提取
	if len(kafkaKey) > 0 {
		keyStr := strings.TrimSpace(string(kafkaKey))
		if keyStr != "" {
			if err := validateAggregateID(keyStr); err == nil {
				return keyStr, nil
			}
		}
	}

	// 4. 从 NATS Subject 启发式提取（最后兜底）
	// 注意：只有当明确传递natsSubject时才提取，Subscribe调用应该传递空字符串
	if natsSubject != "" {
		parts := strings.Split(natsSubject, ".")
		for i := len(parts) - 1; i >= 0; i-- {
			segment := strings.TrimSpace(parts[i])
			if segment != "" {
				if err := validateAggregateID(segment); err == nil {
					return segment, nil
				}
			}
		}
	}

	return "", errors.New("aggregate_id not found in any source")
}

// validateAggregateID 校验聚合ID格式
func validateAggregateID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("aggregate_id cannot be empty")
	}
	if len(id) > 256 {
		return errors.New("aggregate_id too long (max 256 characters)")
	}

	// 允许的字符：A-Z a-z 0-9 : _ - . /
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == ':' || r == '_' || r == '-' || r == '.' || r == '/':
		default:
			return fmt.Errorf("aggregate_id contains invalid character: %c", r)
		}
	}

	return nil
}

// EnvelopePublisher 支持Envelope的发布器接口
type EnvelopePublisher interface {
	// PublishEnvelope 发布Envelope消息
	PublishEnvelope(ctx context.Context, topic string, envelope *Envelope) error
}

// EnvelopeSubscriber 支持Envelope的订阅器接口
type EnvelopeSubscriber interface {
	// SubscribeEnvelope 订阅Envelope消息
	SubscribeEnvelope(ctx context.Context, topic string, handler EnvelopeHandler) error
}

// EnvelopeHandler Envelope消息处理器
type EnvelopeHandler func(ctx context.Context, envelope *Envelope) error

// EnvelopeEventBus 支持Envelope的EventBus接口
type EnvelopeEventBus interface {
	EventBus
	EnvelopePublisher
	EnvelopeSubscriber
}
