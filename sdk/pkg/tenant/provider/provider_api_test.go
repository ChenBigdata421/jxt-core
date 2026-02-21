package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProvider_GetServiceDatabaseConfig tests the GetServiceDatabaseConfig method
func TestProvider_GetServiceDatabaseConfig(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add meta
	metaJSON := `{"id":1,"code":"test_tenant","name":"Test Tenant","status":"active"}`
	metaKey := "tenants/1/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Add database config
	dbJSON := `{"tenantId":1,"serviceCode":"evidence-command","driver":"mysql","database":"testdb","host":"localhost","port":3306}`
	dbKey := "tenants/1/database/evidence-command"
	if err := p.parseServiceDatabaseConfig(dbKey, dbJSON, data); err != nil {
		t.Fatalf("parseServiceDatabaseConfig failed: %v", err)
	}

	p.data.Store(data)

	tests := []struct {
		name        string
		tenantID    int
		serviceCode string
		wantFound   bool
		wantDriver  string
	}{
		{
			name:        "existing config",
			tenantID:    1,
			serviceCode: "evidence-command",
			wantFound:   true,
			wantDriver:  "mysql",
		},
		{
			name:        "non-existent tenant",
			tenantID:    999,
			serviceCode: "evidence-command",
			wantFound:   false,
		},
		{
			name:        "non-existent service",
			tenantID:    1,
			serviceCode: "non-existent",
			wantFound:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := p.GetServiceDatabaseConfig(tt.tenantID, tt.serviceCode)
			if ok != tt.wantFound {
				t.Errorf("GetServiceDatabaseConfig() ok = %v, want %v", ok, tt.wantFound)
			}
			if tt.wantFound && cfg.Driver != tt.wantDriver {
				t.Errorf("GetServiceDatabaseConfig() driver = %v, want %v", cfg.Driver, tt.wantDriver)
			}
		})
	}
}

// TestProvider_GetAllServiceDatabaseConfigs tests the GetAllServiceDatabaseConfigs method
func TestProvider_GetAllServiceDatabaseConfigs(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add meta
	metaJSON := `{"id":1,"code":"test_tenant","name":"Test Tenant","status":"active"}`
	metaKey := "tenants/1/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Add multiple database configs
	dbJSON1 := `{"tenantId":1,"serviceCode":"evidence-command","driver":"mysql","database":"cmd_db","host":"localhost","port":3306}`
	dbKey1 := "tenants/1/database/evidence-command"
	if err := p.parseServiceDatabaseConfig(dbKey1, dbJSON1, data); err != nil {
		t.Fatalf("parseServiceDatabaseConfig failed: %v", err)
	}

	dbJSON2 := `{"tenantId":1,"serviceCode":"evidence-query","driver":"postgres","database":"query_db","host":"localhost","port":5432}`
	dbKey2 := "tenants/1/database/evidence-query"
	if err := p.parseServiceDatabaseConfig(dbKey2, dbJSON2, data); err != nil {
		t.Fatalf("parseServiceDatabaseConfig failed: %v", err)
	}

	p.data.Store(data)

	tests := []struct {
		name         string
		tenantID     int
		wantFound    bool
		wantCount    int
		wantServices []string
	}{
		{
			name:         "existing tenant with multiple services",
			tenantID:     1,
			wantFound:    true,
			wantCount:    2,
			wantServices: []string{"evidence-command", "evidence-query"},
		},
		{
			name:         "non-existent tenant",
			tenantID:     999,
			wantFound:    false,
			wantCount:    0,
			wantServices: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, ok := p.GetAllServiceDatabaseConfigs(tt.tenantID)
			if ok != tt.wantFound {
				t.Errorf("GetAllServiceDatabaseConfigs() ok = %v, want %v", ok, tt.wantFound)
			}
			if tt.wantFound && len(configs) != tt.wantCount {
				t.Errorf("GetAllServiceDatabaseConfigs() count = %v, want %v", len(configs), tt.wantCount)
			}
			if tt.wantFound {
				for _, service := range tt.wantServices {
					if _, exists := configs[service]; !exists {
						t.Errorf("GetAllServiceDatabaseConfigs() missing service %s", service)
					}
				}
			}
		})
	}
}

