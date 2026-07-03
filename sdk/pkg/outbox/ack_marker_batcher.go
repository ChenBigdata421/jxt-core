package outbox

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ackMarkerBatcher 把成功 ACK 的 EventID 攒批，按"满 maxSize / 每 flushEvery / Close"
// 三种时机触发一次 flushFunc（→ repo.MarkBatchAsPublished），把 N 次单条 UPDATE 合成 1 次批量 UPDATE。
//
// 优雅关停（Close）会冲刷剩余缓冲；硬崩溃丢约 maxSize 条（突发 ACK 可瞬时超出，但有限），
// 靠消费端 handler 幂等兜底（at-least-once 固有风险的放大，非新风险）。
type ackMarkerBatcher struct {
	maxSize    int
	flushEvery time.Duration
	threshold  int
	flushFunc  func(ctx context.Context, ids []string) error
	onError    func(error)

	mu     sync.Mutex
	ids    []string
	closed bool
	fails  int // 连续 flush 失败次数；成功重置

	wake   chan struct{} // 满 maxSize 时唤醒 loop 立即 flush
	done   chan struct{} // Close 信号
	exited chan struct{} // loop goroutine 退出信号
}

func newAckMarkerBatcher(maxSize int, flushEvery time.Duration, threshold int,
	flushFunc func(ctx context.Context, ids []string) error, onError func(error)) *ackMarkerBatcher {
	if maxSize < 1 {
		maxSize = 50
	}
	if flushEvery <= 0 {
		flushEvery = 200 * time.Millisecond
	}
	b := &ackMarkerBatcher{
		maxSize:    maxSize,
		flushEvery: flushEvery,
		threshold:  threshold,
		flushFunc:  flushFunc,
		onError:    onError,
		wake:       make(chan struct{}, 1),
		done:       make(chan struct{}),
		exited:     make(chan struct{}),
	}
	go b.loop()
	return b
}

// Add 追加一个待标记 ID；满 maxSize 时唤醒 loop 立即 flush。Close 后静默丢弃。
func (b *ackMarkerBatcher) Add(id string) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.ids = append(b.ids, id)
	full := len(b.ids) >= b.maxSize
	b.mu.Unlock()
	if full {
		select {
		case b.wake <- struct{}{}:
		default:
		}
	}
}

// Close 停止 ticker，冲刷剩余缓冲，等待 flush 完成。重复调用返回 nil。
func (b *ackMarkerBatcher) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()
	close(b.done)
	<-b.exited
	return nil
}

func (b *ackMarkerBatcher) loop() {
	defer close(b.exited)
	ticker := time.NewTicker(b.flushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-b.done:
			b.flush()
			return
		case <-b.wake:
			b.flush()
		case <-ticker.C:
			b.flush()
		}
	}
}

// flush 取出缓冲、调 flushFunc。失败按 threshold 控制告警节奏（避免每 tick 刷屏）。
//
// threshold > 0：仅在 fails%threshold==0 时告警（如 threshold=5 → fails=5,10,15 各一次）。
// threshold <= 0：每次失败都告警（baseline）。
// 成功则 fails 归零。事件失败时丢弃该批 ID、保持 Pending，由 OutboxScheduler poller
// 下个 tick FindPendingEvents 重取 → 重发 → 重 ACK → 重入批（at-least-once，见 plan A7）。
func (b *ackMarkerBatcher) flush() {
	b.mu.Lock()
	if len(b.ids) == 0 {
		b.mu.Unlock()
		return
	}
	batch := b.ids
	b.ids = nil
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := b.flushFunc(ctx, batch)
	cancel()

	b.mu.Lock()
	var alert error
	if err != nil {
		b.fails++
		switch {
		case b.threshold > 0 && b.fails%b.threshold == 0:
			alert = fmt.Errorf("persistent outbox mark failure: %d consecutive batch flushes failed; events remain Pending and will be reprocessed each tick", b.fails)
		case b.threshold <= 0:
			alert = fmt.Errorf("outbox mark flush failed (consecutive=%d): %w", b.fails, err)
		}
	} else {
		b.fails = 0
	}
	b.mu.Unlock()

	if alert != nil && b.onError != nil {
		b.onError(alert)
	}
}
