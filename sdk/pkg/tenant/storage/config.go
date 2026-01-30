package storage

// TenantStorageConfig represents storage configuration for a tenant.
// This includes storage quotas, file size limits, and upload restrictions.
type TenantStorageConfig struct {
	// Tenant identification
	TenantID int `json:"tenant_id"`
	Code     string `json:"code"`
	Name     string `json:"name"`

	// Storage limits
	QuotaBytes          int64 `json:"quota_bytes"`           // Total storage quota in bytes
	MaxFileSizeBytes    int64 `json:"max_file_size_bytes"`   // Maximum individual file size in bytes
	MaxConcurrentUploads int   `json:"max_concurrent_uploads"` // Maximum number of concurrent uploads
}
