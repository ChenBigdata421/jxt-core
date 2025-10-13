package performance_tests

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNATSGoroutineLeakDetection(t *testing.T) {
	t.Log("NATS EventBus Goroutine Leak Detection")

	// Initialize logger
	zapLogger, _ := zap.NewDevelopment()
	logger.Logger = zapLogger
	logger.DefaultLogger = zapLogger.Sugar()

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutine count: %d", initialGoroutines)

	printGoroutineStacks(t, "initial", initialGoroutines)

	config := &eventbus.NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: fmt.Sprintf("nats-leak-test-%d", time.Now().Unix()),
		JetStream: eventbus.JetStreamConfig{
			Enabled: true,
			Stream: eventbus.StreamConfig{
				Name:     fmt.Sprintf("LEAK_TEST_%d", time.Now().Unix()),
				Subjects: []string{"leak.test.>"},
				Storage:  "file",
			},
		},
	}

	bus, err := eventbus.NewNATSEventBus(config)
	require.NoError(t, err)
	require.NotNil(t, bus)

	afterCreateGoroutines := runtime.NumGoroutine()
	t.Logf("After create goroutine count: %d (delta: %d)", afterCreateGoroutines, afterCreateGoroutines-initialGoroutines)

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		envelope := &eventbus.Envelope{
			AggregateID:  fmt.Sprintf("test-agg-%d", i),
			EventType:    "TestEvent",
			EventVersion: int64(i + 1),
			Timestamp:    time.Now(),
			Payload:      []byte(fmt.Sprintf(`{"test": %d}`, i)),
		}
		err := bus.PublishEnvelope(ctx, "leak.test.topic1", envelope)
		require.NoError(t, err)
	}

	time.Sleep(2 * time.Second)

	err = bus.Close()
	require.NoError(t, err)

	time.Sleep(5 * time.Second)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	leak := finalGoroutines - initialGoroutines
	t.Logf("\nFinal goroutine count: %d (leak: %d)", finalGoroutines, leak)

	printGoroutineStacks(t, "final", finalGoroutines)

	if leak > 0 {
		t.Logf("\nDetected %d goroutine leak!", leak)
	}
}

func printGoroutineStacks(t *testing.T, label string, count int) {
	buf := make([]byte, 1<<20)
	stackSize := runtime.Stack(buf, true)
	stacks := string(buf[:stackSize])

	t.Logf("\n%s Goroutine stacks (total: %d):", label, count)

	lines := strings.Split(stacks, "\n")
	for i, line := range lines {
		if i > 100 {
			t.Logf("  ... (omitted %d lines)", len(lines)-100)
			break
		}
		t.Logf("  %s", line)
	}
}
