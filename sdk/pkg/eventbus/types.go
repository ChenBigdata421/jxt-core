package eventbus

import (
	"context"
	"time"
)

// AggregateMessage 聚合消息（用于消息处理）
type AggregateMessage struct {
	Topic       string
	Partition   int32
	Offset      int64
	Key         []byte
	Value       []byte
	Headers     map[string][]byte
	Timestamp   time.Time
	AggregateID string
	Context     context.Context
	Done        chan error
	Handler     MessageHandler // 每个消息携带自己的 handler（支持全局池）
	IsEnvelope  bool          // 标记是否是 Envelope 消息（at-least-once 语义）
}
