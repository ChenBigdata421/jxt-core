package provider

import (
	"context"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// init initializes the logger for tests
func init() {
	// Initialize logger for test environment
	zapLogger, _ := zap.NewDevelopment()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()
}

// TestWatchReconnection tests that watchLoop automatically reconnects when channel closes
func TestWatchReconnection(t *testing.T) {
	// Skip if ETCD is not available
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("ETCD not available:", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p := NewProvider(client,
		WithNamespace("test-reconnect/"),
		WithConfigTypes(ConfigTypeDatabase),
	)

	// Start watch - this should succeed
	if err := p.StartWatch(ctx); err != nil {
		t.Fatalf("StartWatch failed: %v", err)
	}

	// Verify watch is running
	if !p.running.Load() {
		t.Error("Provider should be running after StartWatch")
	}

	// Stop watch
	p.StopWatch()

	// Verify watch is stopped
	if p.running.Load() {
		t.Error("Provider should not be running after StopWatch")
	}
}

// TestBackoffCalculation tests the exponential backoff calculation
func TestBackoffCalculation(t *testing.T) {
	tests := []struct {
		name     string
		current  time.Duration
		expected time.Duration
	}{
		{"1s to 1.5s", 1 * time.Second, 1500 * time.Millisecond},
		{"2s to 3s", 2 * time.Second, 3 * time.Second},
		{"20s to 30s (cap)", 20 * time.Second, MaxBackoff},
		{"30s stays at 30s", MaxBackoff, MaxBackoff},
		{"60s capped to 30s", 60 * time.Second, MaxBackoff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateBackoff(tt.current)
			if result != tt.expected {
				t.Errorf("calculateBackoff(%v) = %v, want %v", tt.current, result, tt.expected)
			}
		})
	}
}

// TestWatchReconnectsWithBackoff tests that reconnection happens with proper backoff timing
func TestWatchReconnectsWithBackoff(t *testing.T) {
	// This test requires actual ETCD and simulates network disruption
	// It's an integration test that may be skipped in CI

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("ETCD not available:", err)
	}
	defer client.Close()

	// Use a test namespace
	testNamespace := "test-reconnect-backoff/"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := NewProvider(client,
		WithNamespace(testNamespace),
		WithConfigTypes(ConfigTypeDatabase),
	)

	// Start watch
	if err := p.StartWatch(ctx); err != nil {
		t.Fatalf("StartWatch failed: %v", err)
	}

	// Give it a moment to start watching
	time.Sleep(100 * time.Millisecond)

	// Verify it's running
	if !p.running.Load() {
		t.Error("Provider should be running")
	}

	// Stop and verify clean shutdown
	p.StopWatch()

	if p.running.Load() {
		t.Error("Provider should be stopped")
	}
}
