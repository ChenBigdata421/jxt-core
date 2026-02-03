package provider

import (
	"testing"
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
