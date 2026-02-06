package mycasbin

// Package mycasbin provides Casbin enforcer setup for multi-tenant environments.
//
// The Setup function (deprecated) creates a single global enforcer using sync.Once,
// which causes all tenants to share the same enforcer instance. This is a known
// issue and will be fixed in future stages.
//
// The SetupForTenant function is the new recommended approach, creating independent
// enforcer instances per tenant with proper error handling.
//
// Migration Guide:
//   - Old: enforcer := mycasbin.Setup(db, "")
//   - New: enforcer, err := mycasbin.SetupForTenant(db, tenantID)
//
// Stage 2 will add Redis Watcher support for per-tenant policy synchronization.
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
// 每个租户拥有独立的 adapter 和 enforcer 实例
// 参数:
//   - db: 该租户的数据库连接
//   - tenantID: 租户ID（用于日志标识和后续 Redis Watcher 频道隔离）
// 返回:
//   - *casbin.SyncedEnforcer: 该租户专属的 enforcer 实例
//   - error: 错误信息
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

	// 5. 设置 Redis Watcher（如果 Redis 已配置）
	if config.CacheConfig.Redis != nil {
		// 每个租户使用独立的 Redis 频道
		channel := fmt.Sprintf("/casbin/tenant/%d", tenantID)

		w, err := redisWatcher.NewWatcher(config.CacheConfig.Redis.Addr, redisWatcher.WatcherOptions{
			Options: redis.Options{
				Network:  "tcp",
				Password: config.CacheConfig.Redis.Password,
			},
			Channel:    channel, // 租户专属频道
			IgnoreSelf: false,
		})
		if err != nil {
			// Watcher 失败不应阻止 enforcer 创建
			logger.Errorf("租户 %d Redis Watcher 创建失败: %v", tenantID, err)
		} else {
			// 创建租户专属的 callback 闭包
			tenantEnforcer := e // 捕获当前租户的 enforcer
			callback := func(msg string) {
				logger.Infof("casbin updateCallback (租户 %d) msg: %v", tenantID, msg)
				if err := tenantEnforcer.LoadPolicy(); err != nil {
					logger.Errorf("casbin LoadPolicy (租户 %d) err: %v", tenantID, err)
				}
			}

			if err := w.SetUpdateCallback(callback); err != nil {
				logger.Errorf("租户 %d 设置 Watcher callback 失败: %v", tenantID, err)
			}
			if err := e.SetWatcher(w); err != nil {
				logger.Errorf("租户 %d 设置 Watcher 失败: %v", tenantID, err)
			}
		}
	}

	// 6. 设置日志
	log.SetLogger(&Logger{})
	e.EnableLog(true)

	return e, nil
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

// updateCallback is kept for backward compatibility but is now a no-op
// In Stage 2, each tenant will have its own callback closure
// Deprecated: This function no longer reloads policy since the global enforcer was removed
func updateCallback(msg string) {
	logger.Infof("casbin updateCallback msg: %v (警告: 全局 enforcer 已移除，此回调不再生效，请使用 SetupForTenant)", msg)
	// 不再执行 LoadPolicy，因为全局 enforcer 已不存在
	// 在 Stage 2 中，每个租户将有自己的 callback 闭包
}
