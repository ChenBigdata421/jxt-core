package provider

import (
	"testing"
)

func TestProvider_processMetaKey(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		metas:    make(map[int]*TenantMeta),
		databases: make(map[int]*DatabaseConfig),
		ftps:      make(map[int]*FtpConfig),
		storages:  make(map[int]*StorageConfig),
	}

	metaJSON := `{"id":123,"code":"test_tenant","name":"Test Tenant","status":"active","billingPlan":"premium"}`
	p.processMetaKey(123, metaJSON, data)

	if meta, ok := data.metas[123]; !ok {
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

func TestProvider_processDatabaseKey(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		metas:    make(map[int]*TenantMeta),
		databases: make(map[int]*DatabaseConfig),
	}

	// First, add meta
	metaJSON := `{"id":456,"code":"db_tenant","name":"DB Tenant","status":"active"}`
	p.processMetaKey(456, metaJSON, data)

	// Then add database config
	dbJSON := `{"tenantId":456,"driver":"postgres","databaseName":"testdb","host":"localhost","port":5432,"maxOpenConns":50,"maxIdleConns":10}`
	p.processDatabaseKey(456, dbJSON, data)

	if db, ok := data.databases[456]; !ok {
		t.Fatal("database config not stored")
	} else {
		if db.TenantID != 456 {
			t.Errorf("TenantID = %v, want 456", db.TenantID)
		}
		if db.Code != "db_tenant" {
			t.Errorf("Code = %v, want db_tenant (from meta)", db.Code)
		}
		if db.Name != "DB Tenant" {
			t.Errorf("Name = %v, want DB Tenant (from meta)", db.Name)
		}
		if db.Driver != "postgres" {
			t.Errorf("Driver = %v, want postgres", db.Driver)
		}
		if db.DbName != "testdb" {
			t.Errorf("DbName = %v, want testdb", db.DbName)
		}
	}
}

func TestProvider_processFtpKey(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		metas: make(map[int]*TenantMeta),
		ftps:   make(map[int]*FtpConfig),
	}

	// First, add meta
	metaJSON := `{"id":789,"code":"ftp_tenant","name":"FTP Tenant","status":"active"}`
	p.processMetaKey(789, metaJSON, data)

	// Then add FTP config
	ftpJSON := `{"tenantId":789,"username":"ftp_user","passwordHash":"$2a$10$..."}`
	p.processFtpKey(789, ftpJSON, data)

	if ftp, ok := data.ftps[789]; !ok {
		t.Fatal("FTP config not stored")
	} else {
		if ftp.TenantID != 789 {
			t.Errorf("TenantID = %v, want 789", ftp.TenantID)
		}
		if ftp.Code != "ftp_tenant" {
			t.Errorf("Code = %v, want ftp_tenant (from meta)", ftp.Code)
		}
		if ftp.Username != "ftp_user" {
			t.Errorf("Username = %v, want ftp_user", ftp.Username)
		}
	}
}

func TestProvider_processStorageKey(t *testing.T) {
	p := &Provider{namespace: "jxt/"}
	data := &tenantData{
		metas:    make(map[int]*TenantMeta),
		storages: make(map[int]*StorageConfig),
	}

	// First, add meta
	metaJSON := `{"id":999,"code":"storage_tenant","name":"Storage Tenant","status":"active"}`
	p.processMetaKey(999, metaJSON, data)

	// Then add storage config
	storageJSON := `{"tenantId":999,"uploadQuotaGb":100,"maxFileSizeMb":500,"maxConcurrentUploads":20}`
	p.processStorageKey(999, storageJSON, data)

	if storage, ok := data.storages[999]; !ok {
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
