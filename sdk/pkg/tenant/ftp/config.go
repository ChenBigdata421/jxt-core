package ftp

// TenantFtpConfig holds the FTP configuration for a tenant.
// This includes authentication credentials, home directory, and permission settings.
type TenantFtpConfig struct {
	// Tenant identification
	TenantID int    `json:"tenant_id"`
	Code     string `json:"code"`
	Name     string `json:"name"`

	// FTP authentication and access
	Username        string `json:"username"`
	PasswordHash    string `json:"password_hash"`
	HomeDirectory   string `json:"home_directory"`
	WritePermission bool   `json:"write_permission"`
}
