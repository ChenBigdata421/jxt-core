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

	// 使用唯一的 Stream 名称避免冲突
	timestamp := time.Now().UnixNano()
	config := &eventbus.NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: fmt.Sprintf("nats-leak-test-%d", timestamp),
		JetStream: eventbus.JetStreamConfig{
			Enabled: true,
			Stream: eventbus.StreamConfig{
				Name:     fmt.Sprintf("LEAK_TEST_%d", timestamp),
				Subjects: []string{fmt.Sprintf("leak.test.%d.>", timestamp)},
				Storage:  "memory",
			},
		},
	}

	bus, err := eventbus.NewNATSEventBus(config)
	require.NoError(t, err)
	require.NotNil(t, bus)

	afterCreateGoroutines := runtime.NumGoroutine()
	t.Logf("After create goroutine count: %d (delta: %d)", afterCreateGoroutines, afterCreateGoroutines-initialGoroutines)

	ctx := context.Background()
	topic := fmt.Sprintf("leak.test.%d.topic1", timestamp)
	for i := 0; i < 10; i++ {
		envelope := eventbus.NewEnvelopeWithAutoID(
			fmt.Sprintf("test-agg-%d", i),
			"TestEvent",
			int64(i+1),
			[]byte(fmt.Sprintf(`{"test": %d}`, i)),
		)
		err := bus.PublishEnvelope(ctx, topic, envelope)
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