// TestProvider_GetFtpConfigs tests the GetFtpConfigs method
func TestProvider_GetFtpConfigs(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add meta
	metaJSON := `{"id":1,"code":"test_tenant","name":"Test Tenant","status":"active"}`
	metaKey := "tenants/1/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Add FTP configs
	ftpJSON1 := `{"tenantId":1,"username":"ftp_user1","passwordHash":"hash1","description":"FTP 1","status":"active"}`
	ftpKey1 := "tenants/1/ftp/ftp_user1"
	if err := p.parseFtpConfig(ftpKey1, ftpJSON1, data); err != nil {
		t.Fatalf("parseFtpConfig failed: %v", err)
	}

	ftpJSON2 := `{"tenantId":1,"username":"ftp_user2","passwordHash":"hash2","description":"FTP 2","status":"active"}`
	ftpKey2 := "tenants/1/ftp/ftp_user2"
	if err := p.parseFtpConfig(ftpKey2, ftpJSON2, data); err != nil {
		t.Fatalf("parseFtpConfig failed: %v", err)
	}

	p.data.Store(data)

	tests := []struct {
		name      string
		tenantID  int
		wantFound bool
		wantCount int
	}{
		{
			name:      "existing tenant with FTP configs",
			tenantID:  1,
			wantFound: true,
			wantCount: 2,
		},
		{
			name:      "non-existent tenant",
			tenantID:  999,
			wantFound: false,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, ok := p.GetFtpConfigs(tt.tenantID)
			if ok != tt.wantFound {
				t.Errorf("GetFtpConfigs() ok = %v, want %v", ok, tt.wantFound)
			}
			if tt.wantFound && len(configs) != tt.wantCount {
				t.Errorf("GetFtpConfigs() count = %v, want %v", len(configs), tt.wantCount)
			}
		})
	}
}

// TestProvider_GetFtpConfigByUsername tests the GetFtpConfigByUsername method
func TestProvider_GetFtpConfigByUsername(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add meta for tenant 1
	metaJSON1 := `{"id":1,"code":"test_tenant","name":"Test Tenant","status":"active"}`
	metaKey1 := "tenants/1/meta"
	if err := p.parseTenantMeta(metaKey1, metaJSON1, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Add meta for tenant 2
	metaJSON2 := `{"id":2,"code":"test_tenant2","name":"Test Tenant 2","status":"active"}`
	metaKey2 := "tenants/2/meta"
	if err := p.parseTenantMeta(metaKey2, metaJSON2, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Add FTP configs for different tenants
	ftpJSON1 := `{"tenantId":1,"username":"ftp_user1","passwordHash":"hash1","description":"FTP 1","status":"active"}`
	ftpKey1 := "tenants/1/ftp/ftp_user1"
	if err := p.parseFtpConfig(ftpKey1, ftpJSON1, data); err != nil {
		t.Fatalf("parseFtpConfig failed: %v", err)
	}

	ftpJSON2 := `{"tenantId":2,"username":"ftp_user2","passwordHash":"hash2","description":"FTP 2","status":"active"}`
	ftpKey2 := "tenants/2/ftp/ftp_user2"
	if err := p.parseFtpConfig(ftpKey2, ftpJSON2, data); err != nil {
		t.Fatalf("parseFtpConfig failed: %v", err)
	}

	p.data.Store(data)

	tests := []struct {
		name        string
		username    string
		wantFound   bool
		wantTenantID int
	}{
		{
			name:        "existing username in tenant 1",
			username:    "ftp_user1",
			wantFound:   true,
			wantTenantID: 1,
		},
		{
			name:        "existing username in tenant 2",
			username:    "ftp_user2",
			wantFound:   true,
			wantTenantID: 2,
		},
		{
			name:        "non-existent username",
			username:    "non_existent",
			wantFound:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := p.GetFtpConfigByUsername(tt.username)
			if ok != tt.wantFound {
				t.Errorf("GetFtpConfigByUsername() ok = %v, want %v", ok, tt.wantFound)
			}
			if tt.wantFound && cfg.TenantID != tt.wantTenantID {
				t.Errorf("GetFtpConfigByUsername() tenantID = %v, want %v", cfg.TenantID, tt.wantTenantID)
			}
		})
	}
}

// TestProvider_GetActiveFtpConfigs tests the GetActiveFtpConfigs method
func TestProvider_GetActiveFtpConfigs(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add meta
	metaJSON := `{"id":1,"code":"test_tenant","name":"Test Tenant","status":"active"}`
	metaKey := "tenants/1/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Add FTP configs with different statuses
	ftpJSON1 := `{"tenantId":1,"username":"ftp_user1","passwordHash":"hash1","description":"FTP 1","status":"active"}`
	ftpKey1 := "tenants/1/ftp/ftp_user1"
	if err := p.parseFtpConfig(ftpKey1, ftpJSON1, data); err != nil {
		t.Fatalf("parseFtpConfig failed: %v", err)
	}

	ftpJSON2 := `{"tenantId":1,"username":"ftp_user2","passwordHash":"hash2","description":"FTP 2","status":"inactive"}`
	ftpKey2 := "tenants/1/ftp/ftp_user2"
	if err := p.parseFtpConfig(ftpKey2, ftpJSON2, data); err != nil {
		t.Fatalf("parseFtpConfig failed: %v", err)
	}

	ftpJSON3 := `{"tenantId":1,"username":"ftp_user3","passwordHash":"hash3","description":"FTP 3","status":""}`
	ftpKey3 := "tenants/1/ftp/ftp_user3"
	if err := p.parseFtpConfig(ftpKey3, ftpJSON3, data); err != nil {
		t.Fatalf("parseFtpConfig failed: %v", err)
	}

	p.data.Store(data)

	tests := []struct {
		name      string
		tenantID  int
		wantFound bool
		wantCount int
	}{
		{
			name:      "existing tenant with active FTP configs",
			tenantID:  1,
			wantFound: true,
			wantCount: 2, // active and empty status
		},
		{
			name:      "non-existent tenant",
			tenantID:  999,
			wantFound: false,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configs, ok := p.GetActiveFtpConfigs(tt.tenantID)
			if ok != tt.wantFound {
				t.Errorf("GetActiveFtpConfigs() ok = %v, want %v", ok, tt.wantFound)
			}
			if tt.wantFound && len(configs) != tt.wantCount {
				t.Errorf("GetActiveFtpConfigs() count = %v, want %v", len(configs), tt.wantCount)
			}
			// Verify all returned configs are active
			if tt.wantFound {
				for _, cfg := range configs {
					if cfg.Status != "" && cfg.Status != "active" {
						t.Errorf("GetActiveFtpConfigs() returned inactive config: %s", cfg.Status)
					}
				}
			}
		})
	}
}

