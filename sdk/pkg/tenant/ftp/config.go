package ftp

// TenantFtpConfigDetail FTP配置详情
type TenantFtpConfigDetail struct {
	TenantID     int    `json:"tenant_id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Description  string `json:"description"`  // 新增
	Status       string `json:"status"`        // 新增
}
