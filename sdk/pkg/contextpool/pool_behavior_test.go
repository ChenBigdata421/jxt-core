package contextpool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 测试 sync.Pool 在高并发下的行为

func TestPoolBehaviorUnderLoad(t *testing.T) {
	var createCount int64 // 记录创建次数
	
	// 创建一个带计数的池
	testPool := sync.Pool{
		New: func() interface{} {
			atomic.AddInt64(&createCount, 1)
			return &Context{
				data: make(map[string]interface{}, 8),
			}
		},
	}
	
	t.Run("低并发场景", func(t *testing.T) {
		createCount = 0
		
		// 10 个并发请求
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx := testPool.Get().(*Context)
				time.Sleep(10 * time.Millisecond) // 模拟处理
				testPool.Put(ctx)
			}()
		}
		wg.Wait()
		
		fmt.Printf("低并发: 创建了 %d 个对象\n", createCount)
		// 预期：创建 10 个对象（因为都是并发的）
	})
	
	t.Run("高并发场景", func(t *testing.T) {
		createCount = 0
		
		// 1000 个并发请求
		var wg sync.WaitGroup
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx := testPool.Get().(*Context)
				time.Sleep(10 * time.Millisecond) // 模拟处理
				testPool.Put(ctx)
			}()
		}
		wg.Wait()
		
		fmt.Printf("高并发: 创建了 %d 个对象\n", createCount)
		// 预期：创建约 1000 个对象（因为都是并发的）
	})
	
	t.Run("高并发后再次请求", func(t *testing.T) {
		beforeCount := createCount
		
		// 再来 1000 个请求
		var wg sync.WaitGroup
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx := testPool.Get().(*Context)
				time.Sleep(10 * time.Millisecond)
				testPool.Put(ctx)
			}()
		}
		wg.Wait()
		
		newCreated := createCount - beforeCount
		fmt.Printf("第二轮高并发: 新创建了 %d 个对象\n", newCreated)
		// 预期：新创建很少或 0 个（复用了之前的对象）
	})
}

func TestPoolNeverBlocks(t *testing.T) {
	testPool := sync.Pool{
		New: func() interface{} {
			// 模拟慢速创建
			time.Sleep(100 * time.Millisecond)
			return &Context{
				data: make(map[string]interface{}, 8),
			}
		},
	}
	
	// 即使 New() 很慢，Get() 也不会阻塞其他 goroutine
	start := time.Now()
	
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			getStart := time.Now()
			ctx := testPool.Get().(*Context)
			getDuration := time.Since(getStart)
			
			fmt.Printf("Goroutine %d: Get() 耗时 %v\n", id, getDuration)
			
			testPool.Put(ctx)
		}(i)
	}
	wg.Wait()
	
	totalDuration := time.Since(start)
	fmt.Printf("总耗时: %v\n", totalDuration)
	
	// 预期：总耗时约 100ms（并发创建），而不是 1000ms（串行创建）
	if totalDuration > 200*time.Millisecond {
		t.Errorf("Pool 可能阻塞了，总耗时: %v", totalDuration)
	}
}

func TestPoolAutoScaling(t *testing.T) {
	var createCount int64
	var maxConcurrent int64
	var currentConcurrent int64
	
	testPool := sync.Pool{
		New: func() interface{} {
			count := atomic.AddInt64(&createCount, 1)
			current := atomic.AddInt64(&currentConcurrent, 1)
			
			// 记录最大并发数
			for {
				max := atomic.LoadInt64(&maxConcurrent)
				if current <= max || atomic.CompareAndSwapInt64(&maxConcurrent, max, current) {
					break
				}
			}
			
			return &Context{
				data: make(map[string]interface{}, 8),
			}
		},
	}
	
	// 模拟流量波动
	scenarios := []struct {
		name        string
		concurrency int
		duration    time.Duration
	}{
		{"低流量", 10, 50 * time.Millisecond},
		{"中流量", 100, 50 * time.Millisecond},
		{"高流量", 1000, 50 * time.Millisecond},
		{"回落", 50, 50 * time.Millisecond},
	}
	
	for _, scenario := range scenarios {
		createCount = 0
		currentConcurrent = 0
		
		var wg sync.WaitGroup
		for i := 0; i < scenario.concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx := testPool.Get().(*Context)
				time.Sleep(scenario.duration)
				atomic.AddInt64(&currentConcurrent, -1)
				testPool.Put(ctx)
			}()
		}
		wg.Wait()
		
		fmt.Printf("%s: 并发=%d, 创建=%d, 最大并发=%d\n",
			scenario.name, scenario.concurrency, createCount, maxConcurrent)
	}
}

