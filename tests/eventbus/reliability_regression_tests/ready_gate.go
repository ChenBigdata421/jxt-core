package reliability_regression_tests

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// ReadyGate 是 consumer 就绪屏障（CI flake 根修）。
//
// 背景：fault-isolation 家族测试此前在 Subscribe 后用固定 sleep(100ms) 当「consumer
// 已就绪」的假设。该假设在 CI 受限资源上不成立（test-regression run #19/#23/#28 三次
// 失败，签名均为 "expected >= N, got M" 欠投递）：consumer group 完成 partition
// assignment + 首次 fetch 定位可能超过 100ms，assignment 完成前发布的消息虽已写入 log，
// fetch 起点却可能落在其后 → 早期消息整段漏读。本地热 broker 无法复现（0ms 窗口 6 轮
// 全绿），失败只在 CI 负载下出现。
//
// 修复语义：发布一条 probe envelope，等业务 handler 目击它——probe 被处理过，等价于
// 「assignment 已完成且 fetch 已消费到 probe 所在 offset」，其后发布的业务消息不可能
// 落在 fetch 起点之前。probe 带独立 AggregateID（ReadyProbeAggregateID），不污染业务
// 聚合的 hash 路由；handler 对 probe 提前 return，不进业务计数器（exact-equality 断言
// 依赖这一点）。
type ReadyGate struct {
	tripped atomic.Bool
}

// NewReadyGate 创建未放行的就绪门。
func NewReadyGate() *ReadyGate {
	return &ReadyGate{}
}

// Trip 放行（latch 不可逆）。
func (g *ReadyGate) Trip() { g.tripped.Store(true) }

// Tripped 报告是否已放行。
func (g *ReadyGate) Tripped() bool { return g.tripped.Load() }

// ProbeTrip 原子地「目击即放行」：返回是否是首次放行（恰好一次 true）。
func (g *ReadyGate) ProbeTrip() bool { return g.tripped.CompareAndSwap(false, true) }

// ReadyProbeAggregateID 是 consumer-ready probe envelope 的 AggregateID。
// 选独立哨兵值而非业务前缀：hash 路由到独立 Actor，且业务侧按 AggregateID 统计的
// 计数器永远不会把 probe 算进去。
const ReadyProbeAggregateID = "__consumer_ready_probe__"

// ReadyProbePayload 是 raw（非 envelope）路径的 probe JSON 载荷——与 raw 测试的
// 解码结构（Aggregate/Version 字段）形状兼容，handler 解码后按 Aggregate 哨兵识别。
const ReadyProbePayload = `{"aggregate":"__consumer_ready_probe__","version":0}`

// AwaitConsumerReady 就绪屏障：发布 probe envelope 并轮询 gate.Tripped()。
// 业务 handler 开头需三行接线（见 fault-isolation 测试）：
//
//	if envelope.AggregateID == ReadyProbeAggregateID {
//	    gate.Trip()
//	    return nil
//	}
//
// 在 Subscribe 之后【替代固定 sleep】调用。probe 被目击 ⇒ consumer group 的 fetch
// 已推进到 probe 之后，其后发布的业务消息不会被漏读。
func (h *TestHelper) AwaitConsumerReady(ctx context.Context, bus eventbus.EventBus, topic string, gate *ReadyGate, timeout time.Duration) bool {
	probe := &eventbus.Envelope{
		EventID:      fmt.Sprintf("consumer-ready-probe-%d", h.GetTimestamp()),
		AggregateID:  ReadyProbeAggregateID,
		EventType:    "ConsumerReadyProbe",
		EventVersion: 1,
		Timestamp:    time.Now(),
		Payload:      []byte(ReadyProbePayload),
	}
	if err := bus.PublishEnvelope(ctx, topic, probe); err != nil {
		h.t.Logf("⚠️ consumer-ready probe publish failed: %v", err)
		return false
	}
	return h.WaitForCondition(gate.Tripped, timeout, "consumer ready (probe witnessed)")
}

// AwaitRawConsumerReady 是 raw（非 envelope）订阅路径的就绪屏障：以普通 Publish 发布
// probe JSON（ReadyProbePayload），等 gate.Tripped()。raw handler 需在解码后按
// Aggregate 哨兵接线：
//
//	if payload.Aggregate == ReadyProbeAggregateID {
//	    gate.Trip()
//	    return nil
//	}
func (h *TestHelper) AwaitRawConsumerReady(ctx context.Context, bus eventbus.EventBus, topic string, gate *ReadyGate, timeout time.Duration) bool {
	if err := bus.Publish(ctx, topic, []byte(ReadyProbePayload)); err != nil {
		h.t.Logf("⚠️ consumer-ready raw probe publish failed: %v", err)
		return false
	}
	return h.WaitForCondition(gate.Tripped, timeout, "raw consumer ready (probe witnessed)")
}
