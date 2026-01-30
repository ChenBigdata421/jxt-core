package database

import (
	"context"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Database Cache Suite")
}

var _ = Describe("Cache", func() {
	var (
		ctx      context.Context
		cache    *Cache
		testProv *provider.Provider
		etcdClient *clientv3.Client
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create a real ETCD client for testing
		// This will connect to localhost:2379 which should be available via infrastructure/docker-compose
		etcdClient, _ = clientv3.New(clientv3.Config{
			Endpoints:   []string{"localhost:2379"},
			DialTimeout: 5 * 1000 * time.Millisecond, // 5 seconds
		})

		testProv = provider.NewProvider(etcdClient)
		cache = NewCache(testProv)
	})

	AfterEach(func() {
		// Cleanup if needed
		if etcdClient != nil {
			etcdClient.Close()
		}
	})

	Describe("GetByID", func() {
		It("should return error for non-existent tenant", func() {
			// Test with a tenant ID that doesn't exist
			config, err := cache.GetByID(ctx, 99999)

			// Should return error
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database config not found"))
			Expect(config).To(BeNil())
		})
	})

	Describe("GetByCode", func() {
		It("should return error for non-existent tenant code", func() {
			// Test with a tenant code that doesn't exist
			// Note: GetByCode is not yet implemented
			config, err := cache.GetByCode(ctx, "nonexistent-code")

			// Should return error (not yet implemented)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not yet implemented"))
			Expect(config).To(BeNil())
		})
	})
})
