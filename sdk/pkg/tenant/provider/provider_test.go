package provider

import (
	"testing"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestConfigType_String(t *testing.T) {
	tests := []struct {
		name string
		ct   ConfigType
		want string
	}{
		{"database", ConfigTypeDatabase, "database"},
		{"ftp", ConfigTypeFtp, "ftp"},
		{"storage", ConfigTypeStorage, "storage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ct.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProvider_LoadAll(t *testing.T) {
	// Skip if ETCD is not available
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 0, // Use default timeout
	})
	if err != nil {
		t.Skip("ETCD not available:", err)
	}
	defer client.Close()

	p := NewProvider(client,
		WithConfigTypes(ConfigTypeDatabase, ConfigTypeFtp),
		WithNamespace("test/"),
	)

	if p == nil {
		t.Fatal("NewProvider returned nil")
	}

	if len(p.configTypes) != 2 {
		t.Errorf("expected 2 config types, got %d", len(p.configTypes))
	}

	if p.namespace != "test/" {
		t.Errorf("expected namespace 'test/', got %s", p.namespace)
	}
}

func TestProvider_parseTenantID(t *testing.T) {
	tests := []struct {
		key      string
		expected int
		valid    bool
	}{
		{"jxt/tenants/1/meta", 1, true},
		{"jxt/tenants/100/database/driver", 100, true},
		{"invalid/key", 0, false},
		{"jxt/tenants/abc/meta", 0, false},
		{"other/tenants/5/meta", 0, false}, // wrong namespace
		{"jxt/tenants/meta", 0, false},      // no ID
	}

	p := &Provider{namespace: "jxt/"}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			id, ok := p.parseTenantID(tt.key)
			if ok != tt.valid {
				t.Errorf("parseTenantID() valid = %v, want %v", ok, tt.valid)
				return
			}
			if tt.valid && id != tt.expected {
				t.Errorf("parseTenantID() = %v, want %v", id, tt.expected)
			}
		})
	}
}
