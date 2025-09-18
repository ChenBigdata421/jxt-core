package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// RecoveryMode 恢复模式状态
type RecoveryMode int32

const (
	RecoveryModeNormal RecoveryMode = iota // 正常模式
	RecoveryModeActive                     // 恢复模式激活
)

// RecoveryModeCallback 恢复模式回调函数
type RecoveryModeCallback func(ctx context.Context, mode RecoveryMode, reason string) error

// RecoveryManager 恢复模式管理器
type RecoveryManager struct {
	config           config.RecoveryModeConfig
	currentMode      int32 // RecoveryMode
	backlogDetector  *BacklogDetector
	aggregateManager *AggregateProcessorManager

	// 状态管理
	transitionCount    int32
	lastTransitionTime atomic.Value // time.Time

	// 回调管理
	callbacks  []RecoveryModeCallback
	callbackMu sync.RWMutex

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 监控
	monitorTicker *time.Ticker
	isMonitoring  atomic.Bool
}

// NewRecoveryManager 创建恢复模式管理器
func NewRecoveryManager(
	config config.RecoveryModeConfig,
	backlogDetector *BacklogDetector,
	aggregateManager *AggregateProcessorManager,
) *RecoveryManager {
	rm := &RecoveryManager{
		config:           config,
		currentMode:      int32(RecoveryModeNormal),
		backlogDetector:  backlogDetector,
		aggregateManager: aggregateManager,
	}

	rm.lastTransitionTime.Store(time.Now())

	// 设置默认配置
	if rm.config.TransitionThreshold == 0 {
		rm.config.TransitionThreshold = 3
	}

	return rm
}

// Start 启动恢复模式管理器
func (rm *RecoveryManager) Start(ctx context.Context) error {
	if !rm.config.Enabled {
		logger.Info("Recovery mode is disabled")
		return nil
	}

	rm.ctx, rm.cancel = context.WithCancel(ctx)

	// 如果启用自动检测，注册积压检测回调
	if rm.config.AutoDetection && rm.backlogDetector != nil {
		rm.backlogDetector.RegisterCallback(rm.handleBacklogStateChange)
	}

	// 启动监控循环
	if rm.config.AutoDetection {
		rm.startMonitoring()
	}

	logger.Info("Recovery manager started",
		"enabled", rm.config.Enabled,
		"autoDetection", rm.config.AutoDetection,
		"transitionThreshold", rm.config.TransitionThreshold)

	return nil
}

// Stop 停止恢复模式管理器
func (rm *RecoveryManager) Stop() error {
	if rm.cancel != nil {
		rm.cancel()
	}

	rm.stopMonitoring()
	rm.wg.Wait()

	logger.Info("Recovery manager stopped")
	return nil
}

// SetRecoveryMode 设置恢复模式
func (rm *RecoveryManager) SetRecoveryMode(mode RecoveryMode, reason string) error {
	oldMode := RecoveryMode(atomic.LoadInt32(&rm.currentMode))
	if oldMode == mode {
		return nil // 模式没有变化
	}

	atomic.StoreInt32(&rm.currentMode, int32(mode))
	rm.lastTransitionTime.Store(time.Now())
	atomic.AddInt32(&rm.transitionCount, 1)

	logger.Info("Recovery mode changed",
		"oldMode", rm.modeToString(oldMode),
		"newMode", rm.modeToString(mode),
		"reason", reason,
		"transitionCount", atomic.LoadInt32(&rm.transitionCount))

	// 通知回调
	rm.notifyCallbacks(mode, reason)

	return nil
}

// GetRecoveryMode 获取当前恢复模式
func (rm *RecoveryManager) GetRecoveryMode() RecoveryMode {
	return RecoveryMode(atomic.LoadInt32(&rm.currentMode))
}

// IsInRecoveryMode 检查是否在恢复模式
func (rm *RecoveryManager) IsInRecoveryMode() bool {
	return rm.GetRecoveryMode() == RecoveryModeActive
}

// RegisterCallback 注册恢复模式回调
func (rm *RecoveryManager) RegisterCallback(callback RecoveryModeCallback) error {
	rm.callbackMu.Lock()
	defer rm.callbackMu.Unlock()
	rm.callbacks = append(rm.callbacks, callback)
	return nil
}

// handleBacklogStateChange 处理积压状态变化
func (rm *RecoveryManager) handleBacklogStateChange(ctx context.Context, state BacklogState) error {
	if !rm.config.AutoDetection {
		return nil
	}

	currentMode := rm.GetRecoveryMode()

	if state.HasBacklog {
		// 有积压，考虑进入恢复模式
		if currentMode == RecoveryModeNormal {
			// 检查是否达到转换阈值
			if rm.shouldEnterRecoveryMode(state) {
				return rm.SetRecoveryMode(RecoveryModeActive,
					fmt.Sprintf("backlog detected: lag=%d, lagTime=%v", state.LagCount, state.LagTime))
			}
		}
	} else {
		// 无积压，考虑退出恢复模式
		if currentMode == RecoveryModeActive {
			return rm.SetRecoveryMode(RecoveryModeNormal, "backlog cleared")
		}
	}

	return nil
}

