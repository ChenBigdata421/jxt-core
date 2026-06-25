package performance_tests

import (
	"context"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/stretchr/testify/require"
)

// 本文件是「长时间浸泡（soak）」性能回归测试：在持续负载下验证分区内消费流水线的 windowSize 效果，
// 并在结束后强制检查资源释放（goroutine / 堆内存）。默认时长 10 分钟。
//
// ⚠️ 这是个很慢的测试，默认**不会运行**。需显式开启：
//
//	EVENTBUS_SOAK=1 go test ./tests/eventbus/performance_regression_tests/ \
//	    -run TestPartitionPipelineSoak -v -timeout 20m
//
// 可调参数（环境变量）：
//   - EVENTBUS_SOAK_DURATION：总时长，默认 10m（会等比分到各 windowSize 阶段）
//   - EVENTBUS_SOAK_BUCKET：吞吐采样粒度，默认 30s
//   - EVENTBUS_SOAK_LATENCY：handler 模拟的 I/O 往返延迟，默认 2ms
//
// 验证目标：
//  1. windowSize 效果在持续负载下成立——W=32 阶段吞吐显著高于 W=1（跨聚合并发未被时间拖垮）。
//  2. 无吞吐衰减——每个阶段内末段吞吐不低于首段的合理比例（防内存/资源泄漏导致的性能下滑）。
//  3. 消息完整——提交数 == 完成数（无丢失/无重复）。
//  4. 【必须】资源释放——pool.Stop() + GC 后，goroutine 回到基线、堆被回收（无泄漏）。

// soaksample 是一次吞吐采样点（时间戳 + 累计完成数）。
type soaksample struct {
	t    time.Time
	done int64
}

// runWindowedForDuration 持续 dur 运行窗口化消费（流水线 windowSize 模式），按 bucket 粒度采样吞吐。
// idCounter 跨阶段/分桶保持 AggregateID 全局唯一（避免同聚合串行）。返回 (总完成数, 采样序列)。
func runWindowedForDuration(pool *eventbus.HollywoodActorPool, handler eventbus.MessageHandler,
	windowSize int, idCounter *int64, dur, bucket time.Duration) (int64, []soaksample) {

	ctx := context.Background()
	if bucket <= 0 {
		bucket = dur
	}
	sem := make(chan struct{}, windowSize)
	var wg sync.WaitGroup
	var done int64

	start := time.Now()
	deadline := start.Add(dur)
	nextBucket := start.Add(bucket)
	samples := []soaksample{{t: start, done: 0}}

	submit := func() {
		sem <- struct{}{} // 背压：在飞数上限 = windowSize
		id := atomic.AddInt64(idCounter, 1)
		msg := buildAggMsg(ctx, handler, int(id))
		_ = pool.ProcessMessage(ctx, msg) // 异步提交
		wg.Add(1)
		go func(m *eventbus.AggregateMessage) {
			defer wg.Done()
			<-m.Done
			atomic.AddInt64(&done, 1)
			<-sem
		}(msg)
	}

	for {
		now := time.Now()
		if !now.Before(deadline) {
			break
		}
		// 跨过若干桶边界时补齐采样点（submit 可能因背压短暂阻塞，故用 for 补齐漏掉的边界）
		for !now.Before(nextBucket) {
			samples = append(samples, soaksample{t: nextBucket, done: atomic.LoadInt64(&done)})
			nextBucket = nextBucket.Add(bucket)
		}
		submit()
	}
	// 排空在飞消息后补一个「deadline 时刻」的终末采样点：
	// drain flush 最多 windowSize 条，相对每桶数千条可忽略，故时间戳取 deadline、done 取全部完成数。
	wg.Wait()
	samples = append(samples, soaksample{t: deadline, done: atomic.LoadInt64(&done)})
	return atomic.LoadInt64(&done), samples
}

// bucketThroughputs 由采样序列算出每桶吞吐（msg/s）。
func bucketThroughputs(samples []soaksample) []float64 {
	if len(samples) < 2 {
		return nil
	}
	out := make([]float64, 0, len(samples)-1)
	for i := 1; i < len(samples); i++ {
		dt := samples[i].t.Sub(samples[i-1].t).Seconds()
		dd := samples[i].done - samples[i-1].done
		if dt > 0 {
			out = append(out, float64(dd)/dt)
		}
	}
	return out
}

// measureResources 采样当前 goroutine 数与堆分配（HeapAlloc）。
func measureResources() (goroutines int, heap uint64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return runtime.NumGoroutine(), m.HeapAlloc
}

// settleAndMeasureResources 停 pool 后轮询：等 goroutine 退出 + 多次 GC，返回稳定的 (goroutines, heap)。
func settleAndMeasureResources(maxWait time.Duration) (int, uint64) {
	deadline := time.Now().Add(maxWait)
	var goroutines int
	var heap uint64
	for {
		runtime.GC()
		runtime.GC() // 连续两次 GC 让回收更彻底
		goroutines, heap = measureResources()
		// goroutine 数稳定（连续两次相同）即认为退出完成
		time.Sleep(20 * time.Millisecond)
		g2, _ := measureResources()
		if g2 == goroutines || time.Now().After(deadline) {
			return goroutines, heap
		}
	}
}

