package contextpool

import (
	"context"
	"sync"
	"testing"
	"time"
)

// 测试并发安全性

func TestConcurrentSet(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	var wg sync.WaitGroup
	iterations := 1000

	// 多个 goroutine 并发写入
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				ctx.Set("key", id)
			}
		}(i)
	}

	wg.Wait()
	// 如果没有 panic，说明并发安全
}

func TestConcurrentGet(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	ctx.Set("key", "value")

	var wg sync.WaitGroup

	// 多个 goroutine 并发读取
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				ctx.Get("key")
			}
		}()
	}

	wg.Wait()
}

func TestConcurrentReadWrite(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	var wg sync.WaitGroup

	// 并发读
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				ctx.Get("key")
				ctx.GetString("key")
			}
		}()
	}

	// 并发写
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				ctx.Set("key", id)
			}
		}(i)
	}

	wg.Wait()
}

func TestConcurrentErrors(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	var wg sync.WaitGroup

	// 并发添加错误
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ctx.AddError(context.Canceled)
			}
		}()
	}

	// 并发读取错误
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ctx.HasErrors()
				ctx.Errors()
			}
		}()
	}

	wg.Wait()

	// 验证错误数量
	if !ctx.HasErrors() {
		t.Error("expected errors")
	}
}

func TestConcurrentCopy(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	ctx.Set("key", "value")
	ctx.UserID = "user-123"

	var wg sync.WaitGroup

	// 并发复制
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cp := ctx.Copy()
			if cp.UserID != "user-123" {
				t.Error("copy failed")
			}
			if val := cp.GetString("key"); val != "value" {
				t.Error("copy data failed")
			}
		}()
	}

	// 同时修改原对象
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx.Set("key2", id)
		}(i)
	}

	wg.Wait()
}

func TestRaceConditionDetection(t *testing.T) {
	// 运行: go test -race -run TestRaceConditionDetection
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	done := make(chan bool)

	// Writer
	go func() {
		for i := 0; i < 100; i++ {
			ctx.Set("counter", i)
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Reader
	go func() {
		for i := 0; i < 100; i++ {
			ctx.Get("counter")
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	<-done
	<-done
}

func BenchmarkConcurrentAccess(b *testing.B) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	b.Run("ConcurrentSet", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				ctx.Set("key", i)
				i++
			}
		})
	})

	b.Run("ConcurrentGet", func(b *testing.B) {
		ctx.Set("key", "value")
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				ctx.Get("key")
			}
		})
	})

	b.Run("ConcurrentMixed", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%2 == 0 {
					ctx.Set("key", i)
				} else {
					ctx.Get("key")
				}
				i++
			}
		})
	})
}
