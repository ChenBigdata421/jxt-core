package provider

import (
	"testing"
)

func TestProvider_parseTenantMeta(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	metaJSON := `{"id":123,"code":"test_tenant","name":"Test Tenant","status":"active","billingPlan":"premium"}`
	// Pass stripped key (without "jxt/" prefix) as processKey does
	key := "tenants/123/meta"
	if err := p.parseTenantMeta(key, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	if meta, ok := data.Metas[123]; !ok {
		t.Fatal("meta not stored")
	} else {
		if meta.TenantID != 123 {
			t.Errorf("TenantID = %v, want 123", meta.TenantID)
		}
		if meta.Code != "test_tenant" {
			t.Errorf("Code = %v, want test_tenant", meta.Code)
		}
		if meta.Name != "Test Tenant" {
			t.Errorf("Name = %v, want Test Tenant", meta.Name)
		}
		if meta.Status != "active" {
			t.Errorf("Status = %v, want active", meta.Status)
		}
		if !meta.IsEnabled() {
			t.Error("IsEnabled() = false, want true for active status")
		}
	}
}

func TestProvider_parseServiceDatabaseConfig(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
	}

	// First, add meta
	metaJSON := `{"id":456,"code":"db_tenant","name":"DB Tenant","status":"active"}`
	metaKey := "tenants/456/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Then add database config with service code
	dbJSON := `{"tenantId":456,"serviceCode":"evidence-command","driver":"postgres","database":"testdb","host":"localhost","port":5432,"maxOpenConns":50,"maxIdleConns":10}`
	dbKey := "tenants/456/database/evidence-command"
	if err := p.parseServiceDatabaseConfig(dbKey, dbJSON, data); err != nil {
		t.Fatalf("parseServiceDatabaseConfig failed: %v", err)
	}

	if dbMap, ok := data.Databases[456]; !ok {
		t.Fatal("database config not stored")
	} else {
		db, ok := dbMap["evidence-command"]
		if !ok {
			t.Fatal("evidence-command database config not stored")
		}
		if db.TenantID != 456 {
			t.Errorf("TenantID = %v, want 456", db.TenantID)
		}
		if db.ServiceCode != "evidence-command" {
			t.Errorf("ServiceCode = %v, want evidence-command", db.ServiceCode)
		}
		if db.Driver != "postgres" {
			t.Errorf("Driver = %v, want postgres", db.Driver)
		}
		if db.Database != "testdb" {
			t.Errorf("Database = %v, want testdb", db.Database)
		}
	}
}

func TestProvider_parseFtpConfig(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas: make(map[int]*TenantMeta),
		Ftps:  make(map[int][]*FtpConfigDetail),
	}

	// First, add meta
	metaJSON := `{"id":789,"code":"ftp_tenant","name":"FTP Tenant","status":"active"}`
	metaKey := "tenants/789/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Then add FTP config
	ftpJSON := `{"tenantId":789,"username":"ftp_user","passwordHash":"$2a$10$...","description":"Main FTP","status":"active"}`
	ftpKey := "tenants/789/ftp/ftp_user"
	if err := p.parseFtpConfig(ftpKey, ftpJSON, data); err != nil {
		t.Fatalf("parseFtpConfig failed: %v", err)
	}

	if ftpList, ok := data.Ftps[789]; !ok {
		t.Fatal("FTP config not stored")
	} else {
		if len(ftpList) != 1 {
			t.Fatalf("Expected 1 FTP config, got %d", len(ftpList))
		}
		ftp := ftpList[0]
		if ftp.TenantID != 789 {
			t.Errorf("TenantID = %v, want 789", ftp.TenantID)
		}
		if ftp.Username != "ftp_user" {
			t.Errorf("Username = %v, want ftp_user", ftp.Username)
		}
		if ftp.Description != "Main FTP" {
			t.Errorf("Description = %v, want Main FTP", ftp.Description)
		}
	}
}

