package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"go.uber.org/zap"
)

// ExampleBusinessHealthChecker 示例业务健康检查器
type ExampleBusinessHealthChecker struct {
	serviceName string
	version     string
}

// NewExampleBusinessHealthChecker 创建示例业务健康检查器
func NewExampleBusinessHealthChecker(serviceName, version string) *ExampleBusinessHealthChecker {
	return &ExampleBusinessHealthChecker{
		serviceName: serviceName,
		version:     version,
	}
}

// CheckBusinessHealth 检查业务健康状态
func (e *ExampleBusinessHealthChecker) CheckBusinessHealth(ctx context.Context) error {
	// 示例：检查业务逻辑
	// 这里可以添加具体的业务健康检查逻辑
	// 例如：检查数据库连接、检查外部服务、检查业务队列等

	// 模拟业务检查
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		// 模拟检查完成
	}

	return nil
}

// GetBusinessMetrics 获取业务指标
func (e *ExampleBusinessHealthChecker) GetBusinessMetrics() interface{} {
	return map[string]interface{}{
		"serviceName":  e.serviceName,
		"version":      e.version,
		"uptime":       time.Since(time.Now().Add(-1 * time.Hour)).String(),
		"requestCount": 12345,
		"errorRate":    0.01,
		"responseTime": "50ms",
	}
}

// GetBusinessConfig 获取业务配置
func (e *ExampleBusinessHealthChecker) GetBusinessConfig() interface{} {
	return map[string]interface{}{
		"serviceName": e.serviceName,
		"version":     e.version,
		"environment": "production",
	}
}

// HealthCheckHandler HTTP 健康检查处理器
type HealthCheckHandler struct {
	eventBus eventbus.EventBus
	logger   *zap.Logger
}

// NewHealthCheckHandler 创建健康检查处理器
func NewHealthCheckHandler(eventBus eventbus.EventBus) *HealthCheckHandler {
	return &HealthCheckHandler{
		eventBus: eventBus,
		logger:   logger.Logger,
	}
}

// HandleHealthCheck 处理健康检查请求
func (h *HealthCheckHandler) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	// 执行健康检查
	var status *eventbus.HealthStatus
	var err error

	if healthChecker, ok := h.eventBus.(eventbus.EventBusHealthChecker); ok {
		// 获取聚合健康状态
		healthStatus := healthChecker.GetHealthStatus()
		if healthStatus.IsHealthy {
			status = &eventbus.HealthStatus{
				Overall:   "healthy",
				Timestamp: time.Now(),
				Infrastructure: eventbus.InfrastructureHealth{
					EventBus: eventbus.EventBusHealthMetrics{
						ConnectionStatus: "connected",
						LastSuccessTime:  healthStatus.LastSuccessTime,
					},
				},
			}
		} else {
			status = &eventbus.HealthStatus{
				Overall:   "unhealthy",
				Timestamp: time.Now(),
				Infrastructure: eventbus.InfrastructureHealth{
					EventBus: eventbus.EventBusHealthMetrics{
						ConnectionStatus:    "disconnected",
						LastFailureTime:     healthStatus.LastFailureTime,
						ConsecutiveFailures: healthStatus.ConsecutiveFailures,
					},
				},
			}
			err = fmt.Errorf("eventbus is unhealthy")
		}
	} else {
		// 无法获取健康状态
		status = &eventbus.HealthStatus{
			Overall:   "unknown",
			Timestamp: time.Now(),
			Infrastructure: eventbus.InfrastructureHealth{
				EventBus: eventbus.EventBusHealthMetrics{
					ConnectionStatus: "unknown",
				},
			},
		}
		err = fmt.Errorf("eventbus does not support health checking")
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// 根据健康状态设置 HTTP 状态码
	if err != nil || status.Overall != "healthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
		h.logger.Warn("Health check failed", zap.Error(err))
	} else {
		w.WriteHeader(http.StatusOK)
		h.logger.Debug("Health check passed")
	}

	// 返回健康状态
	if err := json.NewEncoder(w).Encode(status); err != nil {
		h.logger.Error("Failed to encode health status", zap.Error(err))
	}
}

// HandleLiveness 处理存活检查请求（简化版健康检查）
func (h *HealthCheckHandler) HandleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "alive",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(response)
}

// HandleReadiness 处理就绪检查请求
func (h *HealthCheckHandler) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 检查 EventBus 健康状态
	var isReady bool
	if healthChecker, ok := h.eventBus.(eventbus.EventBusHealthChecker); ok {
		healthStatus := healthChecker.GetHealthStatus()
		isReady = healthStatus.IsHealthy
	} else {
		// 无法获取健康状态，假设未就绪
		isReady = false
	}

	if !isReady {
		w.WriteHeader(http.StatusServiceUnavailable)
		response := map[string]interface{}{
			"status":    "not ready",
			"error":     "eventbus is not healthy",
			"timestamp": time.Now().Format(time.RFC3339),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	w.WriteHeader(http.StatusOK)
	response := map[string]interface{}{
		"status":    "ready",
		"timestamp": time.Now().Format(time.RFC3339),
	}
	json.NewEncoder(w).Encode(response)
}

// SetupHealthCheckRoutes 设置健康检查路由
func SetupHealthCheckRoutes(mux *http.ServeMux, eventBus eventbus.EventBus) {
	handler := NewHealthCheckHandler(eventBus)

	// 完整健康检查
	mux.HandleFunc("/health", handler.HandleHealthCheck)
	mux.HandleFunc("/healthz", handler.HandleHealthCheck)

	// 存活检查
	mux.HandleFunc("/livez", handler.HandleLiveness)
	mux.HandleFunc("/alive", handler.HandleLiveness)

	// 就绪检查
	mux.HandleFunc("/readyz", handler.HandleReadiness)
	mux.HandleFunc("/ready", handler.HandleReadiness)
}

// ExampleUsage 使用示例
func ExampleUsage() {
	// 1. 创建 EventBus
	config := &eventbus.EventBusConfig{
		Type: "kafka",
		// ... 其他配置
	}

	eventBus, err := eventbus.NewEventBus(config)
	if err != nil {
		panic(err)
	}

	// 2. 创建业务健康检查器
	businessChecker := NewExampleBusinessHealthChecker("my-service", "v1.0.0")

	// 3. 注册业务健康检查器
	if healthChecker, ok := eventBus.(eventbus.EventBusHealthChecker); ok {
		healthChecker.RegisterBusinessHealthCheck(businessChecker)
	}

	// 4. 设置 HTTP 健康检查端点
	mux := http.NewServeMux()
	SetupHealthCheckRoutes(mux, eventBus)

	// 5. 启动 HTTP 服务器
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	fmt.Println("Health check endpoints available at:")
	fmt.Println("  http://localhost:8080/health   - 完整健康检查")
	fmt.Println("  http://localhost:8080/livez    - 存活检查")
	fmt.Println("  http://localhost:8080/readyz   - 就绪检查")

	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}
