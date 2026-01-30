package ftp

import (
	"context"
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/tenant/provider"
)

func TestFtpCache_GetByID(t *testing.T) {
	prov := provider.NewProvider(nil,
		provider.WithConfigTypes(provider.ConfigTypeFtp))

	cache := NewCache(prov)

	_, err := cache.GetByID(context.Background(), 1)
	if err == nil {
		t.Error("expected error for non-existent tenant")
	}
}
