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

// 本文件是「波动压力」性能回归测试：windowSize 固定 32，提交速率在 HIGH 突发与 LULL 停顿之间周期波动，
// 验证流水线在「忽高忽低」的真实流量下：① 性能表现（突发期吞吐达标、停顿期窗口能排空）；
// ② 资源占用（突发期 goroutine/堆峰值有界，不随周期累积）；③ 资源释放（结束后全部回收，无泄漏）。
//
// ⚠️ 较慢，默认**不运行**。需显式开启：
//
//	EVENTBUS_BURST=1 go test ./tests/eventbus/performance_regression_tests/ \
//	    -run TestPartitionPipelineBurstLoad -v -timeout 10m
//
// 可调参数（环境变量）：
//   - EVENTBUS_BURST_DURATION：总时长，默认 5m
//   - EVENTBUS_BURST_HIGH / EVENTBUS_BURST_LULL：每个周期的突发时长 / 停顿时长，默认 10s / 5s
//   - EVENTBUS_BURST_BUCKET：采样粒度，默认 5s（建议整除 HIGH/LULL 以对齐边界）
//   - EVENTBUS_BURST_LATENCY：handler I/O 延迟，默认 2ms

// burstSample 是波动测试的一个采样点：时间、累计完成数、goroutine 数、堆(KB)。
type burstSample struct {
	t          time.Time
	done       int64
	goroutines int
	heapKB     float64
}

// runWindowedBursty 以 windowSize 在飞运行，但提交速率按周期波动：HIGH 段全速提交、LULL 段暂停提交（窗口排空）。
// 返回 (总完成数, 采样序列)。采样点含 goroutine/堆，用于看占用波动。
func runWindowedBursty(pool *eventbus.HollywoodActorPool, handler eventbus.MessageHandler,
	windowSize int, idCounter *int64, totalDur, highDur, lullDur, bucket time.Duration) (int64, []burstSample) {

	ctx := context.Background()
	sem := make(chan struct{}, windowSize)
	var wg sync.WaitGroup
	var done int64

	start := time.Now()
	deadline := start.Add(totalDur)
	cycle := highDur + lullDur
	nextBucket := start.Add(bucket)

	sampleNow := func(t time.Time) burstSample {
		_, heap := measureResources()
		return burstSample{t: t, done: atomic.LoadInt64(&done),
			goroutines: runtime.NumGoroutine(), heapKB: float64(heap) / 1024}
	}
	samples := []burstSample{sampleNow(start)}

	submit := func() {
		sem <- struct{}{} // 背压：在飞数上限 = windowSize
		id := atomic.AddInt64(idCounter, 1)
		msg := buildAggMsg(ctx, handler, int(id))
		_ = pool.ProcessMessage(ctx, msg)
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
		// 桶边界补采样（submit/LULL-sleep 可能短暂阻塞，用 for 补齐漏掉的边界）
		for !now.Before(nextBucket) {
			samples = append(samples, sampleNow(nextBucket))
			nextBucket = nextBucket.Add(bucket)
		}
		inHigh := now.Sub(start)%cycle < highDur
		if inHigh {
			submit() // HIGH：全速提交（窗口满时背压阻塞）
		} else {
			time.Sleep(5 * time.Millisecond) // LULL：暂停提交，让在飞窗口排空
		}
	}
	wg.Wait() // 排空残余在飞
	samples = append(samples, sampleNow(deadline))
	return atomic.LoadInt64(&done), samples
}

