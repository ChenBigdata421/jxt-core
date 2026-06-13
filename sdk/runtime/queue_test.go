package runtime

import (
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/ChenBigdata421/jxt-core/storage"
	"github.com/ChenBigdata421/jxt-core/storage/queue"
)

func TestNewMemoryQueue(t *testing.T) {
	inner := queue.NewMemory(100)
	got := NewQueue("test", inner)

	// Verify the wrapper delegates String() to the inner queue.
	if got.String() != inner.String() {
		t.Errorf("NewQueue().String() = %q, want %q", got.String(), inner.String())
	}
}

func TestNewQueue_NilInner(t *testing.T) {
	// Wrapper with nil inner queue should not panic on String().
	got := NewQueue("test", nil)
	if got.String() != "" {
		t.Errorf("NewQueue(nil).String() = %q, want empty string", got.String())
	}
}

func TestQueue_Register(t *testing.T) {
	type fields struct {
		prefix string
		queue  storage.AdapterQueue
	}
	type args struct {
		name string
		f    storage.ConsumerFunc
	}
	client := redis.NewClient(&redis.Options{})
	q, err := queue.NewRedis(client, &queue.ProducerConfig{
		StreamMaxLength:      100,
		ApproximateMaxLength: true,
	}, &queue.ConsumerConfig{
		VisibilityTimeout: 60,
		BlockingTimeout:   5,
		ReclaimInterval:   1,
		BufferSize:        100,
		Concurrency:       10,
	})
	if err != nil {
		t.Error(err)
		return
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			"test0",
			fields{
				prefix: "",
				queue:  q,
			},
			args{
				name: "operate_log_queue",
				f: func(m storage.Messager) error {
					fmt.Println(m.GetValues())
					return nil
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Queue{
				prefix: tt.fields.prefix,
				queue:  tt.fields.queue,
			}
			e.Register(tt.args.name, tt.args.f)
			e.Run()
		})
	}
}
