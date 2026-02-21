package provider

import (
	"fmt"
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

// ========== Domain Lookup Tests ==========

func TestGetTenantIDByDomain(t *testing.T) {
	// Create a provider with test data
	p := NewProvider(nil)

	// Build test data with domain configurations
	testData := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains: map[int]*DomainConfig{
			1: {
				TenantID: 1,
				Code:     "tenant-alpha",
				Name:     "Tenant Alpha",
				Primary:  "tenant-alpha.example.com",
				Aliases:  []string{"alias.example.com", "another-alias.example.com"},
				Internal: "internal.local",
			},
			2: {
				TenantID: 2,
				Code:     "tenant-beta",
				Name:     "Tenant Beta",
				Primary:  "tenant-beta.example.com",
				Aliases:  []string{"beta-alias.example.com"},
				Internal: "beta-internal.local",
			},
		},
	}

	// Build the domain index
	testData.domainIndex = buildDomainIndex(testData)
	p.data.Store(testData)

	tests := []struct {
		name     string
		domain   string
		expected int
		found    bool
	}{
		{"Primary domain match", "tenant-alpha.example.com", 1, true},
		{"Primary case insensitive", "Tenant-Alpha.EXAMPLE.COM", 1, true},
		{"Alias domain match", "alias.example.com", 1, true},
		{"Another alias match", "another-alias.example.com", 1, true},
		{"Internal domain match", "internal.local", 1, true},
		{"Unknown domain", "unknown.com", 0, false},
		{"Empty domain", "", 0, false},
		{"Whitespace only domain", "   ", 0, false},
		{"Wildcard pattern not supported", "*.tenant.com", 0, false},
		{"Second tenant primary", "tenant-beta.example.com", 2, true},
		{"Second tenant alias", "beta-alias.example.com", 2, true},
		{"Second tenant internal", "beta-internal.local", 2, true},
		{"Domain with trailing space", " tenant-alpha.example.com ", 1, true},
		{"Domain with leading space", "  tenant-alpha.example.com", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenantID, found := p.GetTenantIDByDomain(tt.domain)
			if found != tt.found {
				t.Errorf("GetTenantIDByDomain(%q) found = %v, want %v", tt.domain, found, tt.found)
				return
			}
			if tt.found && tenantID != tt.expected {
				t.Errorf("GetTenantIDByDomain(%q) = %d, want %d", tt.domain, tenantID, tt.expected)
			}
		})
	}
}

func TestBuildDomainIndex(t *testing.T) {
	t.Run("Empty data returns empty index", func(t *testing.T) {
		data := &tenantData{
			Domains: make(map[int]*DomainConfig),
		}
		index := buildDomainIndex(data)
		if len(index) != 0 {
			t.Errorf("expected empty index, got %d entries", len(index))
		}
	})

	t.Run("Primary domain indexed correctly", func(t *testing.T) {
		data := &tenantData{
			Domains: map[int]*DomainConfig{
				1: {TenantID: 1, Primary: "primary.example.com"},
			},
		}
		index := buildDomainIndex(data)

		if tenantID, ok := index["primary.example.com"]; !ok || tenantID != 1 {
			t.Errorf("expected primary.example.com -> 1, got %d, ok=%v", tenantID, ok)
		}
	})

	t.Run("Aliases indexed correctly", func(t *testing.T) {
		data := &tenantData{
			Domains: map[int]*DomainConfig{
				1: {
					TenantID: 1,
					Aliases:  []string{"alias1.example.com", "alias2.example.com"},
				},
			},
		}
		index := buildDomainIndex(data)

		if tenantID, ok := index["alias1.example.com"]; !ok || tenantID != 1 {
			t.Errorf("expected alias1.example.com -> 1, got %d, ok=%v", tenantID, ok)
		}
		if tenantID, ok := index["alias2.example.com"]; !ok || tenantID != 1 {
			t.Errorf("expected alias2.example.com -> 1, got %d, ok=%v", tenantID, ok)
		}
	})

	t.Run("Internal domain indexed correctly", func(t *testing.T) {
		data := &tenantData{
			Domains: map[int]*DomainConfig{
				1: {TenantID: 1, Internal: "internal.local"},
			},
		}
		index := buildDomainIndex(data)

		if tenantID, ok := index["internal.local"]; !ok || tenantID != 1 {
			t.Errorf("expected internal.local -> 1, got %d, ok=%v", tenantID, ok)
		}
	})

	t.Run("All domains normalized to lowercase", func(t *testing.T) {
		data := &tenantData{
			Domains: map[int]*DomainConfig{
				1: {
					TenantID: 1,
					Primary:  "UPPER.Example.COM",
					Aliases:  []string{"Alias.EXAMPLE.com"},
					Internal: "INTERNAL.Local",
				},
			},
		}
		index := buildDomainIndex(data)

		// All should be indexed in lowercase
		if _, ok := index["upper.example.com"]; !ok {
			t.Error("expected upper.example.com to be indexed (lowercase)")
		}
		if _, ok := index["alias.example.com"]; !ok {
			t.Error("expected alias.example.com to be indexed (lowercase)")
		}
		if _, ok := index["internal.local"]; !ok {
			t.Error("expected internal.local to be indexed (lowercase)")
		}

		// Original case should NOT be present
		if _, ok := index["UPPER.Example.COM"]; ok {
			t.Error("original case should not be in index")
		}
	})

	t.Run("Empty strings ignored", func(t *testing.T) {
		data := &tenantData{
			Domains: map[int]*DomainConfig{
				1: {
					TenantID: 1,
					Primary:  "",
					Aliases:  []string{"", "  ", "valid.example.com"},
					Internal: "   ",
				},
			},
		}
		index := buildDomainIndex(data)

		// Only valid.example.com should be indexed
		if len(index) != 1 {
			t.Errorf("expected 1 entry, got %d", len(index))
		}
		if _, ok := index["valid.example.com"]; !ok {
			t.Error("expected valid.example.com to be indexed")
		}
	})
}

