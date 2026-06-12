package queue

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/storage"
)

func TestMemory_Append(t *testing.T) {
	type args struct {
		name    string
		message storage.Messager
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			"test01",
			args{
				name: "test",
				message: &Message{
					id:     "",
					stream: "test",
					values: map[string]interface{}{
						"key": "value",
					},
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMemory(100)
			if err := m.Append(tt.args.message); (err != nil) != tt.wantErr {
				t.Errorf("Append() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMemory_Register(t *testing.T) {
	log.SetFlags(19)
	type args struct {
		name string
		f    storage.ConsumerFunc
	}
	tests := []struct {
		name string
		args args
	}{
		{
			"test01",
			args{
				name: "test",
				f: func(message storage.Messager) error {
					fmt.Println(message.GetValues())
					return nil
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMemory(100)
			m.Register(tt.name, tt.args.f)
			if err := m.Append(&Message{
				stream: "test",
				values: map[string]interface{}{
					"key": "value",
				},
			}); err != nil {
				t.Error(err)
				return
			}
			go func() {
				m.Run()
			}()
			time.Sleep(3 * time.Second)
			m.Shutdown()
		})
	}
}

// TestMemory_ConcurrentAppend verifies that 100 goroutines appending to the
// same stream does not lose any messages. This is a regression test for the
// data race that existed when Append used Load+Store under RLock instead of
// the atomic LoadOrStore.
func TestMemory_ConcurrentAppend(t *testing.T) {
	const numGoroutines = 100

	m := NewMemory(uint(numGoroutines + 10))

	var received atomic.Int32

	// Register a consumer that counts every message it receives.
	m.Register("concurrent-stream", func(message storage.Messager) error {
		received.Add(1)
		return nil
	})

	// Give the consumer goroutine time to start.
	time.Sleep(50 * time.Millisecond)

	// Launch numGoroutines goroutines that all Append to the same stream.
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = m.Append(&Message{
				stream: "concurrent-stream",
				values: map[string]interface{}{
					"key": "value",
				},
			})
		}()
	}
	wg.Wait()

	// Wait for all messages to be delivered through the channel.
	deadline := time.After(5 * time.Second)
	for received.Load() < numGoroutines {
		select {
		case <-deadline:
			t.Fatalf("timed out: received %d/%d messages", received.Load(), numGoroutines)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if got := received.Load(); got != numGoroutines {
		t.Errorf("expected %d messages, got %d", numGoroutines, got)
	}
}
