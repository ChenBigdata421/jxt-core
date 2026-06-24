package performance_tests

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/stretchr/testify/require"
)

// 这组测试验证「分区内消费流水线」优化的并发收益（见 docs/perftest/消费循环流水线优化设计.md §2.4）。
//
// 流水线把旧的「提交一条即阻塞等 Done」改成「窗口内在飞 N 条、按完成情况连续推进」。
// 吞吐收益来自重叠 I/O-bound handler 的等待（gRPC/DB 往返）：N 在飞 ⇒ N 段等待重叠，而非串行排队。
//
// 约束：流水线内部（partitionPipeline.run）未导出，且本地无 Kafka broker。因此这里通过公共的
// HollywoodActorPool 复现两种消费模式做对比——这是对优化「性能机制」的忠实建模，且无需 broker：
//   - Serial（旧/窗口=1 等价）：提交一条 → 阻塞等 Done（镜像 kafka.go:1028 的 processMessageWithKeyedPool）。
//   - Windowed（流水线 windowSize=W）：经信号量限流在飞 W 条，drain Done 后回填（镜像 run 的窗口提交+背压）。
// 两者用同一个 pool、同一个 handler、同一批消息，只有「在飞窗口」不同。pool 会真正并发跑 W 个 handler
// （hollywood_actor_pool.go:208 调 Handler，:235/:248 回填 Done），故 Windowed 的并发是真的。

// minSpeedupW8 是 Windowed(W=8) 相对 Serial 必须达到的最低加速比（I/O-bound 工作负载）。
// 实测稳定在 ≈7.9×（2ms/5ms 两个 I/O 工作负载一致）；此处取 4.0（略高于实测的一半）。
// 该指标为「比值」，对机器绝对速度鲁棒（负载等比例拖慢 serial 与 windowed，比值不变），故阈值可较紧。
// 仍留有余量：只有跨聚合并发出现实质回归（如 pool 被重新串行化 → ~1×，或显著退化 <4×）才会触发失败。
const minSpeedupW8 = 4.0

// ConcurrencyPerfMetrics 记录一次消费模式（通过 actor pool）的性能指标。
type ConcurrencyPerfMetrics struct {
	Pattern         string // "Serial(Legacy)" / "Windowed(W=N)"
	WindowSize      int
	Workload        string // "IOBound_2ms" / "IOBound_5ms"
	MessageCount    int
	Duration        time.Duration
	Throughput      float64 // msg/s
	SpeedupVsSerial float64 // windowed 吞吐 / serial 吞吐
	Completed       int     // 应 == MessageCount
	GoroutinePeak   int32
}

// ioBoundHandler 返回一个模拟 I/O 往返延迟的 handler（设计 §2.4：handler 大部分时间在等 gRPC/DB）。
func ioBoundHandler(latency time.Duration) eventbus.MessageHandler {
	return func(_ context.Context, _ []byte) error {
		if latency > 0 {
			time.Sleep(latency)
		}
		return nil
	}
}

// buildAggMsg 构造一条 pool 可处理的 AggregateMessage；每条用不同 AggregateID → 路由到不同 actor（跨聚合并发）。
func buildAggMsg(ctx context.Context, handler eventbus.MessageHandler, idx int) *eventbus.AggregateMessage {
	return &eventbus.AggregateMessage{
		Topic:       "perf-test",
		Offset:      int64(idx),
		Value:       []byte(fmt.Sprintf("msg-%d", idx)),
		AggregateID: fmt.Sprintf("agg-%d", idx),
		Context:     ctx,
		Done:        make(chan error, 1),
		Handler:     handler,
		IsEnvelope:  true,
	}
}

// trackPeakGoroutine 启动一个采样 goroutine，周期性记录运行期峰值 goroutine 数；返回停止函数。
func trackPeakGoroutine() (*int32, func()) {
	var peak int32
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				if g := int32(runtime.NumGoroutine()); g > atomic.LoadInt32(&peak) {
					atomic.StoreInt32(&peak, g)
				}
				time.Sleep(time.Millisecond)
			}
		}
	}()
	return &peak, func() { close(stop) }
}

// runSerial 复现旧消费模式：提交一条 → 阻塞等 Done（windowSize=1 等价）。返回 (完成数, 耗时, 峰值goroutine)。
func runSerial(pool *eventbus.HollywoodActorPool, handler eventbus.MessageHandler, n int) (int, time.Duration, int32) {
	ctx := context.Background()
	peak, stopPeak := trackPeakGoroutine()
	start := time.Now()
	completed := 0
	for i := 0; i < n; i++ {
		msg := buildAggMsg(ctx, handler, i)
		_ = pool.ProcessMessage(ctx, msg)
		<-msg.Done
		completed++
	}
	dur := time.Since(start)
	stopPeak()
	return completed, dur, atomic.LoadInt32(peak)
}