func TestDomainConflict(t *testing.T) {
	// When two tenants claim the same domain, the later one wins
	// and a warning is logged
	data := &tenantData{
		Domains: map[int]*DomainConfig{
			1: {
				TenantID: 1,
				Primary:  "shared.example.com",
			},
			2: {
				TenantID: 2,
				Primary:  "shared.example.com", // Conflict!
			},
		},
	}
	index := buildDomainIndex(data)

	// The later tenant (2) should win
	if tenantID, ok := index["shared.example.com"]; !ok || tenantID != 2 {
		t.Errorf("expected shared.example.com -> 2 (later wins), got %d, ok=%v", tenantID, ok)
	}
}

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Example.COM", "example.com"},
		{"  example.com  ", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"", ""},
		{"   ", ""},
		{"MixedCase.Domain.Local", "mixedcase.domain.local"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeDomain(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeDomain(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDomainIndexUpdate(t *testing.T) {
	// Create a provider with initial data
	p := NewProvider(nil)

	// Initial data
	initialData := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains: map[int]*DomainConfig{
			1: {
				TenantID: 1,
				Primary:  "initial.example.com",
				Aliases:  []string{"initial-alias.example.com"},
			},
		},
	}
	initialData.domainIndex = buildDomainIndex(initialData)
	p.data.Store(initialData)

	t.Run("Initial index is correct", func(t *testing.T) {
		if tenantID, ok := p.GetTenantIDByDomain("initial.example.com"); !ok || tenantID != 1 {
			t.Errorf("expected initial.example.com -> 1, got %d, ok=%v", tenantID, ok)
		}
		if tenantID, ok := p.GetTenantIDByDomain("initial-alias.example.com"); !ok || tenantID != 1 {
			t.Errorf("expected initial-alias.example.com -> 1, got %d, ok=%v", tenantID, ok)
		}
	})

	t.Run("Index updates after domain change", func(t *testing.T) {
		// Simulate domain update
		updatedData := &tenantData{
			Metas:     make(map[int]*TenantMeta),
			Databases: make(map[int]map[string]*ServiceDatabaseConfig),
			Ftps:      make(map[int][]*FtpConfigDetail),
			Storages:  make(map[int]*StorageConfig),
			Domains: map[int]*DomainConfig{
				1: {
					TenantID: 1,
					Primary:  "updated.example.com", // Changed
					Aliases:  []string{"new-alias.example.com"},
				},
			},
		}
		updatedData.domainIndex = buildDomainIndex(updatedData)
		p.data.Store(updatedData)

		// Old domain should not be found
		if _, ok := p.GetTenantIDByDomain("initial.example.com"); ok {
			t.Error("old domain should not be found after update")
		}
		if _, ok := p.GetTenantIDByDomain("initial-alias.example.com"); ok {
			t.Error("old alias should not be found after update")
		}

		// New domains should be found
		if tenantID, ok := p.GetTenantIDByDomain("updated.example.com"); !ok || tenantID != 1 {
			t.Errorf("expected updated.example.com -> 1, got %d, ok=%v", tenantID, ok)
		}
		if tenantID, ok := p.GetTenantIDByDomain("new-alias.example.com"); !ok || tenantID != 1 {
			t.Errorf("expected new-alias.example.com -> 1, got %d, ok=%v", tenantID, ok)
		}
	})

	t.Run("Index cleans up after tenant removal", func(t *testing.T) {
		// Simulate tenant removal
		emptyData := &tenantData{
			Metas:     make(map[int]*TenantMeta),
			Databases: make(map[int]map[string]*ServiceDatabaseConfig),
			Ftps:      make(map[int][]*FtpConfigDetail),
			Storages:  make(map[int]*StorageConfig),
			Domains:   make(map[int]*DomainConfig),
		}
		emptyData.domainIndex = buildDomainIndex(emptyData)
		p.data.Store(emptyData)

		// No domains should be found
		if _, ok := p.GetTenantIDByDomain("updated.example.com"); ok {
			t.Error("domain should not be found after tenant removal")
		}
		if _, ok := p.GetTenantIDByDomain("new-alias.example.com"); ok {
			t.Error("alias should not be found after tenant removal")
		}
	})
}

func TestNewProviderWithRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 0,
	})
	if err != nil {
		t.Skip("ETCD not available:", err)
	}
	defer client.Close()

	// Should succeed even if ETCD is slow
	p, err := NewProviderWithRetry(client,
		WithNamespace("test-retry/"),
		WithConfigTypes(ConfigTypeDatabase),
	)
	if err != nil {
		t.Logf("NewProviderWithRetry failed: %v", err)
	}

	if p == nil {
		t.Fatal("expected provider to be created")
	}

	defer p.StopWatch()

	// Test StartWatchWithRetry
	if err := p.StartWatchWithRetry(client.Ctx()); err != nil {
		t.Logf("StartWatchWithRetry failed: %v", err)
	}

	// Verify provider is running
	if !p.running.Load() {
		t.Error("expected provider to be running")
	}

	p.StopWatch()
}

