package contextpool

import (
	"context"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Context 业务上下文，支持对象池复用
type Context struct {
	context.Context

	// 基础字段
	RequestID string
	UserID    string
	TenantID  string
	TraceID   string

	// 时间信息
	StartTime time.Time

	// 自定义数据存储（使用 sync.Map 保证并发安全）
	data sync.Map

	// 错误收集
	errors []error
	mu     sync.RWMutex // 保护 errors 切片
}

// 全局对象池
var pool = sync.Pool{
	New: func() interface{} {
		return &Context{
			errors: make([]error, 0, 4),
		}
	},
}

// Acquire 从池中获取 Context
// 使用示例:
//   ctx := contextpool.Acquire(c)
//   defer contextpool.Release(ctx)
func Acquire(c *gin.Context) *Context {
	ctx := pool.Get().(*Context)
	ctx.Context = c.Request.Context()
	ctx.StartTime = time.Now()

	// 从 Gin Context 中提取常用信息
	ctx.RequestID = c.GetHeader("X-Request-ID")
	if ctx.RequestID == "" {
		ctx.RequestID = c.GetString("requestID")
	}

	ctx.UserID = c.GetString("userID")
	ctx.TenantID = c.GetString("tenantID")
	ctx.TraceID = c.GetHeader("X-Trace-ID")

	return ctx
}

// AcquireWithContext 从标准 context.Context 获取
func AcquireWithContext(parent context.Context) *Context {
	ctx := pool.Get().(*Context)
	ctx.Context = parent
	ctx.StartTime = time.Now()
	return ctx
}

// Release 释放 Context 回池中
func Release(ctx *Context) {
	if ctx == nil {
		return
	}
	ctx.reset()
	pool.Put(ctx)
}

// reset 重置所有字段
func (c *Context) reset() {
	c.Context = nil
	c.RequestID = ""
	c.UserID = ""
	c.TenantID = ""
	c.TraceID = ""
	c.StartTime = time.Time{}

	// 清空 sync.Map
	c.data.Range(func(key, value interface{}) bool {
		c.data.Delete(key)
		return true
	})

	// 清空 errors，但保留容量
	c.mu.Lock()
	c.errors = c.errors[:0]
	c.mu.Unlock()
}

// Set 设置自定义数据（并发安全）
func (c *Context) Set(key string, value interface{}) {
	c.data.Store(key, value)
}

// Get 获取自定义数据（并发安全）
func (c *Context) Get(key string) (interface{}, bool) {
	return c.data.Load(key)
}

// MustGet 获取自定义数据，不存在则 panic（并发安全）
func (c *Context) MustGet(key string) interface{} {
	if val, ok := c.data.Load(key); ok {
		return val
	}
	panic("key '" + key + "' does not exist")
}

// GetString 获取字符串类型数据（并发安全）
func (c *Context) GetString(key string) string {
	if val, ok := c.data.Load(key); ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// GetInt 获取整数类型数据（并发安全）
func (c *Context) GetInt(key string) int {
	if val, ok := c.data.Load(key); ok {
		if i, ok := val.(int); ok {
			return i
		}
	}
	return 0
}

// GetBool 获取布尔类型数据（并发安全）
func (c *Context) GetBool(key string) bool {
	if val, ok := c.data.Load(key); ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// AddError 添加错误（并发安全）
func (c *Context) AddError(err error) {
	if err != nil {
		c.mu.Lock()
		c.errors = append(c.errors, err)
		c.mu.Unlock()
	}
}

// Errors 获取所有错误（并发安全）
func (c *Context) Errors() []error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// 返回副本，避免外部修改
	errs := make([]error, len(c.errors))
	copy(errs, c.errors)
	return errs
}

// HasErrors 是否有错误（并发安全）
func (c *Context) HasErrors() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.errors) > 0
}

// Duration 获取执行时长
func (c *Context) Duration() time.Duration {
	return time.Since(c.StartTime)
}

// Copy 创建副本（用于 goroutine）
// 注意：副本不会被自动回收到池中
func (c *Context) Copy() *Context {
	cp := &Context{
		Context:   c.Context,
		RequestID: c.RequestID,
		UserID:    c.UserID,
		TenantID:  c.TenantID,
		TraceID:   c.TraceID,
		StartTime: c.StartTime,
	}

	// 复制 data（从 sync.Map）
	c.data.Range(func(key, value interface{}) bool {
		cp.data.Store(key, value)
		return true
	})

	// 复制 errors
	c.mu.RLock()
	cp.errors = make([]error, len(c.errors))
	copy(cp.errors, c.errors)
	c.mu.RUnlock()

	return cp
}
