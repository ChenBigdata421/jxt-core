package contextpool

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestAcquireWithHeaders 测试从 Gin Context 提取 Header 信息
func TestAcquireWithHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("提取 X-Request-ID header", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Request-ID", "req-12345")
		c.Request = req

		ctx := Acquire(c)
		defer Release(ctx)

		if ctx.RequestID != "req-12345" {
			t.Errorf("expected RequestID 'req-12345', got '%s'", ctx.RequestID)
		}
	})

	t.Run("Header 为空时从 Gin Context 获取 requestID", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/test", nil)
		c.Set("requestID", "req-from-context")

		ctx := Acquire(c)
		defer Release(ctx)

		if ctx.RequestID != "req-from-context" {
			t.Errorf("expected RequestID 'req-from-context', got '%s'", ctx.RequestID)
		}
	})

	t.Run("提取 X-Trace-ID header", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Trace-ID", "trace-67890")
		c.Request = req

		ctx := Acquire(c)
		defer Release(ctx)

		if ctx.TraceID != "trace-67890" {
			t.Errorf("expected TraceID 'trace-67890', got '%s'", ctx.TraceID)
		}
	})

	t.Run("同时提取多个字段", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Request-ID", "req-111")
		req.Header.Set("X-Trace-ID", "trace-222")
		c.Request = req
		c.Set("userID", "user-333")
		c.Set("tenantID", "tenant-444")

		ctx := Acquire(c)
		defer Release(ctx)

		if ctx.RequestID != "req-111" {
			t.Errorf("expected RequestID 'req-111', got '%s'", ctx.RequestID)
		}
		if ctx.TraceID != "trace-222" {
			t.Errorf("expected TraceID 'trace-222', got '%s'", ctx.TraceID)
		}
		if ctx.UserID != "user-333" {
			t.Errorf("expected UserID 'user-333', got '%s'", ctx.UserID)
		}
		if ctx.TenantID != "tenant-444" {
			t.Errorf("expected TenantID 'tenant-444', got '%s'", ctx.TenantID)
		}
	})
}

// TestMustGet 测试 MustGet 方法
func TestMustGet(t *testing.T) {
	t.Run("获取存在的键", func(t *testing.T) {
		ctx := AcquireWithContext(context.Background())
		defer Release(ctx)

		ctx.Set("key", "value")
		val := ctx.MustGet("key")

		if val != "value" {
			t.Errorf("expected 'value', got '%v'", val)
		}
	})

	t.Run("获取不存在的键应该 panic", func(t *testing.T) {
		ctx := AcquireWithContext(context.Background())
		defer Release(ctx)

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic, but didn't panic")
			} else {
				expectedMsg := "key 'nonexistent' does not exist"
				if r != expectedMsg {
					t.Errorf("expected panic message '%s', got '%v'", expectedMsg, r)
				}
			}
		}()

		ctx.MustGet("nonexistent")
	})
}

// TestDuration 测试执行时长统计
func TestDuration(t *testing.T) {
	t.Run("Duration 应该返回正确的时长", func(t *testing.T) {
		ctx := AcquireWithContext(context.Background())
		defer Release(ctx)

		// 等待一小段时间
		time.Sleep(10 * time.Millisecond)

		duration := ctx.Duration()
		if duration < 10*time.Millisecond {
			t.Errorf("expected duration >= 10ms, got %v", duration)
		}
		if duration > 100*time.Millisecond {
			t.Errorf("expected duration < 100ms, got %v", duration)
		}
	})

	t.Run("多次调用 Duration 应该返回递增的值", func(t *testing.T) {
		ctx := AcquireWithContext(context.Background())
		defer Release(ctx)

		d1 := ctx.Duration()
		time.Sleep(5 * time.Millisecond)
		d2 := ctx.Duration()

		if d2 <= d1 {
			t.Errorf("expected d2 > d1, got d1=%v, d2=%v", d1, d2)
		}
	})
}

// TestReleaseNil 测试释放 nil Context
func TestReleaseNil(t *testing.T) {
	t.Run("Release nil 不应该 panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Release(nil) should not panic, got: %v", r)
			}
		}()

		Release(nil)
	})
}