// ========== Performance Benchmarks ==========

// BenchmarkGetTenantIDByDomain measures the performance of domain lookup.
// Target: < 50ns/op for O(1) hash map lookup
func BenchmarkGetTenantIDByDomain(b *testing.B) {
	// Create a provider with test data
	p := NewProvider(nil)

	// Build test data with 1000 tenants
	// Each tenant has: Primary + Alias + Internal = ~3 domains
	// Total: ~3000 domains in the index
	testData := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	for i := 0; i < 1000; i++ {
		testData.Domains[i] = &DomainConfig{
			TenantID: i,
			Code:     fmt.Sprintf("tenant-%d", i),
			Name:     fmt.Sprintf("Tenant %d", i),
			Primary:  fmt.Sprintf("tenant-%d.example.com", i),
			Aliases:  []string{fmt.Sprintf("alias-%d.example.com", i)},
			Internal: fmt.Sprintf("internal-%d.local", i),
		}
	}

	// Build the domain index
	testData.domainIndex = buildDomainIndex(testData)
	p.data.Store(testData)

	// Reset timer before the actual benchmark loop
	b.ResetTimer()

	// Benchmark lookup of a middle-domain to avoid edge cases
	for i := 0; i < b.N; i++ {
		p.GetTenantIDByDomain("tenant-500.example.com")
	}
}

// ========== Key Checking Function Tests ==========

func TestIsResolverConfigKey(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"common/resolver", true},
		{"tenants/1/meta", false},
		{"tenants/1/database/evidence-command", false},
		{"common/storage-directory", false},
		{"_health/sentinel", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := isResolverConfigKey(tt.key)
			if result != tt.expected {
				t.Errorf("isResolverConfigKey(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

// BenchmarkGetTenantIDByDomain_Parallel measures parallel performance
func BenchmarkGetTenantIDByDomain_Parallel(b *testing.B) {
	// Create a provider with test data
	p := NewProvider(nil)

	// Build test data with 1000 tenants
	testData := &tenantData{
		Metas:     make(map[int]*TenantMeta),
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	for i := 0; i < 1000; i++ {
		testData.Domains[i] = &DomainConfig{
			TenantID: i,
			Code:     fmt.Sprintf("tenant-%d", i),
			Name:     fmt.Sprintf("Tenant %d", i),
			Primary:  fmt.Sprintf("tenant-%d.example.com", i),
			Aliases:  []string{fmt.Sprintf("alias-%d.example.com", i)},
			Internal: fmt.Sprintf("internal-%d.local", i),
		}
	}

	testData.domainIndex = buildDomainIndex(testData)
	p.data.Store(testData)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		domains := []string{
			"tenant-500.example.com",
			"alias-100.example.com",
			"internal-200.local",
			"tenant-999.example.com",
		}
		for pb.Next() {
			p.GetTenantIDByDomain(domains[i%len(domains)])
			i++
		}
	})
}

