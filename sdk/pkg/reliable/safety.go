package reliable

import "context"

// ReplaySafety 按 handler 声明（§6.1）。未实现 ReplayableHandler 的 handler 默认 ReplayUnsafe。
type ReplaySafety int

const (
	// ReplayUnsafe 默认值：有进程外副作用。可重试失败直接进 DEAD_LETTER（§6.1）。
	ReplayUnsafe ReplaySafety = iota
	// ReplayNeedsTxClaim 会写新行，需事务化占位。
	ReplayNeedsTxClaim
	// ReplayIdempotent 纯 upsert / delete-by-id / set-state，可安全自动重放。
	ReplayIdempotent
)

// ReplayableHandler 是可靠消费装饰器对 handler 的能力要求（§6.1）。
//
// **签名偏离 spec §6.1（本轮评审 C3）**：spec §6.1 把 ReplayableHandler 定义为嵌入
// `eventbus.EnvelopeDeliveryHandler`（收 `*Envelope` + `RawMeta`）。本计划改为 `Handle(ctx, []byte, DeliveryMeta)`
// ——raw envelope bytes + kernel 侧 DeliveryMeta 投影。这是刻意的 J2 决策（kernel 根包不得 import
// eventbus/sarama）。PR-3 的可靠消费 decorator 负责把 `env.ToBytes()` 的 bytes 解码回 `*Envelope` 再调
// handler；spec §6.1 的 `EnvelopeDeliveryHandler` 嵌入由 decorator 层实现，不在 kernel 接口体现。
type ReplayableHandler interface {
	Handle(ctx context.Context, envelopeBytes []byte, delivery DeliveryMeta) error
	HandlerID() HandlerID
	ReplaySafety() ReplaySafety
	RequiresAggregateGate() bool
}

// CanAutoReplay 报告该 safety 是否允许自动重投。
func CanAutoReplay(s ReplaySafety) bool {
	return s == ReplayIdempotent || s == ReplayNeedsTxClaim
}

// CanManualReplay 报告该 safety 是否允许人工重放（三类都允许；ReplayUnsafe 需额外双人确认）。
func CanManualReplay(s ReplaySafety) bool { return true }

// AggregateGateKey 是 DB aggregate lease 的身份（§6.2.1）。同 key 串行，不同 key 并行。
type AggregateGateKey struct {
	TenantID      int
	AggregateType string
	AggregateID   string
}

// Empty 报告聚合身份是否为空（无聚合的通知类事件跳过 gate）。
func (a AggregateGateKey) Empty() bool {
	return a.AggregateType == "" || a.AggregateID == ""
}
