// Package cache provides persistent file-based caching for tenant configuration.
//
// The FileCache component enables the tenant Provider to degrade gracefully
// when ETCD is unavailable by maintaining a local copy of tenant data.
//
// Features:
//   - Atomic writes using temporary file + rename
//   - JSON encoding for human-readable cache files
//   - Configurable cache path via TENANT_CACHE_PATH env variable
//   - Thread-safe operations with mutex protection
//
// Usage:
//
//	cache := cache.NewFileCache()
//	data, err := cache.Load()
//	if err == nil {
//	    // Use cached data as fallback
//	}
//
//	cache.Save(tenantData) // Persist on ETCD updates
package cache