// TestProvider_GetDomainConfig tests the GetDomainConfig method
func TestProvider_GetDomainConfig(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add meta
	metaJSON := `{"id":1,"code":"test_tenant","name":"Test Tenant","status":"active"}`
	metaKey := "tenants/1/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Add domain config - primary
	primaryJSON := `"tenant1.example.com"`
	primaryKey := "tenants/1/domain/primary"
	p.parseDomainConfig(primaryKey, primaryJSON, data)

	// Add domain config - aliases
	aliasesJSON := `["www.tenant1.example.com", "tenant1.com"]`
	aliasesKey := "tenants/1/domain/aliases"
	p.parseDomainConfig(aliasesKey, aliasesJSON, data)

	// Add domain config - internal
	internalJSON := `"tenant1.internal"`
	internalKey := "tenants/1/domain/internal"
	p.parseDomainConfig(internalKey, internalJSON, data)

	p.data.Store(data)

	tests := []struct {
		name         string
		tenantID     int
		wantFound    bool
		wantPrimary  string
		wantAliasCount int
	}{
		{
			name:         "existing tenant with domain config",
			tenantID:     1,
			wantFound:    true,
			wantPrimary:  "tenant1.example.com",
			wantAliasCount: 2,
		},
		{
			name:         "non-existent tenant",
			tenantID:     999,
			wantFound:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := p.GetDomainConfig(tt.tenantID)
			if ok != tt.wantFound {
				t.Errorf("GetDomainConfig() ok = %v, want %v", ok, tt.wantFound)
			}
			if tt.wantFound {
				if cfg.Primary != tt.wantPrimary {
					t.Errorf("GetDomainConfig() primary = %v, want %v", cfg.Primary, tt.wantPrimary)
				}
				if len(cfg.Aliases) != tt.wantAliasCount {
					t.Errorf("GetDomainConfig() alias count = %v, want %v", len(cfg.Aliases), tt.wantAliasCount)
				}
				if cfg.Code != "test_tenant" {
					t.Errorf("GetDomainConfig() code = %v, want test_tenant", cfg.Code)
				}
			}
		})
	}
}