func BenchmarkPoolUnderDifferentLoad(b *testing.B) {
	testPool := sync.Pool{
		New: func() interface{} {
			return &Context{
				data: make(map[string]interface{}, 8),
			}
		},
	}
	
	// 预热池
	for i := 0; i < 100; i++ {
		ctx := testPool.Get().(*Context)
		testPool.Put(ctx)
	}
	
	b.Run("池中有对象", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				ctx := testPool.Get().(*Context)
				ctx.Set("key", "value")
				testPool.Put(ctx)
			}
		})
	})
	
	b.Run("池被清空", func(b *testing.B) {
		// 触发 GC，清空池
		testPool = sync.Pool{
			New: func() interface{} {
				return &Context{
					data: make(map[string]interface{}, 8),
				}
			},
		}
		
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				ctx := testPool.Get().(*Context)
				ctx.Set("key", "value")
				testPool.Put(ctx)
			}
		})
	})
}

func TestPoolWithRealWorldPattern(t *testing.T) {
	var createCount int64
	
	testPool := sync.Pool{
		New: func() interface{} {
			atomic.AddInt64(&createCount, 1)
			return &Context{
				Context: context.Background(),
				data:    make(map[string]interface{}, 8),
			}
		},
	}
	
	// 模拟真实世界的请求模式
	t.Run("模拟真实流量", func(t *testing.T) {
		createCount = 0
		
		// 阶段 1: 启动阶段（低流量）
		fmt.Println("\n=== 阶段 1: 启动阶段 ===")
		simulateTraffic(testPool, 10, 100*time.Millisecond)
		fmt.Printf("创建对象数: %d\n", createCount)
		
		// 阶段 2: 流量上升
		fmt.Println("\n=== 阶段 2: 流量上升 ===")
		beforeCount := createCount
		simulateTraffic(testPool, 100, 100*time.Millisecond)
		fmt.Printf("新创建对象数: %d\n", createCount-beforeCount)
		
		// 阶段 3: 流量高峰
		fmt.Println("\n=== 阶段 3: 流量高峰 ===")
		beforeCount = createCount
		simulateTraffic(testPool, 1000, 100*time.Millisecond)
		fmt.Printf("新创建对象数: %d\n", createCount-beforeCount)
		
		// 阶段 4: 流量回落
		fmt.Println("\n=== 阶段 4: 流量回落 ===")
		beforeCount = createCount
		simulateTraffic(testPool, 50, 100*time.Millisecond)
		fmt.Printf("新创建对象数: %d (应该很少)\n", createCount-beforeCount)
		
		// 阶段 5: 再次高峰
		fmt.Println("\n=== 阶段 5: 再次高峰 ===")
		beforeCount = createCount
		simulateTraffic(testPool, 1000, 100*time.Millisecond)
		fmt.Printf("新创建对象数: %d (应该很少，复用之前的)\n", createCount-beforeCount)
	})
}

func simulateTraffic(pool sync.Pool, concurrency int, duration time.Duration) {
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := pool.Get().(*Context)
			ctx.Set("request", "data")
			time.Sleep(duration)
			pool.Put(ctx)
		}()
	}
	wg.Wait()
}