// TestGetTypeMismatch 测试类型不匹配的场景
func TestGetTypeMismatch(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	t.Run("GetString 获取非字符串类型", func(t *testing.T) {
		ctx.Set("key", 123)
		val := ctx.GetString("key")
		if val != "" {
			t.Errorf("expected empty string, got '%s'", val)
		}
	})

	t.Run("GetInt 获取非整数类型", func(t *testing.T) {
		ctx.Set("key", "string")
		val := ctx.GetInt("key")
		if val != 0 {
			t.Errorf("expected 0, got %d", val)
		}
	})

	t.Run("GetBool 获取非布尔类型", func(t *testing.T) {
		ctx.Set("key", 123)
		val := ctx.GetBool("key")
		if val != false {
			t.Errorf("expected false, got %v", val)
		}
	})

	t.Run("Get 不存在的键", func(t *testing.T) {
		_, ok := ctx.Get("nonexistent")
		if ok {
			t.Error("expected ok=false for nonexistent key")
		}
	})

	t.Run("GetString 不存在的键", func(t *testing.T) {
		val := ctx.GetString("nonexistent")
		if val != "" {
			t.Errorf("expected empty string, got '%s'", val)
		}
	})

	t.Run("GetInt 不存在的键", func(t *testing.T) {
		val := ctx.GetInt("nonexistent")
		if val != 0 {
			t.Errorf("expected 0, got %d", val)
		}
	})

	t.Run("GetBool 不存在的键", func(t *testing.T) {
		val := ctx.GetBool("nonexistent")
		if val != false {
			t.Errorf("expected false, got %v", val)
		}
	})
}

// TestAddErrorNil 测试添加 nil 错误
func TestAddErrorNil(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	t.Run("AddError(nil) 不应该添加错误", func(t *testing.T) {
		ctx.AddError(nil)

		if ctx.HasErrors() {
			t.Error("expected no errors after AddError(nil)")
		}

		if len(ctx.Errors()) != 0 {
			t.Errorf("expected 0 errors, got %d", len(ctx.Errors()))
		}
	})

	t.Run("AddError(nil) 和正常错误混合", func(t *testing.T) {
		ctx.AddError(errors.New("error1"))
		ctx.AddError(nil)
		ctx.AddError(errors.New("error2"))
		ctx.AddError(nil)

		if !ctx.HasErrors() {
			t.Error("expected errors")
		}

		errs := ctx.Errors()
		if len(errs) != 2 {
			t.Errorf("expected 2 errors, got %d", len(errs))
		}
	})
}

// TestMultipleDataItems 测试多个数据项
func TestMultipleDataItems(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	t.Run("存储和获取多个不同类型的数据", func(t *testing.T) {
		ctx.Set("string", "value")
		ctx.Set("int", 42)
		ctx.Set("bool", true)
		ctx.Set("float", 3.14)
		ctx.Set("slice", []int{1, 2, 3})
		ctx.Set("map", map[string]int{"a": 1})

		if ctx.GetString("string") != "value" {
			t.Error("string value mismatch")
		}
		if ctx.GetInt("int") != 42 {
			t.Error("int value mismatch")
		}
		if ctx.GetBool("bool") != true {
			t.Error("bool value mismatch")
		}

		val, ok := ctx.Get("float")
		if !ok || val.(float64) != 3.14 {
			t.Error("float value mismatch")
		}

		val, ok = ctx.Get("slice")
		if !ok {
			t.Error("slice not found")
		}

		val, ok = ctx.Get("map")
		if !ok {
			t.Error("map not found")
		}
	})

	t.Run("覆盖已存在的键", func(t *testing.T) {
		ctx.Set("key", "value1")
		if ctx.GetString("key") != "value1" {
			t.Error("first value mismatch")
		}

		ctx.Set("key", "value2")
		if ctx.GetString("key") != "value2" {
			t.Error("second value mismatch")
		}
	})
}

// TestCopyWithErrors 测试 Copy 复制 errors
func TestCopyWithErrors(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	t.Run("Copy 应该复制 errors", func(t *testing.T) {
		err1 := errors.New("error1")
		err2 := errors.New("error2")

		ctx.AddError(err1)
		ctx.AddError(err2)

		cp := ctx.Copy()

		// 验证副本有相同的错误
		if !cp.HasErrors() {
			t.Error("copy should have errors")
		}

		cpErrs := cp.Errors()
		if len(cpErrs) != 2 {
			t.Errorf("expected 2 errors in copy, got %d", len(cpErrs))
		}

		// 验证错误内容相同
		if cpErrs[0].Error() != err1.Error() {
			t.Error("first error mismatch")
		}
		if cpErrs[1].Error() != err2.Error() {
			t.Error("second error mismatch")
		}

		// 修改原对象的错误不应影响副本
		ctx.AddError(errors.New("error3"))
		if len(cp.Errors()) != 2 {
			t.Error("copy should still have 2 errors")
		}
	})

	t.Run("Copy 空 errors", func(t *testing.T) {
		ctx2 := AcquireWithContext(context.Background())
		defer Release(ctx2)

		cp := ctx2.Copy()

		if cp.HasErrors() {
			t.Error("copy should not have errors")
		}
	})
}