// TestProvider_GetStorageConfig tests the GetStorageConfig method
func TestProvider_GetStorageConfig(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add meta
	metaJSON := `{"id":1,"code":"test_tenant","name":"Test Tenant","status":"active"}`
	metaKey := "tenants/1/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Add storage config
	storageJSON := `{"tenantId":1,"quotaBytes":107374182400,"maxFileSizeBytes":524288000,"maxConcurrentUploads":20}`
	storageKey := "tenants/1/storage"
	if err := p.parseStorageConfig(storageKey, storageJSON, data); err != nil {
		t.Fatalf("parseStorageConfig failed: %v", err)
	}

	p.data.Store(data)

	tests := []struct {
		name           string
		tenantID       int
		wantFound      bool
		wantQuotaBytes int64
	}{
		{
			name:           "existing tenant with storage config",
			tenantID:       1,
			wantFound:      true,
			wantQuotaBytes: 107374182400,
		},
		{
			name:           "non-existent tenant",
			tenantID:       999,
			wantFound:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := p.GetStorageConfig(tt.tenantID)
			if ok != tt.wantFound {
				t.Errorf("GetStorageConfig() ok = %v, want %v", ok, tt.wantFound)
			}
			if tt.wantFound && cfg.QuotaBytes != tt.wantQuotaBytes {
				t.Errorf("GetStorageConfig() quotaBytes = %v, want %v", cfg.QuotaBytes, tt.wantQuotaBytes)
			}
		})
	}
}

// TestProvider_GetTenantMeta tests the GetTenantMeta method
func TestProvider_GetTenantMeta(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add meta
	metaJSON := `{"id":1,"code":"test_tenant","name":"Test Tenant","status":"active"}`
	metaKey := "tenants/1/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	p.data.Store(data)

	tests := []struct {
		name      string
		tenantID  int
		wantFound bool
		wantCode  string
	}{
		{
			name:      "existing tenant",
			tenantID:  1,
			wantFound: true,
			wantCode:  "test_tenant",
		},
		{
			name:      "non-existent tenant",
			tenantID:  999,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, ok := p.GetTenantMeta(tt.tenantID)
			if ok != tt.wantFound {
				t.Errorf("GetTenantMeta() ok = %v, want %v", ok, tt.wantFound)
			}
			if tt.wantFound && meta.Code != tt.wantCode {
				t.Errorf("GetTenantMeta() code = %v, want %v", meta.Code, tt.wantCode)
			}
		})
	}
}

// TestProvider_IsTenantEnabled tests the IsTenantEnabled method
func TestProvider_IsTenantEnabled(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add active tenant
	activeJSON := `{"id":1,"code":"active_tenant","name":"Active Tenant","status":"active"}`
	activeKey := "tenants/1/meta"
	if err := p.parseTenantMeta(activeKey, activeJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Add inactive tenant
	inactiveJSON := `{"id":2,"code":"inactive_tenant","name":"Inactive Tenant","status":"inactive"}`
	inactiveKey := "tenants/2/meta"
	if err := p.parseTenantMeta(inactiveKey, inactiveJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	p.data.Store(data)

	tests := []struct {
		name     string
		tenantID int
		want     bool
	}{
		{
			name:     "active tenant",
			tenantID: 1,
			want:     true,
		},
		{
			name:     "inactive tenant",
			tenantID: 2,
			want:     false,
		},
		{
			name:     "non-existent tenant",
			tenantID: 999,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.IsTenantEnabled(tt.tenantID)
			if got != tt.want {
				t.Errorf("IsTenantEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetResolverConfig_Nil(t *testing.T) {
	p := &Provider{}
	p.data.Store(&tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  nil,
	})

	cfg := p.GetResolverConfig()
	assert.Nil(t, cfg)
}

func TestGetResolverConfig_WithConfig(t *testing.T) {
	expected := &ResolverConfig{
		ID:             1,
		HTTPType:       "host",
		HTTPHeaderName: "X-Tenant-ID",
		FTPType:        "username",
	}
	p := &Provider{}
	p.data.Store(&tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  expected,
	})

	cfg := p.GetResolverConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, expected, cfg)
}

func TestGetResolverConfigOrDefault_WithConfig(t *testing.T) {
	expected := &ResolverConfig{
		ID:             1,
		HTTPType:       "query",
		HTTPHeaderName: "X-Custom-Tenant",
		FTPType:        "username",
	}
	p := &Provider{}
	p.data.Store(&tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  expected,
	})

	cfg := p.GetResolverConfigOrDefault()
	assert.Equal(t, "query", cfg.HTTPType)
	assert.Equal(t, "X-Custom-Tenant", cfg.HTTPHeaderName)
}

func TestGetResolverConfigOrDefault_DefaultValues(t *testing.T) {
	p := &Provider{}
	p.data.Store(&tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  nil,
	})

	cfg := p.GetResolverConfigOrDefault()
	assert.Equal(t, "header", cfg.HTTPType)
	assert.Equal(t, "X-Tenant-ID", cfg.HTTPHeaderName)
	assert.Equal(t, "username", cfg.FTPType)
}

func TestGetResolverConfigOrDefault_ReturnsValueNotPointer(t *testing.T) {
	p := &Provider{}
	p.data.Store(&tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
		Resolver:  nil,
	})

	cfg1 := p.GetResolverConfigOrDefault()
	cfg2 := p.GetResolverConfigOrDefault()

	// Modify cfg1 should not affect cfg2
	cfg1.HTTPType = "modified"

	assert.Equal(t, "header", cfg2.HTTPType)
}

