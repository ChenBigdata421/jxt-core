package provider

import (
	"context"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func BenchmarkProvider_GetServiceDatabaseConfig(b *testing.B) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		b.Skip("ETCD not available:", err)
	}
	defer client.Close()

	p := NewProvider(client,
		WithConfigTypes(ConfigTypeDatabase),
	)
	p.LoadAll(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg, _ := p.GetServiceDatabaseConfig(1, "evidence-command")
		_ = cfg
	}
}

func BenchmarkProvider_GetFtpConfigs(b *testing.B) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		b.Skip("ETCD not available:", err)
	}
	defer client.Close()

	p := NewProvider(client,
		WithConfigTypes(ConfigTypeFtp),
	)
	p.LoadAll(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg, _ := p.GetFtpConfigs(1)
		_ = cfg
	}
}

func BenchmarkProvider_GetStorageConfig(b *testing.B) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		b.Skip("ETCD not available:", err)
	}
	defer client.Close()

	p := NewProvider(client,
		WithConfigTypes(ConfigTypeStorage),
	)
	p.LoadAll(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg, _ := p.GetStorageConfig(1)
		_ = cfg
	}
}

func BenchmarkProvider_IsTenantEnabled(b *testing.B) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		b.Skip("ETCD not available:", err)
	}
	defer client.Close()

	p := NewProvider(client,
		WithConfigTypes(ConfigTypeDatabase),
	)
	p.LoadAll(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.IsTenantEnabled(1)
	}
}
