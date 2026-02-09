package mycasbin

// Package mycasbin provides Casbin enforcer setup for multi-tenant environments.
//
// The Setup function (deprecated) creates a single global enforcer using sync.Once,
// which causes all tenants to share the same enforcer instance. This issue has been
// fixed in SetupForTenant.
//
// The SetupForTenant function is the recommended approach, creating independent
// enforcer instances per tenant with proper error handling and per-tenant Redis
// Watcher channels for policy synchronization.
//
// Migration Guide:
//   - Old: enforcer := mycasbin.Setup(db, "")
//   - New: enforcer, err := mycasbin.SetupForTenant(db, tenantID)
//
// Redis Watcher:
//   - Each tenant uses a dedicated Redis channel: /casbin/tenant/{tenantID}
//   - Policy changes are automatically synchronized across instances via Redis pub/sub
//   - Watcher failures do not prevent enforcer creation (graceful degradation)
import (
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/log"
	"github.com/casbin/casbin/v2/model"
	redisWatcher "github.com/go-admin-team/redis-watcher/v2"
	"github.com/go-redis/redis/v9"
	"gorm.io/gorm"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	gormAdapter "github.com/go-admin-team/gorm-adapter/v3"
)

// Initialize the model from a string.
var text = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && (keyMatch2(r.obj, p.obj) || keyMatch(r.obj, p.obj)) && (r.act == p.act || p.act == "*")
`

// SetupForTenant 为指定租户创建独立的 Casbin enforcer
// 每个租户拥有独立的 adapter、enforcer 实例和 Redis Watcher 频道
// 参数:
//   - db: 该租户的数据库连接
//   - tenantID: 租户ID（用于日志标识和 Redis Watcher 频道隔离）
// 返回:
//   - *casbin.SyncedEnforcer: 该租户专属的 enforcer 实例
//   - error: 错误信息
//
// Redis Watcher:
//   - 每个租户使用独立的 Redis 频道: /casbin/tenant/{tenantID}
//   - 当租户的权限策略变更时，通过 Redis pub/sub 自动通知所有实例重新加载策略
//   - Redis 不可用时，enforcer 仍能正常创建和使用（优雅降级）
func SetupForTenant(db *gorm.DB, tenantID int) (*casbin.SyncedEnforcer, error) {
	// 验证输入参数
	if db == nil {
		return nil, fmt.Errorf("创建 Casbin adapter 失败 (租户 %d): %w", tenantID, fmt.Errorf("数据库连接不能为空"))
	}

	// 1. 为该租户创建独立的 GORM Adapter
	adapter, err := gormAdapter.NewAdapterByDBUseTableName(db, "sys", "casbin_rule")
	if err != nil && err.Error() != "invalid DDL" {
		return nil, fmt.Errorf("创建 Casbin adapter 失败 (租户 %d): %w", tenantID, err)
	}

	// 2. 加载权限模型
	m, err := model.NewModelFromString(text)
	if err != nil {
		return nil, fmt.Errorf("加载 Casbin 模型失败: %w", err)
	}

	// 3. 创建该租户专属的 SyncedEnforcer
	e, err := casbin.NewSyncedEnforcer(m, adapter)
	if err != nil {
		return nil, fmt.Errorf("创建 Casbin enforcer 失败 (租户 %d): %w", tenantID, err)
	}

	// 4. 从该租户的数据库加载策略
	if err := e.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("加载 Casbin 策略失败 (租户 %d): %w", tenantID, err)
	}

	// 5. Set up Redis Watcher using shared helper
	setupRedisWatcherForEnforcer(e, tenantID)

	// 6. 设置日志
	log.SetLogger(&Logger{})
	e.EnableLog(true)

	return e, nil
}

// setupRedisWatcherForEnforcer sets up Redis Watcher for an enforcer
// Called by both SetupForTenant and SetupWithProvider to eliminate code duplication
//
// Behavior:
//   - If Redis is configured, creates Watcher and sets callback
//   - If Redis is not configured or creation fails, logs only (does not prevent enforcer creation)
//   - Each tenant uses a dedicated Redis channel: /casbin/tenant/{tenantID}
func setupRedisWatcherForEnforcer(e *casbin.SyncedEnforcer, tenantID int) {
	if config.CacheConfig.Redis == nil {
		return
	}

	channel := fmt.Sprintf("/casbin/tenant/%d", tenantID)

	w, err := redisWatcher.NewWatcher(config.CacheConfig.Redis.Addr, redisWatcher.WatcherOptions{
		Options: redis.Options{
			Network:  "tcp",
			Password: config.CacheConfig.Redis.Password,
		},
		Channel:    channel,
		IgnoreSelf: false,
	})
	if err != nil {
		logger.Errorf("租户 %d Redis Watcher 创建失败: %v", tenantID, err)
		return
	}

	// Capture enforcer in closure for this tenant
	tenantEnforcer := e
	_ = w.SetUpdateCallback(func(msg string) {
		logger.Infof("casbin updateCallback (租户 %d) msg: %v", tenantID, msg)
		if err := tenantEnforcer.LoadPolicy(); err != nil {
			logger.Errorf("casbin LoadPolicy (租户 %d) err: %v", tenantID, err)
		}
	})
	_ = e.SetWatcher(w)
}

// Setup 为指定租户创建 Casbin enforcer（向后兼容函数）
// 注意: 此函数保留用于向后兼容，新代码应使用 SetupForTenant
// Deprecated: 使用 SetupForTenant 替代，以获得更好的错误处理和多租户支持
func Setup(db *gorm.DB, _ string) *casbin.SyncedEnforcer {
	e, err := SetupForTenant(db, 0)
	if err != nil {
		// 保持原有行为：发生错误时 panic
		panic(err)
	}

	// SetupForTenant 已经为租户 0 创建了 Redis Watcher（使用 /casbin/tenant/0 频道）
	// 不需要在这里重复设置

	return e
}
