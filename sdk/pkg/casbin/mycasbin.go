package mycasbin

// Package mycasbin provides Casbin enforcer setup for multi-tenant environments.
//
// SetupWithProvider is the primary entry point for microservices that load policies
// from a remote gRPC provider. SetupRedisWatcherForEnforcer attaches a per-tenant
// Redis watcher for cross-instance policy synchronization.
//
// Local-DB callers (security-management) should use the standalone SetupForTenant
// in security-management/internal/infrastructure/casbin instead. It was moved out
// of this package to keep infrastructure concerns (GORM, local DB schema) out of
// the shared library.
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

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// Initialize the model from a string.
// ModelText is the canonical RBAC model for Casbin enforcers. Exported so
// security-management's local SetupForTenant reuses the same model instead
// of copying (avoids model drift between local-DB and remote-provider paths).
const ModelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && (keyMatch2(r.obj, p.obj) || keyMatch(r.obj, p.obj)) && (r.act == p.act || p.act == "*")
`

// SetupRedisWatcherForEnforcer sets up the CasbinWatcherMux watcher for an enforcer.
// Called by both SetupForTenant and SetupWithProvider to eliminate code duplication.
//
// Behavior:
//   - Lazily initialises the mux from the configured Redis clients on first use
//     (idempotent), so callers don't need to wire InitCasbinWatcherMux manually
//   - If Redis is not configured, degrades gracefully (no cross-instance sync)
//   - If Client #1 is configured but the subscriber Client #3 is missing, logs a
//     warning so operators can tell that policy sync is silently disabled
func SetupRedisWatcherForEnforcer(e *casbin.SyncedEnforcer, tenantID int) {
	m := ensureWatcherMux()
	if m == nil {
		return // Redis not configured — graceful degradation
	}

	w := m.NewWatcher(tenantID)
	if err := e.SetWatcher(w); err != nil {
		logger.Errorf("租户 %d 设置 Watcher 失败: %v", tenantID, err)
	}
}

// ensureWatcherMux returns the singleton mux, lazily initialising it from the
// configured Redis clients on first use. Returns nil when Redis is not
// configured (graceful degradation). InitCasbinWatcherMux is guarded by
// sync.Once, so concurrent enforcer setup is safe.
func ensureWatcherMux() *CasbinWatcherMux {
	if m := loadWatcherMux(); m != nil {
		return m
	}
	pub := config.GetRedisClient()
	if pub == nil {
		return nil // Redis not configured — graceful degradation
	}
	sub := config.GetSubscriberClient()
	if sub == nil {
		logger.Warnf("CasbinWatcherMux: Redis Client #1 configured but subscriber Client #3 is nil; " +
			"cross-instance Casbin policy sync is disabled")
		return nil
	}
	InitCasbinWatcherMux(pub, sub)
	return loadWatcherMux()
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
	m, err := model.NewModelFromString(ModelText) // ModelText is defined in mycasbin.go
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
	SetupRedisWatcherForEnforcer(e, tenantID)

	// 6. Enable logging
	log.SetLogger(&Logger{})
	e.EnableLog(true)

	return e, nil
}
