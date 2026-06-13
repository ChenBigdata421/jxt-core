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
//   - Uses CasbinWatcherMux: single PSUBSCRIBE connection (Client #3) for all tenants
//   - Each tenant uses a dedicated Redis channel: /casbin/tenant/{tenantID}
//   - Policy changes are automatically synchronized across instances via Redis pub/sub
//   - Watcher failures do not prevent enforcer creation (graceful degradation)
import (
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/log"
	"github.com/casbin/casbin/v2/model"
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
//
// ⚠️  仅供 security-management 使用（本地数据库模式）
// 其他微服务请使用 SetupWithProvider
//
// 参数:
//   - db: 该租户的数据库连接
//   - tenantID: 租户ID（用于日志标识和 Redis Watcher 频道隔离）
//
// 返回:
//   - *casbin.SyncedEnforcer: 该租户专属的 enforcer 实例
//   - error: 错误信息
//
// Redis Watcher:
//   - 使用 CasbinWatcherMux: 单一 PSUBSCRIBE 连接监听所有租户
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

	// 5. 设置 Redis Watcher（使用 CasbinWatcherMux）
	setupRedisWatcherForEnforcer(e, tenantID)

	// 6. 设置日志
	log.SetLogger(&Logger{})
	e.EnableLog(true)

	return e, nil
}

// setupRedisWatcherForEnforcer sets up the CasbinWatcherMux watcher for an enforcer.
// Called by both SetupForTenant and SetupWithProvider to eliminate code duplication.
//
// Behavior:
//   - If CasbinWatcherMux is initialised, creates a per-tenant watcher handle via mux
//   - If mux is nil (Redis not configured), logs nothing — graceful degradation
func setupRedisWatcherForEnforcer(e *casbin.SyncedEnforcer, tenantID int) {
	m := loadWatcherMux()
	if m == nil {
		return // Redis not configured — graceful degradation
	}

	w := m.NewWatcher(tenantID)
	if err := e.SetWatcher(w); err != nil {
		logger.Errorf("租户 %d 设置 Watcher 失败: %v", tenantID, err)
	}
}

// SetupWithProvider creates a Casbin enforcer using a PolicyProvider (for microservices)
// Unlike SetupForTenant, this does not require a local database connection
//
// ⚠️  For non-security-management microservices only (remote fetch mode)
// security-management should continue using SetupForTenant
//
// Parameters:
//   - provider: Policy provider (implemented by microservice, typically a gRPC adapter)
//   - tenantID: Tenant identifier
//
// Returns:
//   - *casbin.SyncedEnforcer: Enforcer with loaded policies
//   - error: Initialization error
func SetupWithProvider(provider PolicyProvider, tenantID int) (*casbin.SyncedEnforcer, error) {
	// 1. Create ProviderAdapter
	adapter := NewProviderAdapter(provider, tenantID)

	// 2. Load the same RBAC model as SetupForTenant
	m, err := model.NewModelFromString(text) // text variable is already defined in mycasbin.go
	if err != nil {
		return nil, fmt.Errorf("failed to create casbin model: %w", err)
	}

	// 3. Create SyncedEnforcer
	e, err := casbin.NewSyncedEnforcer(m, adapter)
	if err != nil {
		return nil, fmt.Errorf("failed to create casbin enforcer: %w", err)
	}

	// 4. Load policies (via ProviderAdapter → Provider → remote service)
	if err := e.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("failed to load policy via provider: %w", err)
	}

	// 5. Set up Redis Watcher using CasbinWatcherMux
	setupRedisWatcherForEnforcer(e, tenantID)

	// 6. Enable logging
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

	// SetupForTenant 已经为租户 0 设置了 Redis Watcher（使用 CasbinWatcherMux）
	// 不需要在这里重复设置

	return e
}