// TestCopyWithMultipleData 测试 Copy 复制多个数据项
func TestCopyWithMultipleData(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	t.Run("Copy 应该复制所有数据项", func(t *testing.T) {
		ctx.Set("key1", "value1")
		ctx.Set("key2", 123)
		ctx.Set("key3", true)

		cp := ctx.Copy()

		if cp.GetString("key1") != "value1" {
			t.Error("key1 not copied")
		}
		if cp.GetInt("key2") != 123 {
			t.Error("key2 not copied")
		}
		if cp.GetBool("key3") != true {
			t.Error("key3 not copied")
		}

		// 修改副本不应影响原对象
		cp.Set("key1", "modified")
		if ctx.GetString("key1") != "value1" {
			t.Error("original should not be modified")
		}
	})
}

// TestContextInheritance 测试 Context 继承
func TestContextInheritance(t *testing.T) {
	t.Run("继承 context.Context 的功能", func(t *testing.T) {
		parentCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ctx := AcquireWithContext(parentCtx)
		defer Release(ctx)

		// 验证可以使用 context.Context 的方法
		select {
		case <-ctx.Done():
			t.Error("context should not be done")
		default:
			// 正常
		}

		// 取消父 context
		cancel()

		// 验证子 context 也被取消
		select {
		case <-ctx.Done():
			// 正常
		case <-time.After(100 * time.Millisecond):
			t.Error("context should be done after cancel")
		}
	})

	t.Run("继承 context.WithTimeout", func(t *testing.T) {
		parentCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		ctx := AcquireWithContext(parentCtx)
		defer Release(ctx)

		// 等待超时
		time.Sleep(100 * time.Millisecond)

		select {
		case <-ctx.Done():
			if ctx.Err() != context.DeadlineExceeded {
				t.Errorf("expected DeadlineExceeded, got %v", ctx.Err())
			}
		default:
			t.Error("context should be done after timeout")
		}
	})
}

// TestErrorsReturnsCopy 测试 Errors() 返回副本
func TestErrorsReturnsCopy(t *testing.T) {
	ctx := AcquireWithContext(context.Background())
	defer Release(ctx)

	t.Run("修改返回的 errors 不应影响内部状态", func(t *testing.T) {
		err1 := errors.New("error1")
		ctx.AddError(err1)

		errs := ctx.Errors()
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d", len(errs))
		}

		// 尝试修改返回的切片
		errs[0] = errors.New("modified")

		// 再次获取，应该还是原来的错误
		errs2 := ctx.Errors()
		if errs2[0].Error() != err1.Error() {
			t.Error("internal errors should not be modified")
		}
	})
}

// TestResetClearsAllFields 测试 reset 清理所有字段
func TestResetClearsAllFields(t *testing.T) {
	t.Run("reset 应该清理所有字段", func(t *testing.T) {
		ctx := AcquireWithContext(context.Background())
		
		// 设置所有字段
		ctx.RequestID = "req-123"
		ctx.UserID = "user-123"
		ctx.TenantID = "tenant-123"
		ctx.TraceID = "trace-123"
		ctx.Set("key1", "value1")
		ctx.Set("key2", "value2")
		ctx.AddError(errors.New("error1"))

		// 释放（会调用 reset）
		Release(ctx)

		// 再次获取
		ctx2 := AcquireWithContext(context.Background())
		defer Release(ctx2)

		// 验证所有字段都被清理
		if ctx2.RequestID != "" {
			t.Error("RequestID not cleared")
		}
		if ctx2.UserID != "" {
			t.Error("UserID not cleared")
		}
		if ctx2.TenantID != "" {
			t.Error("TenantID not cleared")
		}
		if ctx2.TraceID != "" {
			t.Error("TraceID not cleared")
		}
		if _, ok := ctx2.Get("key1"); ok {
			t.Error("data not cleared")
		}
		if ctx2.HasErrors() {
			t.Error("errors not cleared")
		}
	})
}

// TestStartTime 测试 StartTime 设置
func TestStartTime(t *testing.T) {
	t.Run("Acquire 应该设置 StartTime", func(t *testing.T) {
		before := time.Now()
		
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/test", nil)
		
		ctx := Acquire(c)
		defer Release(ctx)
		
		after := time.Now()

		if ctx.StartTime.Before(before) || ctx.StartTime.After(after) {
			t.Error("StartTime not set correctly")
		}
	})

	t.Run("AcquireWithContext 应该设置 StartTime", func(t *testing.T) {
		before := time.Now()
		ctx := AcquireWithContext(context.Background())
		defer Release(ctx)
		after := time.Now()

		if ctx.StartTime.Before(before) || ctx.StartTime.After(after) {
			t.Error("StartTime not set correctly")
		}
	})
}