func TestProvider_parseStorageConfig(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:    make(map[int]*TenantMeta),
		Storages: make(map[int]*StorageConfig),
	}

	// First, add meta
	metaJSON := `{"id":999,"code":"storage_tenant","name":"Storage Tenant","status":"active"}`
	metaKey := "tenants/999/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Then add storage config
	storageJSON := `{"tenantId":999,"quotaBytes":107374182400,"maxFileSizeBytes":524288000,"maxConcurrentUploads":20}`
	storageKey := "tenants/999/storage"
	if err := p.parseStorageConfig(storageKey, storageJSON, data); err != nil {
		t.Fatalf("parseStorageConfig failed: %v", err)
	}

	if storage, ok := data.Storages[999]; !ok {
		t.Fatal("Storage config not stored")
	} else {
		if storage.TenantID != 999 {
			t.Errorf("TenantID = %v, want 999", storage.TenantID)
		}
		if storage.Code != "storage_tenant" {
			t.Errorf("Code = %v, want storage_tenant (from meta)", storage.Code)
		}
		if storage.QuotaBytes != 100*1024*1024*1024 {
			t.Errorf("QuotaBytes = %v, want %v (100GB in bytes)", storage.QuotaBytes, 100*1024*1024*1024)
		}
		if storage.MaxFileSizeBytes != 500*1024*1024 {
			t.Errorf("MaxFileSizeBytes = %v, want %v (500MB in bytes)", storage.MaxFileSizeBytes, 500*1024*1024)
		}
		if storage.MaxConcurrentUploads != 20 {
			t.Errorf("MaxConcurrentUploads = %v, want 20", storage.MaxConcurrentUploads)
		}
	}
}

func TestTenantMeta_IsEnabled(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"active status", "active", true},
		{"inactive status", "inactive", false},
		{"suspended status", "suspended", false},
		{"unknown status", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := &TenantMeta{Status: tt.status}
			if got := meta.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_parseDomainConfig(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	// First, add meta
	metaJSON := `{"id":100,"code":"domain_tenant","name":"Domain Tenant","status":"active"}`
	metaKey := "tenants/100/meta"
	if err := p.parseTenantMeta(metaKey, metaJSON, data); err != nil {
		t.Fatalf("parseTenantMeta failed: %v", err)
	}

	// Verify meta was added
	if _, ok := data.Metas[100]; !ok {
		t.Fatal("Meta not added for tenant 100")
	}

	// Then add domain configs
	// The values should be proper JSON that can be unmarshaled
	// For string values, we need the quoted JSON string
	primaryJSON := `"tenant100.example.com"`
	primaryKey := "tenants/100/domain/primary"
	p.parseDomainConfig(primaryKey, primaryJSON, data)

	aliasesJSON := `["www.tenant100.example.com","tenant100.com"]`
	aliasesKey := "tenants/100/domain/aliases"
	p.parseDomainConfig(aliasesKey, aliasesJSON, data)

	internalJSON := `"tenant100.internal"`
	internalKey := "tenants/100/domain/internal"
	p.parseDomainConfig(internalKey, internalJSON, data)

	if domain, ok := data.Domains[100]; !ok {
		t.Fatal("Domain config not stored")
	} else {

		if domain.TenantID != 100 {
			t.Errorf("TenantID = %v, want 100", domain.TenantID)
		}
		if domain.Code != "domain_tenant" {
			t.Errorf("Code = %v, want domain_tenant (from meta)", domain.Code)
		}
		if domain.Name != "Domain Tenant" {
			t.Errorf("Name = %v, want Domain Tenant (from meta)", domain.Name)
		}
		if domain.Primary != "tenant100.example.com" {
			t.Errorf("Primary = %q, want %q", domain.Primary, "tenant100.example.com")
		}
		if len(domain.Aliases) != 2 {
			t.Errorf("Aliases count = %v, want 2", len(domain.Aliases))
		} else {
			if domain.Aliases[0] != "www.tenant100.example.com" {
				t.Errorf("Aliases[0] = %v, want www.tenant100.example.com", domain.Aliases[0])
			}
			if domain.Aliases[1] != "tenant100.com" {
				t.Errorf("Aliases[1] = %v, want tenant100.com", domain.Aliases[1])
			}
		}
		if domain.Internal != "tenant100.internal" {
			t.Errorf("Internal = %v, want tenant100.internal", domain.Internal)
		}
	}
}
