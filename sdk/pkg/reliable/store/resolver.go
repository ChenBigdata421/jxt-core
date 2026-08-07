package store

import "gorm.io/gorm"

// TenantStoreResolver is the per-tenant Store + QuarantineStore + *gorm.DB lookup
// contract that the reliable DLQ adapter (sdk/pkg/reliable/adapters/eventbus) and
// the per-service StoreCache both satisfy. It is the promoted shape of file-storage's
// unexported cachePort (file-storage-service/internal/infrastructure/reliable/eventbus_dlq_adapter.go),
// verified identical against both file-storage's and evidence-management's StoreCache.
//
// Why this lives HERE (Q2=A, controller-locked) and not inside adapters/eventbus:
// opsvc (Task B2) must be able to inject a resolver WITHOUT importing the
// adapters/eventbus package — that package imports eventbus, which imports sarama
// (github.com/IBM/sarama). Hosting the interface in `store` keeps sarama out of
// any resolver-only consumer's dependency graph. The `store` package already
// imports gorm.io/gorm (its Store interface has *gorm.DB params), so adding this
// interface introduces NO new dependency and NO import cycle (store does not
// import adapters/eventbus or eventbus).
//
// Trust boundary (§7): a tenant is "served" iff Store returns a non-nil *gorm.DB
// for it. An unserved tenant must surface as a non-nil error from BOTH methods so
// the DLQ adapter can fail closed (strategy-A) rather than silently drop the
// poison record.
type TenantStoreResolver interface {
	// Store returns the memoized reliable Store for the tenant and the raw tenant
	// *gorm.DB (the db passed to Mark*/RecordTerminal). Fails closed (non-nil err)
	// if the tenant is not served by this process.
	Store(tenantID int) (Store, *gorm.DB, error)

	// QuarantineStore returns the memoized QuarantineStore for the tenant. Fails
	// closed (non-nil err) if the tenant is not served.
	QuarantineStore(tenantID int) (QuarantineStore, error)
}
