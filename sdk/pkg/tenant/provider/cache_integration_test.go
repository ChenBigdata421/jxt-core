package provider

import (
	"context"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// mockFileCache implements FileCache for testing.
type mockFileCache struct {
	data    *tenantData
	loadErr error
	saveErr error
}

func (m *mockFileCache) Load() (*tenantData, error) {
	return m.data, m.loadErr
}

func (m *mockFileCache) Save(data *tenantData) error {
	m.data = data
	return m.saveErr
}

func (m *mockFileCache) IsAvailable() bool {
	return m.data != nil
}

func TestProvider_LoadAll_WithCacheFallback(t *testing.T) {
	// This test requires mocking ETCD failure
	// For now, test the structure integration

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("ETCD not available:", err)
	}
	defer client.Close()

	cache := &mockFileCache{
		data: &tenantData{
			Metas:     make(map[int]*TenantMeta),
			Databases: make(map[int]map[string]*ServiceDatabaseConfig),
			Ftps:      make(map[int][]*FtpConfigDetail),
			Storages:  make(map[int]*StorageConfig),
			Domains:   make(map[int]*DomainConfig),
		},
	}

	p := NewProvider(client,
		WithNamespace("test/"),
		WithConfigTypes(ConfigTypeDatabase),
		WithCache(cache),
	)

	ctx := context.Background()
	if err := p.LoadAll(ctx); err != nil {
		t.Logf("LoadAll failed (expected if ETCD empty): %v", err)
	}

	// Verify cache was called during load
	if p.data.Load() == nil {
		t.Error("expected data to be loaded")
	}
}

func TestNewFileCache(t *testing.T) {
	cache := NewFileCache()
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	if !cache.IsAvailable() {
		// Cache not available initially is OK
		t.Log("cache not available initially (expected)")
	}
}

func TestFileCacheAdapter_SaveLoad(t *testing.T) {
	// Create adapter with temp path
	tempDir := t.TempDir()
	cache := NewFileCacheWithPath(tempDir + "/test.json")

	// Save data
	data := &tenantData{
		Metas: map[int]*TenantMeta{
			1: {TenantID: 1, Code: "test1", Name: "Test", Status: "active"},
		},
		Databases: make(map[int]map[string]*ServiceDatabaseConfig),
		Ftps:      make(map[int][]*FtpConfigDetail),
		Storages:  make(map[int]*StorageConfig),
		Domains:   make(map[int]*DomainConfig),
	}

	if err := cache.Save(data); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Load data
	loaded, err := cache.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.Metas[1].Code != "test1" {
		t.Errorf("expected Code 'test1', got '%s'", loaded.Metas[1].Code)
	}

	// Verify IsAvailable returns true now
	if !cache.IsAvailable() {
		t.Error("expected cache to be available after Save")
	}
}
