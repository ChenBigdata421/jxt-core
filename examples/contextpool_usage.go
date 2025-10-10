package main

import (
	"fmt"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/contextpool"
	"github.com/gin-gonic/gin"
)

// 演示如何使用 contextpool

func runContextPoolExample() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 示例 1: 基本使用
	r.GET("/basic", func(c *gin.Context) {
		ctx := contextpool.Acquire(c)
		defer contextpool.Release(ctx)

		// 设置业务数据
		ctx.Set("orderID", "order-12345")
		ctx.Set("amount", 99.99)

		// 获取数据
		orderID := ctx.GetString("orderID")
		amount := ctx.Get("amount")

		c.JSON(200, gin.H{
			"orderID":   orderID,
			"amount":    amount,
			"userID":    ctx.UserID,
			"requestID": ctx.RequestID,
			"duration":  ctx.Duration().String(),
		})
	})

	// 示例 2: 错误处理
	r.GET("/errors", func(c *gin.Context) {
		ctx := contextpool.Acquire(c)
		defer contextpool.Release(ctx)

		// 模拟业务处理
		if err := validateOrder(ctx); err != nil {
			ctx.AddError(err)
		}

		if err := processPayment(ctx); err != nil {
			ctx.AddError(err)
		}

		if ctx.HasErrors() {
			c.JSON(500, gin.H{
				"success": false,
				"errors":  ctx.Errors(),
			})
			return
		}

		c.JSON(200, gin.H{
			"success": true,
			"msg":     "订单处理成功",
		})
	})

	// 示例 3: 在 goroutine 中使用
	r.GET("/async", func(c *gin.Context) {
		ctx := contextpool.Acquire(c)
		defer contextpool.Release(ctx)

		ctx.UserID = c.GetString("userID")
		ctx.Set("orderID", "order-67890")

		// 创建副本用于异步处理
		ctxCopy := ctx.Copy()
		go func() {
			// 模拟异步任务
			time.Sleep(100 * time.Millisecond)
			orderID := ctxCopy.GetString("orderID")
			fmt.Printf("异步处理订单: %s, 用户: %s\n", orderID, ctxCopy.UserID)
		}()

		c.JSON(200, gin.H{
			"msg": "异步任务已提交",
		})
	})

	// 示例 4: 多层调用
	r.GET("/multi-layer", func(c *gin.Context) {
		ctx := contextpool.Acquire(c)
		defer contextpool.Release(ctx)

		ctx.UserID = "user-123"
		ctx.TenantID = "tenant-456"

		// 调用业务层
		result, err := businessLayer(ctx)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		c.JSON(200, gin.H{
			"result":   result,
			"duration": ctx.Duration().String(),
		})
	})

	// 示例 5: 性能对比
	r.GET("/benchmark", func(c *gin.Context) {
		// 使用对象池
		start1 := time.Now()
		for i := 0; i < 10000; i++ {
			ctx := contextpool.Acquire(c)
			ctx.Set("key", "value")
			contextpool.Release(ctx)
		}
		duration1 := time.Since(start1)

		// 不使用对象池
		start2 := time.Now()
		for i := 0; i < 10000; i++ {
			ctx := &contextpool.Context{}
			ctx.Set("key", "value")
			_ = ctx
		}
		duration2 := time.Since(start2)

		c.JSON(200, gin.H{
			"withPool":    duration1.String(),
			"withoutPool": duration2.String(),
			"improvement": fmt.Sprintf("%.2f%%", float64(duration2-duration1)/float64(duration2)*100),
		})
	})

	fmt.Println("=== Context Pool 示例服务启动 ===")
	fmt.Println("访问以下端点测试:")
	fmt.Println("  http://localhost:8080/basic        - 基本使用")
	fmt.Println("  http://localhost:8080/errors       - 错误处理")
	fmt.Println("  http://localhost:8080/async        - 异步处理")
	fmt.Println("  http://localhost:8080/multi-layer  - 多层调用")
	fmt.Println("  http://localhost:8080/benchmark    - 性能对比")
	fmt.Println()

	r.Run(":8080")
}

// 业务层函数
func businessLayer(ctx *contextpool.Context) (string, error) {
	// 调用数据层
	data, err := dataLayer(ctx)
	if err != nil {
		return "", err
	}

	// 业务处理
	result := fmt.Sprintf("处理结果: %s, 用户: %s, 租户: %s",
		data, ctx.UserID, ctx.TenantID)

	return result, nil
}

// 数据层函数
func dataLayer(ctx *contextpool.Context) (string, error) {
	// 模拟数据库查询
	time.Sleep(10 * time.Millisecond)
	return "数据查询成功", nil
}

// 验证订单
func validateOrder(ctx *contextpool.Context) error {
	orderID := ctx.GetString("orderID")
	if orderID == "" {
		return fmt.Errorf("订单ID不能为空")
	}
	return nil
}

// 处理支付
func processPayment(ctx *contextpool.Context) error {
	amount, ok := ctx.Get("amount")
	if !ok {
		return fmt.Errorf("支付金额不能为空")
	}

	if amt, ok := amount.(float64); ok && amt <= 0 {
		return fmt.Errorf("支付金额必须大于0")
	}

	return nil
}
