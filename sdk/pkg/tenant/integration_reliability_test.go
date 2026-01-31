// +build integration

package tenant

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/database"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestIntegration_ReliabilityFeatures(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
	if etcdEndpoints == "" {
		etcdEndpoints = "localhost:2379"
	}

	// Create ETCD client
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{etcdEndpoints},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Skipf("ETCD not available: %v", err)
	}
	defer client.Close()

	// Setup test data
	ctx := context.Background()
	testNamespace := "test-reliability/"

	// Test 1: Create provider with retry
	t.Run("NewProviderWithRetry", func(t *testing.T) {
		fileCache := provider.NewFileCache()
		// Note: provider.FileCache doesn't have Clear method

		prov, err := provider.NewProviderWithRetry(client,
			provider.WithNamespace(testNamespace),
			provider.WithConfigTypes(provider.ConfigTypeDatabase),
			provider.WithCache(fileCache),
		)
		if err != nil {
			t.Logf("NewProviderWithRetry: %v", err)
		}
		if prov != nil {
			defer prov.StopWatch()
		}
	})

	// Test 2: Cache fallback
	t.Run("CacheFallback", func(t *testing.T) {
		fileCache := provider.NewFileCache()
		// Note: provider.FileCache doesn't have Clear method

		// Create provider with cache
		prov := provider.NewProvider(client,
			provider.WithNamespace(testNamespace),
			provider.WithConfigTypes(provider.ConfigTypeDatabase),
			provider.WithCache(fileCache),
		)

		// Load from ETCD
		prov.LoadAll(ctx)

		// Verify cache was created
		if !fileCache.IsAvailable() {
			t.Error("expected cache file to be created")
		}

		// Load from cache
		_, err := fileCache.Load()
		if err != nil {
			t.Errorf("cache.Load() failed: %v", err)
		}
	})

	// Test 3: Database cache usage
	t.Run("DatabaseCache", func(t *testing.T) {
		prov := provider.NewProvider(client,
			provider.WithNamespace(testNamespace),
			provider.WithConfigTypes(provider.ConfigTypeDatabase),
		)

		prov.LoadAll(ctx)

		dbCache := database.NewCache(prov)
		cfg, err := dbCache.GetByID(ctx, 1)
		if err == nil {
			t.Logf("Got config: %+v", cfg)
		}
	})
}
