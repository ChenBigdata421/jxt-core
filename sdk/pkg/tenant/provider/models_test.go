package provider

import (
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestServiceDatabaseConfig(t *testing.T) {
	cfg := ServiceDatabaseConfig{
		TenantID:    1,
		ServiceCode: "evidence-command",
		Driver:      "mysql",
		Host:        "localhost",
		Port:        3306,
		Database:    "evidence_cmd",
	}

	if cfg.ServiceCode != "evidence-command" {
		t.Errorf("expected ServiceCode to be evidence-command, got %s", cfg.ServiceCode)
	}
}

func TestFtpConfigDetail(t *testing.T) {
	cfg := FtpConfigDetail{
		TenantID:      1,
		Username:      "tenant1_ftp",
		PasswordHash:  "hash",
		Description:   "Main FTP",
		Status:        "active",
	}

	if cfg.Description != "Main FTP" {
		t.Errorf("expected Description to be 'Main FTP', got %s", cfg.Description)
	}
	if cfg.Status != "active" {
		t.Errorf("expected Status to be 'active', got %s", cfg.Status)
	}
}

func TestDomainConfig(t *testing.T) {
	cfg := DomainConfig{
		TenantID: 1,
		Code:     "tenant1",
		Name:     "Tenant 1",
		Primary:  "tenant1.example.com",
		Aliases:  []string{"www.tenant1.example.com"},
		Internal: "tenant1.internal",
	}

	if cfg.Primary != "tenant1.example.com" {
		t.Errorf("expected Primary to be 'tenant1.example.com', got %s", cfg.Primary)
	}
	if len(cfg.Aliases) != 1 {
		t.Errorf("expected 1 alias, got %d", len(cfg.Aliases))
	}
}

func TestServiceDatabaseConfig_JSONRoundTrip(t *testing.T) {
	original := ServiceDatabaseConfig{
		TenantID:     1,
		ServiceCode:  "evidence-command",
		Driver:       "mysql",
		Host:         "localhost",
		Port:         3306,
		Database:     "evidence_cmd",
		Username:     "tenant1_user",
		SSLMode:      "require",
		MaxOpenConns: 100,
		MaxIdleConns: 10,
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal back to struct
	var decoded ServiceDatabaseConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify all fields match
	if decoded.TenantID != original.TenantID {
		t.Errorf("TenantID: expected %d, got %d", original.TenantID, decoded.TenantID)
	}
	if decoded.ServiceCode != original.ServiceCode {
		t.Errorf("ServiceCode: expected %s, got %s", original.ServiceCode, decoded.ServiceCode)
	}
	if decoded.Driver != original.Driver {
		t.Errorf("Driver: expected %s, got %s", original.Driver, decoded.Driver)
	}
	if decoded.Host != original.Host {
		t.Errorf("Host: expected %s, got %s", original.Host, decoded.Host)
	}
	if decoded.Port != original.Port {
		t.Errorf("Port: expected %d, got %d", original.Port, decoded.Port)
	}
	if decoded.Database != original.Database {
		t.Errorf("Database: expected %s, got %s", original.Database, decoded.Database)
	}
	if decoded.Username != original.Username {
		t.Errorf("Username: expected %s, got %s", original.Username, decoded.Username)
	}
	if decoded.SSLMode != original.SSLMode {
		t.Errorf("SSLMode: expected %s, got %s", original.SSLMode, decoded.SSLMode)
	}
	if decoded.MaxOpenConns != original.MaxOpenConns {
		t.Errorf("MaxOpenConns: expected %d, got %d", original.MaxOpenConns, decoded.MaxOpenConns)
	}
	if decoded.MaxIdleConns != original.MaxIdleConns {
		t.Errorf("MaxIdleConns: expected %d, got %d", original.MaxIdleConns, decoded.MaxIdleConns)
	}
}

func TestFtpConfigDetail_JSONRoundTrip(t *testing.T) {
	original := FtpConfigDetail{
		TenantID:        1,
		Username:        "tenant1_ftp",
		PasswordHash:    "hashed_password_123",
		Description:     "Main FTP configuration",
		Status:          "active",
		HomeDirectory:   "/home/tenant1/ftp",
		WritePermission: true,
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal back to struct
	var decoded FtpConfigDetail
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify all fields match
	if decoded.TenantID != original.TenantID {
		t.Errorf("TenantID: expected %d, got %d", original.TenantID, decoded.TenantID)
	}
	if decoded.Username != original.Username {
		t.Errorf("Username: expected %s, got %s", original.Username, decoded.Username)
	}
	if decoded.PasswordHash != original.PasswordHash {
		t.Errorf("PasswordHash: expected %s, got %s", original.PasswordHash, decoded.PasswordHash)
	}
	if decoded.Description != original.Description {
		t.Errorf("Description: expected %s, got %s", original.Description, decoded.Description)
	}
	if decoded.Status != original.Status {
		t.Errorf("Status: expected %s, got %s", original.Status, decoded.Status)
	}
	if decoded.HomeDirectory != original.HomeDirectory {
		t.Errorf("HomeDirectory: expected %s, got %s", original.HomeDirectory, decoded.HomeDirectory)
	}
	if decoded.WritePermission != original.WritePermission {
		t.Errorf("WritePermission: expected %v, got %v", original.WritePermission, decoded.WritePermission)
	}
}

func TestDomainConfig_JSONRoundTrip(t *testing.T) {
	original := DomainConfig{
		TenantID: 1,
		Code:     "tenant1",
		Name:     "Tenant 1 Corporation",
		Primary:  "tenant1.example.com",
		Aliases:  []string{"www.tenant1.example.com", "tenant1.com"},
		Internal: "tenant1.internal.local",
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal back to struct
	var decoded DomainConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify all fields match
	if decoded.TenantID != original.TenantID {
		t.Errorf("TenantID: expected %d, got %d", original.TenantID, decoded.TenantID)
	}
	if decoded.Code != original.Code {
		t.Errorf("Code: expected %s, got %s", original.Code, decoded.Code)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name: expected %s, got %s", original.Name, decoded.Name)
	}
	if decoded.Primary != original.Primary {
		t.Errorf("Primary: expected %s, got %s", original.Primary, decoded.Primary)
	}
	if decoded.Internal != original.Internal {
		t.Errorf("Internal: expected %s, got %s", original.Internal, decoded.Internal)
	}
	if len(decoded.Aliases) != len(original.Aliases) {
		t.Errorf("Aliases length: expected %d, got %d", len(original.Aliases), len(decoded.Aliases))
	}
	for i, alias := range original.Aliases {
		if decoded.Aliases[i] != alias {
			t.Errorf("Aliases[%d]: expected %s, got %s", i, alias, decoded.Aliases[i])
		}
	}
}

func TestAppendOrUpdateFtpConfig(t *testing.T) {
	configs := []*FtpConfigDetail{
		{Username: "user1", Description: "First"},
		{Username: "user2", Description: "Second"},
	}

	// Test update
	newConfig := &FtpConfigDetail{Username: "user1", Description: "Updated"}
	result := appendOrUpdateFtpConfig(configs, newConfig)

	if len(result) != 2 {
		t.Errorf("expected 2 configs, got %d", len(result))
	}
	if result[0].Description != "Updated" {
		t.Errorf("expected first config to be updated")
	}

	// Test append
	newConfig2 := &FtpConfigDetail{Username: "user3", Description: "Third"}
	result = appendOrUpdateFtpConfig(result, newConfig2)

	if len(result) != 3 {
		t.Errorf("expected 3 configs, got %d", len(result))
	}
}

func TestRemoveFtpConfigByUsername(t *testing.T) {
	configs := []*FtpConfigDetail{
		{Username: "user1"},
		{Username: "user2"},
		{Username: "user3"},
	}

	result := removeFtpConfigByUsername(configs, "user2")

	if len(result) != 2 {
		t.Errorf("expected 2 configs, got %d", len(result))
	}
	if result[1].Username != "user3" {
		t.Errorf("expected second config to be user3")
	}
}

func TestTenantDataCopyData(t *testing.T) {
	original := &tenantData{
		Metas: map[int]*TenantMeta{
			1: {TenantID: 1, Code: "t1"},
		},
		Databases: map[int]map[string]*ServiceDatabaseConfig{
			1: {
				"evidence-command": {ServiceCode: "evidence-command"},
			},
		},
		Ftps: map[int][]*FtpConfigDetail{
			1: {{Username: "user1"}},
		},
		Domains: map[int]*DomainConfig{
			1: {Primary: "example.com"},
		},
	}

	copied := original.copyData()

	// Verify copy is independent
	copied.Metas[2] = &TenantMeta{TenantID: 2}
	if _, ok := original.Metas[2]; ok {
		t.Error("copy should be independent")
	}

	// Verify nested maps are independent
	if copied.Databases[1] == nil {
		t.Error("copied databases should not be nil")
	}
	if copied.Ftps[1] == nil {
		t.Error("copied ftps should not be nil")
	}
}

func TestServiceDatabaseConfig_PasswordFields(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ServiceDatabaseConfig Suite")
}

var _ = Describe("ServiceDatabaseConfig", func() {
	Describe("Password fields", func() {
		It("should have Password and PasswordEncrypted fields", func() {
			config := &ServiceDatabaseConfig{
				TenantID:          1,
				ServiceCode:       "evidence-command",
				Password:          "encrypted-password-here",
				PasswordEncrypted: true,
			}

			Expect(config.TenantID).To(Equal(1))
			Expect(config.Password).To(Equal("encrypted-password-here"))
			Expect(config.PasswordEncrypted).To(BeTrue())
		})

		It("should support JSON marshaling with password fields", func() {
			original := &ServiceDatabaseConfig{
				TenantID:          1,
				ServiceCode:       "evidence-command",
				Driver:            "mysql",
				Host:              "localhost",
				Port:              3306,
				Database:          "evidence_cmd",
				Username:          "tenant1_user",
				Password:          "base64-encoded-password",
				PasswordEncrypted: true,
				SSLMode:           "require",
				MaxOpenConns:      100,
				MaxIdleConns:      10,
			}

			// Marshal to JSON
			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			// Unmarshal back to struct
			var decoded ServiceDatabaseConfig
			err = json.Unmarshal(data, &decoded)
			Expect(err).NotTo(HaveOccurred())

			// Verify all fields including password fields
			Expect(decoded.TenantID).To(Equal(original.TenantID))
			Expect(decoded.ServiceCode).To(Equal(original.ServiceCode))
			Expect(decoded.Driver).To(Equal(original.Driver))
			Expect(decoded.Host).To(Equal(original.Host))
			Expect(decoded.Port).To(Equal(original.Port))
			Expect(decoded.Database).To(Equal(original.Database))
			Expect(decoded.Username).To(Equal(original.Username))
			Expect(decoded.Password).To(Equal(original.Password))
			Expect(decoded.PasswordEncrypted).To(Equal(original.PasswordEncrypted))
			Expect(decoded.SSLMode).To(Equal(original.SSLMode))
			Expect(decoded.MaxOpenConns).To(Equal(original.MaxOpenConns))
			Expect(decoded.MaxIdleConns).To(Equal(original.MaxIdleConns))
		})

		It("should support empty password with PasswordEncrypted false", func() {
			config := &ServiceDatabaseConfig{
				TenantID:          2,
				ServiceCode:       "evidence-query",
				Password:          "",
				PasswordEncrypted: false,
			}

			Expect(config.Password).To(BeEmpty())
			Expect(config.PasswordEncrypted).To(BeFalse())
		})
	})
})