// TestProvider_GetTenantIDByCode tests the GetTenantIDByCode method
func TestProvider_GetTenantIDByCode(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}
	data.Metas[1] = &TenantMeta{TenantID: 1, Code: "acmeCorp", Name: "ACME Corp"}
	data.Metas[2] = &TenantMeta{TenantID: 2, Code: "techInc", Name: "Tech Inc"}

	// Build codeIndex manually for test
	data.codeIndex = map[string]int{
		"acmecorp": 1,
		"techinc":  2,
	}
	p.data.Store(data)

	tests := []struct {
		name      string
		code      string
		wantID    int
		wantFound bool
	}{
		{
			name:      "existing code lowercase",
			code:      "acmecorp",
			wantID:    1,
			wantFound: true,
		},
		{
			name:      "existing code uppercase (case insensitive)",
			code:      "ACMECORP",
			wantID:    1,
			wantFound: true,
		},
		{
			name:      "existing code mixed case",
			code:      "TechInc",
			wantID:    2,
			wantFound: true,
		},
		{
			name:      "non-existing code",
			code:      "unknown",
			wantID:    0,
			wantFound: false,
		},
		{
			name:      "empty code",
			code:      "",
			wantID:    0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, found := p.GetTenantIDByCode(tt.code)
			if found != tt.wantFound {
				t.Errorf("GetTenantIDByCode(%q) found = %v, want %v", tt.code, found, tt.wantFound)
			}
			if id != tt.wantID {
				t.Errorf("GetTenantIDByCode(%q) id = %v, want %v", tt.code, id, tt.wantID)
			}
		})
	}
}

// TestBuildIndexes tests that both domainIndex and codeIndex are built correctly
func TestBuildIndexes(t *testing.T) {
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// Add tenant metas
	data.Metas[1] = &TenantMeta{TenantID: 1, Code: "acmeCorp", Name: "ACME"}
	data.Metas[2] = &TenantMeta{TenantID: 2, Code: "techInc", Name: "Tech"}

	// Add domain configs
	data.Domains[1] = &DomainConfig{
		TenantID: 1,
		Primary:  "acme.example.com",
		Aliases:  []string{"www.acme.example.com"},
	}
	data.Domains[2] = &DomainConfig{
		TenantID: 2,
		Primary:  "tech.example.com",
	}

	// Build indexes
	buildIndexes(data)

	// Verify domainIndex
	assert.Equal(t, 1, data.domainIndex["acme.example.com"])
	assert.Equal(t, 1, data.domainIndex["www.acme.example.com"])
	assert.Equal(t, 2, data.domainIndex["tech.example.com"])

	// Verify codeIndex (lowercase)
	assert.Equal(t, 1, data.codeIndex["acmecorp"])
	assert.Equal(t, 2, data.codeIndex["techinc"])

	// Verify case insensitivity in index keys
	assert.NotContains(t, data.codeIndex, "ACMECORP") // Should be lowercase
}