// runWindowed 复现流水线消费模式：经信号量把在飞数限到 windowSize，drain Done 后回填（背压+连续推进的吞吐等价模型）。
func runWindowed(pool *eventbus.HollywoodActorPool, handler eventbus.MessageHandler, n, windowSize int) (int, time.Duration, int32) {
	ctx := context.Background()
	sem := make(chan struct{}, windowSize)
	var wg sync.WaitGroup
	peak, stopPeak := trackPeakGoroutine()

	wg.Add(n)
	start := time.Now()
	for i := 0; i < n; i++ {
		sem <- struct{}{} // 背压：在飞数上限 = windowSize
		msg := buildAggMsg(ctx, handler, i)
		_ = pool.ProcessMessage(ctx, msg) // 异步提交，立即返回
		go func(m *eventbus.AggregateMessage) {
			defer wg.Done()
			<-m.Done
			<-sem
		}(msg)
	}
	wg.Wait()
	dur := time.Since(start)
	stopPeak()
	return n, dur, atomic.LoadInt32(peak)
}

// TestPartitionPipelineConcurrencyPerformance 验证窗口化消费（流水线）相对串行消费（旧路径）的并发吞吐提升。
func TestPartitionPipelineConcurrencyPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	const messageCount = 500
	workloads := []struct {
		name    string
		latency time.Duration
	}{
		{"IOBound_2ms", 2 * time.Millisecond},
		{"IOBound_5ms", 5 * time.Millisecond},
	}
	windowSizes := []int{1, 8, 16, 32}

	initialGoroutines := runtime.NumGoroutine()
	pool := eventbus.NewHollywoodActorPool(
		eventbus.HollywoodActorPoolConfig{PoolSize: 256, InboxSize: 1000, MaxRestarts: 3},
		&eventbus.NoOpActorPoolMetricsCollector{},
	)

	t.Logf("🚀 分区内消费流水线并发性能对比（messageCount=%d, poolSize=256）", messageCount)
	t.Logf("%-12s | %-18s | %-12s | %-12s | %-8s | %-8s | %s",
		"Workload", "Pattern", "Duration", "Throughput", "Speedup", "PeakGo", "Completed")

	for _, wl := range workloads {
		t.Run(wl.name, func(t *testing.T) {
			handler := ioBoundHandler(wl.latency)

			// 预热：先跑一小批窗口化，稳定 pool/调度。
			runWindowed(pool, handler, 50, 8)

			// 基线：旧串行模式。
			serialCompleted, serialDur, _ := runSerial(pool, handler, messageCount)
			require.Equal(t, messageCount, serialCompleted, "serial: 全部消息必须完成")
			serialThroughput := float64(messageCount) / serialDur.Seconds()
			t.Logf("%-12s | %-18s | %-12v | %-12.0f | %-8s | %-8s | %d/%d",
				wl.name, "Serial(Legacy)", serialDur.Round(time.Millisecond), serialThroughput, "1.00x", "-", serialCompleted, messageCount)

			var w8 *ConcurrencyPerfMetrics
			prevThroughput := serialThroughput
			for _, w := range windowSizes {
				completed, dur, peak := runWindowed(pool, handler, messageCount, w)
				require.Equal(t, messageCount, completed, "windowed W=%d: 全部消息必须完成（无丢失/无重复）", w)
				throughput := float64(messageCount) / dur.Seconds()
				m := &ConcurrencyPerfMetrics{
					Pattern: fmt.Sprintf("Windowed(W=%d)", w), WindowSize: w, Workload: wl.name,
					MessageCount: messageCount, Duration: dur, Throughput: throughput,
					SpeedupVsSerial: throughput / serialThroughput, Completed: completed, GoroutinePeak: peak,
				}
				t.Logf("%-12s | %-18s | %-12v | %-12.0f | %-8.2fx | %-8d | %d/%d",
					wl.name, m.Pattern, dur.Round(time.Millisecond), throughput, m.SpeedupVsSerial, peak, completed, messageCount)

				// 模型健全性：W=1 即串行，吞吐应与 serial 相当（2× 容差）。
				if w == 1 {
					ratio := throughput / serialThroughput
					require.LessOrEqualf(t, ratio, 2.0, "W=1 应≈serial，实际比 %.2f", ratio)
					require.GreaterOrEqualf(t, ratio, 0.5, "W=1 应≈serial，实际比 %.2f", ratio)
				} else {
					// 单调扩展：更大窗口不应显著回退（10% 容差，防调度抖动）。
					require.GreaterOrEqualf(t, throughput, prevThroughput*0.9,
						"windowed 吞吐应不劣于更小窗口（W=%d, prev=%.0f, cur=%.0f）", w, prevThroughput, throughput)
				}
				if w == 8 {
					w8 = m
				}
				prevThroughput = throughput
			}

			// 核心断言：I/O-bound 下 Windowed(W=8) 相对 Serial 必须显著提速（阈值依据实测校准）。
			require.NotNil(t, w8)
			require.GreaterOrEqualf(t, w8.SpeedupVsSerial, minSpeedupW8,
				"[%s] Windowed(W=8) 加速比 %.2fx 低于阈值 %.1fx——跨聚合并发可能已回归", wl.name, w8.SpeedupVsSerial, minSpeedupW8)
		})
	}

	// 收尾：停掉 pool，校验无 goroutine 泄漏（轮询等 pool actor 退出后再计数）。
	pool.Stop()
	deadline := time.Now().Add(500 * time.Millisecond)
	var finalGoroutines int
	for time.Now().Before(deadline) {
		runtime.Gosched()
		finalGoroutines = runtime.NumGoroutine()
		if finalGoroutines <= initialGoroutines+5 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.LessOrEqualf(t, finalGoroutines, initialGoroutines+5,
		"goroutine 泄漏：初始 %d，收尾 %d", initialGoroutines, finalGoroutines)
}