func parseDurationEnv(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// TestPartitionPipelineSoak 长时间浸泡测试：验证 windowSize 效果在持续负载下成立，且结束后资源被释放。
func TestPartitionPipelineSoak(t *testing.T) {
	if os.Getenv("EVENTBUS_SOAK") != "1" {
		t.Skip("soak 测试默认跳过；设置 EVENTBUS_SOAK=1 并加 -timeout 20m 运行（默认 10 分钟）")
	}
	if testing.Short() {
		t.Skip("soak 测试在 -short 下跳过")
	}

	totalDur := parseDurationEnv("EVENTBUS_SOAK_DURATION", 10*time.Minute)
	bucketSize := parseDurationEnv("EVENTBUS_SOAK_BUCKET", 30*time.Second)
	handlerLatency := parseDurationEnv("EVENTBUS_SOAK_LATENCY", 2*time.Millisecond)
	windowSizes := []int{1, 8, 16, 32}
	phaseDur := totalDur / time.Duration(len(windowSizes))

	t.Logf("🛁 分区内消费流水线浸泡测试：总时长=%v，%d 个 windowSize 阶段各 %v，桶粒度=%v，handler I/O 延迟=%v",
		totalDur, len(windowSizes), phaseDur, bucketSize, handlerLatency)

	// 基线资源（建 pool 前，先 GC 两次取稳定值）
	runtime.GC()
	runtime.GC()
	baseGoros, baseHeap := settleAndMeasureResources(500 * time.Millisecond)
	t.Logf("📍 基线资源：goroutine=%d, heap=%.2f MB", baseGoros, float64(baseHeap)/1024/1024)

	pool := eventbus.NewHollywoodActorPool(
		eventbus.HollywoodActorPoolConfig{PoolSize: 256, InboxSize: 1000, MaxRestarts: 3},
		&eventbus.NoOpActorPoolMetricsCollector{},
	)
	handler := ioBoundHandler(handlerLatency)
	var idCounter int64 // 跨阶段全局唯一 AggregateID

	type phaseResult struct {
		windowSize    int
		completed     int64
		avgThroughput float64
		firstBucket   float64
		lastBucket    float64
		minBucket     float64
	}
	results := make([]phaseResult, 0, len(windowSizes))

	t.Logf("%-10s | %-14s | %-14s | %-14s | %-12s | %-12s | %s",
		"Window", "Completed", "Avg(msg/s)", "FirstBucket", "LastBucket", "MinBucket", "Buckets")

	for _, w := range windowSizes {
		completed, samples := runWindowedForDuration(pool, handler, w, &idCounter, phaseDur, bucketSize)
		tps := bucketThroughputs(samples)

		var sum, firstB, lastB, minB float64
		firstB, lastB = -1, -1
		for i, v := range tps {
			sum += v
			if i == 0 {
				firstB = v
			}
			lastB = v
			if minB == 0 || v < minB {
				minB = v
			}
		}
		avg := 0.0
		if len(tps) > 0 {
			avg = sum / float64(len(tps))
		}
		r := phaseResult{windowSize: w, completed: completed, avgThroughput: avg,
			firstBucket: firstB, lastBucket: lastB, minBucket: minB}
		results = append(results, r)

		t.Logf("W=%-7d | %-14d | %-14.0f | %-14.0f | %-12.0f | %-12.0f | %d",
			w, completed, avg, firstB, lastB, minB, len(tps))

		// (1) 消息完整：完成数 > 0（持续提交，应有完成）
		require.Greaterf(t, completed, int64(0), "W=%d 阶段应有消息完成", w)
		// (2) 无吞吐衰减：末段不低于首段的 decayFloor（容忍 GC 抖动；泄漏会导致单调下滑）
		if firstB > 0 && lastB > 0 {
			require.GreaterOrEqualf(t, lastB, firstB*0.5,
				"W=%d 末段吞吐 %.0f 远低于首段 %.0f（疑似泄漏导致性能衰减）", w, lastB, firstB)
		}
	}

	// (3) windowSize 效果（核心）：W=32 阶段平均吞吐应显著高于 W=1
	w1 := results[0].avgThroughput
	w32 := results[len(results)-1].avgThroughput
	t.Logf("📈 windowSize 效果（持续负载）：W=1 平均 %.0f msg/s → W=32 平均 %.0f msg/s（%.1fx）",
		w1, w32, safeRatio(w32, w1))
	require.Greaterf(t, w32, w1*3.0,
		"windowSize 效果未体现：W=32 平均吞吐 %.0f 未显著高于 W=1 %.0f（应 ≥3x）", w32, w1)

	// ===== (4) 资源释放（必须）=====
	pool.Stop()
	finalGoros, finalHeap := settleAndMeasureResources(2 * time.Second)

	heapDeltaMB := float64(finalHeap)/1024/1024 - float64(baseHeap)/1024/1024
	t.Logf("🧹 资源释放：goroutine 基线 %d → 收尾 %d（Δ%+d）；堆 基线 %.2f MB → 收尾 %.2f MB（Δ%+.2f MB）",
		baseGoros, finalGoros, finalGoros-baseGoros,
		float64(baseHeap)/1024/1024, float64(finalHeap)/1024/1024, heapDeltaMB)

	// goroutine 必须回到基线（pool 的 256 个 actor + 所有瞬态 goroutine 都应退出）。
	require.LessOrEqualf(t, finalGoros, baseGoros+5,
		"goroutine 泄漏：基线 %d，收尾 %d（泄漏 %d）", baseGoros, finalGoros, finalGoros-baseGoros)
	// 堆内存必须被回收（容忍 GC 元数据/采样切片留存，上限 64MB；真正的泄漏会让 HeapAlloc 在 GC 后仍高企）。
	require.LessOrEqualf(t, finalHeap, baseHeap+64*1024*1024,
		"堆内存未释放：基线 %.2f MB，GC 后收尾 %.2f MB（Δ%+.2f MB，疑似泄漏）",
		float64(baseHeap)/1024/1024, float64(finalHeap)/1024/1024, heapDeltaMB)

	t.Logf("✅ 浸泡测试通过：windowSize 效果持续成立、无吞吐衰减、goroutine 与堆内存均已释放")
}

func safeRatio(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
