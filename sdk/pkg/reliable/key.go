package reliable

import (
	"errors"
	"time"
)

// HandlerID 是持久协议标识，不随 Go 类型/函数重命名而变化（§3.1）。
type HandlerID string

// Key 是 event_consumption 的全局身份（§3.1）：
//   - EventID  恒为 Envelope.EventID = 生产端 outbox 行 id（M5）；
//   - Handler  稳定 HandlerID；
//   - ItemKey  单事件恒为空串；批量 item 填稳定业务身份（禁止用数组下标，M11 v2.6）。
type Key struct {
	EventID string
	Handler HandlerID
	ItemKey string
}

// Validate 校验 Key 三要素。
func (k Key) Validate() error {
	if k.EventID == "" {
		return errors.New("reliable: Key.EventID is required")
	}
	if k.Handler == "" {
		return errors.New("reliable: Key.Handler is required")
	}
	for _, r := range k.ItemKey {
		if r == '\n' || r == '\r' || r == 0 {
			return errors.New("reliable: Key.ItemKey must not contain control characters")
		}
	}
	return nil
}

// Meta 描述性元数据（§3.1）。AggregateType/AggregateID 用于 §6.2.1 跨 topic/跨服务聚合分组；
// CausalSeq 是事件自带的领域版本号（没有则 nil，跨 topic 排序退化为同 topic 内可靠，v2.7）。
type Meta struct {
	EventType     string
	AggregateType string
	AggregateID   string
	CausalSeq     *int64
}

// MetaProvider 是默认提取方式。无法修改既有事件 DTO 时，装饰器接受显式 MetaFunc（§3.1）。
type MetaProvider interface{ Meta() Meta }

// ClaimInput 是 TryClaim 一次取得 Key、业务 Meta、tenant 与 broker RawMeta 的入参（§3.1 v2.7）。
type ClaimInput struct {
	Key      Key
	Meta     Meta
	TenantID int
	// Delivery 是 eventbus.RawMeta 的 kernel 侧投影（避免 import eventbus，从而不把 sarama 带进 kernel）。
	Delivery DeliveryMeta
}

// DeliveryMeta 是 RawMeta 的 kernel 侧投影。由消费服务从 eventbus.RawMeta 映射。
type DeliveryMeta struct {
	Topic           string
	Partition       int32
	Offset          int64
	BrokerTimestamp time.Time
	PayloadHash     string
	RawKey          []byte
	Headers         []HeaderPair
}

// HeaderPair 是有序、允许重复 key 的 header（镜像 eventbus.MessageHeader，但不依赖该包）。
type HeaderPair struct {
	Key   string
	Value []byte
}

// ClaimToken 是占位的「票据」= claim_id（UUID 字符串）。MarkSucceeded/MarkFailed 凭它做
// WHERE claim_id = ? 校验（§3.1）；持令牌者丢失所有权（租约被回收）时命中 0 行 → 记 CLAIM_TOKEN_MISMATCH。
type ClaimToken string

// String 返回底层 claim_id，供日志/调试。
func (t ClaimToken) String() string { return string(t) }

// Decision 是 TryClaim 的三种结果（§3.1）。
type Decision int

const (
	// Claimed 拿到票据，可以处理。
	Claimed Decision = iota
	// AlreadyProcessing 他人持有且租约未过期：不 ACK，交给 broker 稍后重投（返回 ErrRetryLater）。
	AlreadyProcessing
	// AlreadySettled 已终结（SUCCEEDED/DEAD_LETTER/DISCARDED）：直接 ACK。
	AlreadySettled
)
