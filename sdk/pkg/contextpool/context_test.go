package contextpool

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestContextPool(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Acquire and Release", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/test", nil)
		c.Set("userID", "user-123")
		c.Set("tenantID", "tenant-456")

		ctx := Acquire(c)
		defer Release(ctx)

		if ctx.UserID != "user-123" {
			t.Errorf("expected userID 'user-123', got '%s'", ctx.UserID)
		}

		if ctx.TenantID != "tenant-456" {
			t.Errorf("expected tenantID 'tenant-456', got '%s'", ctx.TenantID)
		}
	})

	t.Run("Set and Get", func(t *testing.T) {
		ctx := AcquireWithContext(context.Background())
		defer Release(ctx)

		ctx.Set("key1", "value1")
		ctx.Set("key2", 123)
		ctx.Set("key3", true)

		if val := ctx.GetString("key1"); val != "value1" {
			t.Errorf("expected 'value1', got '%s'", val)
		}

		if val := ctx.GetInt("key2"); val != 123 {
			t.Errorf("expected 123, got %d", val)
		}

		if val := ctx.GetBool("key3"); !val {
			t.Error("expected true, got false")
		}
	})

	t.Run("Error handling", func(t *testing.T) {
		ctx := AcquireWithContext(context.Background())
		defer Release(ctx)

		if ctx.HasErrors() {
			t.Error("expected no errors initially")
		}

		ctx.AddError(context.Canceled)
		ctx.AddError(context.DeadlineExceeded)

		if !ctx.HasErrors() {
			t.Error("expected errors")
		}

		if len(ctx.Errors()) != 2 {
			t.Errorf("expected 2 errors, got %d", len(ctx.Errors()))
		}
	})

	t.Run("Copy", func(t *testing.T) {
		ctx := AcquireWithContext(context.Background())
		ctx.UserID = "user-123"
		ctx.Set("key", "value")

		cp := ctx.Copy()

		if cp.UserID != ctx.UserID {
			t.Error("copy failed: UserID mismatch")
		}

		if val := cp.GetString("key"); val != "value" {
			t.Error("copy failed: data mismatch")
		}

		// 修改副本不应影响原对象
		cp.UserID = "user-456"
		if ctx.UserID == "user-456" {
			t.Error("copy is not independent")
		}

		Release(ctx)
	})

	t.Run("Reset", func(t *testing.T) {
		ctx := AcquireWithContext(context.Background())
		ctx.UserID = "user-123"
		ctx.Set("key", "value")
		ctx.AddError(context.Canceled)

		Release(ctx)

		// 从池中再次获取
		ctx2 := AcquireWithContext(context.Background())
		defer Release(ctx2)

		// 应该是干净的
		if ctx2.UserID != "" {
			t.Error("context not properly reset")
		}

		if _, ok := ctx2.Get("key"); ok {
			t.Error("data not properly cleared")
		}

		if ctx2.HasErrors() {
			t.Error("errors not properly cleared")
		}
	})
}

func BenchmarkContextPool(b *testing.B) {
	b.Run("WithPool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ctx := AcquireWithContext(context.Background())
			ctx.Set("key", "value")
			Release(ctx)
		}
	})

	b.Run("WithoutPool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ctx := &Context{
				Context: context.Background(),
				data:    make(map[string]interface{}),
			}
			ctx.Set("key", "value")
			_ = ctx
		}
	})
}