// TestPartitionPipelineBurstLoad 波动压力测试：windowSize=32，吞吐忽高忽低，验证性能与资源占用/释放。
func TestPartitionPipelineBurstLoad(t *testing.T) {
	if os.Getenv("EVENTBUS_BURST") != "1" {
		t.Skip("波动压力测试默认跳过；设置 EVENTBUS_BURST=1 并加 -timeout 10m 运行（默认 5 分钟）")
	}
	if testing.Short() {
		t.Skip("波动压力测试在 -short 下跳过")
	}

	const windowSize = 32
	const poolSize = 256
	totalDur := parseDurationEnv("EVENTBUS_BURST_DURATION", 5*time.Minute)
	highDur := parseDurationEnv("EVENTBUS_BURST_HIGH", 10*time.Second)
	lullDur := parseDurationEnv("EVENTBUS_BURST_LULL", 5*time.Second)
	bucket := parseDurationEnv("EVENTBUS_BURST_BUCKET", 5*time.Second)
	handlerLatency := parseDurationEnv("EVENTBUS_BURST_LATENCY", 2*time.Millisecond)

	t.Logf("🌊 波动压力测试：windowSize=%d，周期 HIGH=%v / LULL=%v，总时长=%v，桶=%v，handler I/O 延迟=%v",
		windowSize, highDur, lullDur, totalDur, bucket, handlerLatency)

	// 基线资源
	runtime.GC()
	runtime.GC()
	baseGoros, baseHeap := settleAndMeasureResources(500 * time.Millisecond)
	t.Logf("📍 基线资源：goroutine=%d, heap=%.2f MB", baseGoros, float64(baseHeap)/1024/1024)

	pool := eventbus.NewHollywoodActorPool(
		eventbus.HollywoodActorPoolConfig{PoolSize: poolSize, InboxSize: 1000, MaxRestarts: 3},
		&eventbus.NoOpActorPoolMetricsCollector{},
	)
	handler := ioBoundHandler(handlerLatency)
	var idCounter int64

	completed, samples := runWindowedBursty(pool, handler, windowSize, &idCounter, totalDur, highDur, lullDur, bucket)

	// 计算每桶吞吐 + 运行期峰值占用
	var tpsList []float64
	peakGoros := 0
	peakHeapKB := 0.0
	highBuckets, lowBuckets := 0, 0
	const highThresh = 3000.0 // 突发桶：W=32 突发期应远高于此
	const lowThresh = 500.0   // 停顿桶：LULL 期吞吐应低于此（仅残余 drain flush）

	for i := 1; i < len(samples); i++ {
		dt := samples[i].t.Sub(samples[i-1].t).Seconds()
		dd := samples[i].done - samples[i-1].done
		var tps float64
		if dt > 0 {
			tps = float64(dd) / dt
		}
		tpsList = append(tpsList, tps)
		if tps >= highThresh {
			highBuckets++
		}
		if tps <= lowThresh {
			lowBuckets++
		}
		if samples[i].goroutines > peakGoros {
			peakGoros = samples[i].goroutines
		}
		if samples[i].heapKB > peakHeapKB {
			peakHeapKB = samples[i].heapKB
		}
	}
	maxTps, minTps := 0.0, 0.0
	for _, v := range tpsList {
		if v > maxTps {
			maxTps = v
		}
		if minTps == 0 || v < minTps {
			minTps = v
		}
	}

	t.Logf("📈 波动概览：完成 %d 条 | 桶 %d（HIGH≥%.0f=%d 个, LULL≤%.0f=%d 个）| 吞吐 max=%.0f min=%.0f msg/s",
		completed, len(tpsList), highThresh, highBuckets, lowThresh, lowBuckets, maxTps, minTps)
	t.Logf("🧮 运行期资源占用峰值：goroutine=%d（上限 %d）, heap=%.2f MB",
		peakGoros, windowSize+poolSize+50, peakHeapKB/1024)

	// 打印每桶波形（直观看到「忽高忽低」）
	t.Logf("📊 每桶吞吐波形（msg/s，# = HIGH, . = LULL）：")
	for i, v := range tpsList {
		mark := "."
		if v >= highThresh {
			mark = "#"
		}
		t.Logf("  [%2d] %s %6.0f  goro=%d  heap=%.1fMB", i, mark, v, samples[i+1].goroutines, samples[i+1].heapKB/1024)
	}

	// (1) 波动确实施加：同时存在 HIGH 桶与 LULL 桶
	require.Greaterf(t, highBuckets, 0, "未出现 HIGH 突发桶（吞吐≥%.0f）——波动负载未生效", highThresh)
	require.Greaterf(t, lowBuckets, 0, "未出现 LULL 停顿桶（吞吐≤%.0f）——波动负载未生效", lowThresh)
	require.Greaterf(t, maxTps, minTps*5+highThresh, "吞吐未呈现忽高忽低（max=%.0f, min=%.0f）", maxTps, minTps)
	// (2) 突发期性能达标：W=32 突发吞吐应显著
	require.GreaterOrEqualf(t, maxTps, 5000.0, "突发期峰值吞吐 %.0f 过低（W=32 应≥5000 msg/s）", maxTps)
	// (3) 运行期占用有界：峰值 goroutine 不超过 windowSize + poolSize + 余量（跨周期不累积）
	require.LessOrEqualf(t, peakGoros, windowSize+poolSize+50,
		"运行期 goroutine 峰值 %d 超界 %d——疑似跨周期累积未排空", peakGoros, windowSize+poolSize+50)
	// (4) 消息完整
	require.Greaterf(t, completed, int64(0), "应有消息完成")

	// ===== (5) 资源释放（必须）=====
	pool.Stop()
	finalGoros, finalHeap := settleAndMeasureResources(2 * time.Second)
	heapDeltaMB := float64(finalHeap)/1024/1024 - float64(baseHeap)/1024/1024
	t.Logf("🧹 资源释放：goroutine 基线 %d → 收尾 %d（Δ%+d）；堆 基线 %.2f MB → 收尾 %.2f MB（Δ%+.2f MB）",
		baseGoros, finalGoros, finalGoros-baseGoros,
		float64(baseHeap)/1024/1024, float64(finalHeap)/1024/1024, heapDeltaMB)

	require.LessOrEqualf(t, finalGoros, baseGoros+5,
		"goroutine 泄漏：基线 %d，收尾 %d（泄漏 %d）", baseGoros, finalGoros, finalGoros-baseGoros)
	require.LessOrEqualf(t, finalHeap, baseHeap+64*1024*1024,
		"堆内存未释放：基线 %.2f MB，GC 后收尾 %.2f MB（Δ%+.2f MB，疑似泄漏）",
		float64(baseHeap)/1024/1024, float64(finalHeap)/1024/1024, heapDeltaMB)

	t.Logf("✅ 波动压力测试通过：吞吐忽高忽低成立、突发期性能达标、运行期占用有界、结束后 goroutine 与堆内存均已释放")
}
