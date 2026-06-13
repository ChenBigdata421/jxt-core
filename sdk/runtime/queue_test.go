package runtime

import (
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

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

// captureQueue is a fake AdapterQueue that records the stream name passed to
// Register/Append and the message handed to Append.
type captureQueue struct {
	registeredStream string
	appendedStream   string
	appendedMessage  storage.Messager
}

func (c *captureQueue) String() string                                  { return "capture" }
func (c *captureQueue) Append(message storage.Messager) error {
	c.appendedStream = message.GetStream()
	c.appendedMessage = message
	return nil
}
func (c *captureQueue) Register(name string, _ storage.ConsumerFunc) { c.registeredStream = name }
func (c *captureQueue) Run()                                          {}
func (c *captureQueue) Shutdown()                                     {}

func TestQueue_Register_AppliesPrefix(t *testing.T) {
	// Non-empty prefix: registered stream name is prefix + ":" + name.
	cq := &captureQueue{}
	q := NewQueue("tenantA", cq)
	q.Register("operate_log_queue", func(storage.Messager) error { return nil })
	assert.Equal(t, "tenantA:operate_log_queue", cq.registeredStream)

	// Empty prefix: registered stream name is the identity (just the name).
	cqEmpty := &captureQueue{}
	qEmpty := NewQueue("", cqEmpty)
	qEmpty.Register("operate_log_queue", func(storage.Messager) error { return nil })
	assert.Equal(t, "operate_log_queue", cqEmpty.registeredStream)
}

func TestQueue_Append_AppliesPrefix(t *testing.T) {
	// Non-empty prefix: Append target stream is prefixed and PrefixKey is
	// stamped into the message values.
	cq := &captureQueue{}
	q := NewQueue("tenantB", cq)

	msg := &queue.Message{}
	msg.SetStream("events_stream")
	msg.SetValues(map[string]interface{}{"foo": "bar"})

	assert.NoError(t, q.Append(msg))

	// Prefixed stream name handed to the underlying adapter.
	assert.Equal(t, "tenantB:events_stream", cq.appendedStream)

	// PrefixKey (__host) tenant tag must be stamped into the message itself
	// (regression: GetValues() returns a shallow copy, so the old mutation never
	// reached the message and left the tag empty).
	assert.Equal(t, "tenantB", msg.GetPrefix(),
		"Append must stamp PrefixKey tenant tag into the message")

	// Empty prefix: stream name is identity.
	cqEmpty := &captureQueue{}
	qEmpty := NewQueue("", cqEmpty)
	msgEmpty := &queue.Message{}
	msgEmpty.SetStream("events_stream")
	msgEmpty.SetValues(map[string]interface{}{"foo": "bar"})

	assert.NoError(t, qEmpty.Append(msgEmpty))
	assert.Equal(t, "events_stream", cqEmpty.appendedStream)
}