// shouldEnterRecoveryMode 判断是否应该进入恢复模式
func (rm *RecoveryManager) shouldEnterRecoveryMode(state BacklogState) bool {
	// 这里可以实现更复杂的判断逻辑
	// 比如连续多次检测到积压才进入恢复模式
	return state.LagCount > int64(rm.config.TransitionThreshold)
}

// startMonitoring 启动监控
func (rm *RecoveryManager) startMonitoring() {
	if rm.isMonitoring.Load() {
		return
	}

	rm.monitorTicker = time.NewTicker(30 * time.Second) // 监控间隔
	rm.isMonitoring.Store(true)

	rm.wg.Add(1)
	go rm.monitoringLoop()
}

// stopMonitoring 停止监控
func (rm *RecoveryManager) stopMonitoring() {
	if !rm.isMonitoring.Load() {
		return
	}

	rm.isMonitoring.Store(false)
	if rm.monitorTicker != nil {
		rm.monitorTicker.Stop()
	}
}

// monitoringLoop 监控循环
func (rm *RecoveryManager) monitoringLoop() {
	defer rm.wg.Done()

	for {
		select {
		case <-rm.monitorTicker.C:
			rm.performHealthCheck()
		case <-rm.ctx.Done():
			return
		}
	}
}

// performHealthCheck 执行健康检查
func (rm *RecoveryManager) performHealthCheck() {
	// 检查聚合处理器状态
	if rm.aggregateManager != nil {
		stats := rm.aggregateManager.GetStats()
		logger.Debug("Recovery manager health check",
			"currentMode", rm.modeToString(rm.GetRecoveryMode()),
			"aggregateProcessorStats", stats)
	}
}

// notifyCallbacks 通知回调
func (rm *RecoveryManager) notifyCallbacks(mode RecoveryMode, reason string) {
	rm.callbackMu.RLock()
	callbacks := make([]RecoveryModeCallback, len(rm.callbacks))
	copy(callbacks, rm.callbacks)
	rm.callbackMu.RUnlock()

	for _, callback := range callbacks {
		go func(cb RecoveryModeCallback) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := cb(ctx, mode, reason); err != nil {
				logger.Error("Recovery mode callback failed",
					"mode", rm.modeToString(mode),
					"reason", reason,
					"error", err)
			}
		}(callback)
	}
}

// GetStats 获取恢复管理器统计信息
func (rm *RecoveryManager) GetStats() map[string]interface{} {
	lastTransition := rm.lastTransitionTime.Load().(time.Time)

	return map[string]interface{}{
		"enabled":             rm.config.Enabled,
		"autoDetection":       rm.config.AutoDetection,
		"currentMode":         rm.modeToString(rm.GetRecoveryMode()),
		"transitionThreshold": rm.config.TransitionThreshold,
		"transitionCount":     atomic.LoadInt32(&rm.transitionCount),
		"lastTransitionTime":  lastTransition,
		"isMonitoring":        rm.isMonitoring.Load(),
	}
}

// modeToString 将模式转换为字符串
func (rm *RecoveryManager) modeToString(mode RecoveryMode) string {
	switch mode {
	case RecoveryModeNormal:
		return "normal"
	case RecoveryModeActive:
		return "active"
	default:
		return "unknown"
	}
}

// GetLastTransitionTime 获取最后转换时间
func (rm *RecoveryManager) GetLastTransitionTime() time.Time {
	return rm.lastTransitionTime.Load().(time.Time)
}

// GetTransitionCount 获取转换次数
func (rm *RecoveryManager) GetTransitionCount() int32 {
	return atomic.LoadInt32(&rm.transitionCount)
}

// ForceRecoveryMode 强制设置恢复模式（用于测试或手动干预）
func (rm *RecoveryManager) ForceRecoveryMode(mode RecoveryMode, reason string) error {
	logger.Warn("Forcing recovery mode change",
		"mode", rm.modeToString(mode),
		"reason", reason)

	return rm.SetRecoveryMode(mode, fmt.Sprintf("forced: %s", reason))
}

// IsAutoDetectionEnabled 检查是否启用自动检测
func (rm *RecoveryManager) IsAutoDetectionEnabled() bool {
	return rm.config.AutoDetection
}

// GetConfig 获取配置
func (rm *RecoveryManager) GetConfig() config.RecoveryModeConfig {
	return rm.config
}
