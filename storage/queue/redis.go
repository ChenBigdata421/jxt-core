package queue

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ChenBigdata421/jxt-core/storage"
)

// consumerID is a unique identifier for this process, used as the Redis
// consumer name so that multiple instances of the same service are treated
// as distinct consumers within the same consumer group.
var consumerID string

func init() {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	consumerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

// ---------------------------------------------------------------------------
// Config types (passed in from sdk/config)
// ---------------------------------------------------------------------------

// ProducerConfig configures the stream producer.
type ProducerConfig struct {
	StreamMaxLength      int64 `mapstructure:"streamMaxLength"`
	ApproximateMaxLength bool  `mapstructure:"approximateMaxLength"`
}

// ConsumerConfig configures the stream consumer group.
type ConsumerConfig struct {
	VisibilityTimeout int `mapstructure:"visibilityTimeout"` // seconds
	BlockingTimeout   int `mapstructure:"blockingTimeout"`   // seconds
	ReclaimInterval   int `mapstructure:"reclaimInterval"`   // seconds
	BufferSize        int `mapstructure:"bufferSize"`
	Concurrency       int `mapstructure:"concurrency"`
}

// ---------------------------------------------------------------------------
// StreamProducer — XADD with optional MAXLEN ~ trimming
// ---------------------------------------------------------------------------

// StreamProducer publishes messages to Redis Streams.
type StreamProducer struct {
	client *redis.Client
	cfg    *ProducerConfig
}

// NewStreamProducer creates a producer that writes to Redis Streams via XADD.
func NewStreamProducer(client *redis.Client, cfg *ProducerConfig) *StreamProducer {
	if cfg == nil {
		cfg = &ProducerConfig{}
	}
	return &StreamProducer{client: client, cfg: cfg}
}

// Send adds a message to the given stream using XADD.
func (p *StreamProducer) Send(ctx context.Context, stream string, values map[string]interface{}) (string, error) {
	args := &redis.XAddArgs{
		Stream: stream,
		Values: values,
		ID:     "*", // auto-generate
	}
	if p.cfg.StreamMaxLength > 0 {
		args.MaxLen = p.cfg.StreamMaxLength
		if p.cfg.ApproximateMaxLength {
			args.Approx = true
		}
	}
	id, err := p.client.XAdd(ctx, args).Result()
	if err != nil {
		return "", fmt.Errorf("XADD %s: %w", stream, err)
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// StreamConsumer — XREADGROUP + XPENDING/XCLAIM + worker goroutines
// ---------------------------------------------------------------------------

// StreamConsumer reads messages from a Redis Stream consumer group with
// at-least-once delivery semantics.
//
// Three goroutine types:
//   - poll():    XREADGROUP BLOCK — pushes messages into a buffered channel
//   - reclaim(): periodic XPENDING + batch XCLAIM for timed-out messages
//   - N*work():  calls the user callback, then XACK on success
type StreamConsumer struct {
	client  *redis.Client
	cfg     *ConsumerConfig
	group   string // consumer group name (= registered stream name)
	stream  string // stream name (= registered stream name)
	handler storage.ConsumerFunc

	msgCh chan storage.Messager
	errCh chan error

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewStreamConsumer creates a consumer bound to the given stream and group.
func NewStreamConsumer(client *redis.Client, stream, group string, cfg *ConsumerConfig, handler storage.ConsumerFunc) (*StreamConsumer, error) {
	if cfg == nil {
		cfg = &ConsumerConfig{}
	}
	if cfg.VisibilityTimeout <= 0 {
		cfg.VisibilityTimeout = 60
	}
	if cfg.BlockingTimeout <= 0 {
		cfg.BlockingTimeout = 5
	}
	if cfg.ReclaimInterval <= 0 {
		cfg.ReclaimInterval = 1
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 100
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}

	// Ensure the consumer group exists (idempotent).
	if err := ensureGroup(client, stream, group); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	sc := &StreamConsumer{
		client:  client,
		cfg:     cfg,
		group:   group,
		stream:  stream,
		handler: handler,
		msgCh:   make(chan storage.Messager, cfg.BufferSize),
		errCh:   make(chan error, cfg.BufferSize),
		ctx:     ctx,
		cancel:  cancel,
	}
	return sc, nil
}

// Run starts the poll, reclaim, and worker goroutines.
func (sc *StreamConsumer) Run() {
	sc.wg.Add(1)
	go sc.poll()

	sc.wg.Add(1)
	go sc.reclaim()

	for i := 0; i < sc.cfg.Concurrency; i++ {
		sc.wg.Add(1)
		go sc.work()
	}
}

// Shutdown signals all goroutines to stop and waits for them to drain.
func (sc *StreamConsumer) Shutdown() {
	sc.cancel()
	sc.wg.Wait()
}

// Errors returns a read-only channel that surfaces consumer errors.
// The channel is buffered and never blocks the consumer.
func (sc *StreamConsumer) Errors() <-chan error {
	return sc.errCh
}

// ---------------------------------------------------------------------------
// Internal: ensure consumer group exists
// ---------------------------------------------------------------------------

// ensureGroup creates the stream and consumer group. Errors from XGROUP CREATE
// that contain "BUSYGROUP" are treated as success (idempotent).
func ensureGroup(client *redis.Client, stream, group string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try creating the group. Redis will create the stream automatically
	// when we use MKSTREAM.
	err := client.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil {
		// BUSYGROUP means the group already exists — that's fine.
		if isBusyGroup(err) {
			return nil
		}
		return fmt.Errorf("XGROUP CREATE %s %s: %w", stream, group, err)
	}
	return nil
}

func isBusyGroup(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "BUSYGROUP") || strings.Contains(s, "busygroup")
}

// ---------------------------------------------------------------------------
// poll — XREADGROUP BLOCK
// ---------------------------------------------------------------------------

func (sc *StreamConsumer) poll() {
	defer sc.wg.Done()

	consumerName := fmt.Sprintf("%s-poll-%s", sc.group, consumerID)
	blockTimeout := time.Duration(sc.cfg.BlockingTimeout) * time.Second

	// Backoff for persistent errors (e.g. NOGROUP after stream deletion).
	// Resets to zero on any successful read.
	backoff := time.Duration(0)
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-sc.ctx.Done():
			return
		default:
		}

		// Apply backoff before next attempt after a persistent error.
		if backoff > 0 {
			select {
			case <-sc.ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		// Use a per-read timeout so we can periodically check ctx.Done().
		readCtx, readCancel := context.WithTimeout(sc.ctx, blockTimeout)

		streams, err := sc.client.XReadGroup(readCtx, &redis.XReadGroupArgs{
			Group:    sc.group,
			Consumer: consumerName,
			Streams:  []string{sc.stream, ">"}, // ">" = only new messages
			Count:    int64(sc.cfg.BufferSize),
			Block:    blockTimeout,
		}).Result()

		readCancel()

		if err != nil {
			// Context cancelled means we're shutting down.
			if sc.ctx.Err() != nil {
				return
			}
			// redis.Nil just means the read timed out with no messages.
			if err == redis.Nil {
				backoff = 0
				continue
			}
			sc.pushErr(fmt.Errorf("XREADGROUP %s: %w", sc.stream, err))
			// Exponential backoff: 100ms → 200ms → 400ms → ... → 30s cap.
			if backoff == 0 {
				backoff = 100 * time.Millisecond
			} else {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}

		backoff = 0 // reset on success

		for _, xs := range streams {
			for _, msg := range xs.Messages {
				m := &Message{
					ID:     msg.ID,
					Stream: sc.stream,
					Values: msg.Values,
				}
				select {
				case sc.msgCh <- m:
				case <-sc.ctx.Done():
					return
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// reclaim — periodic XPENDING + batch XCLAIM
// ---------------------------------------------------------------------------

func (sc *StreamConsumer) reclaim() {
	defer sc.wg.Done()

	consumerName := fmt.Sprintf("%s-reclaim-%s", sc.group, consumerID)
	interval := time.Duration(sc.cfg.ReclaimInterval) * time.Second
	visibility := time.Duration(sc.cfg.VisibilityTimeout) * time.Second

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-sc.ctx.Done():
			return
		case <-ticker.C:
		}

		sc.doReclaim(consumerName, visibility)
	}
}

func (sc *StreamConsumer) doReclaim(consumerName string, visibility time.Duration) {
	ctx, cancel := context.WithTimeout(sc.ctx, 5*time.Second)
	defer cancel()

	// Step 1: Get pending entries summary.
	pending, err := sc.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: sc.stream,
		Group:  sc.group,
		Start:  "-",
		End:    "+",
		Count:  int64(sc.cfg.BufferSize),
	}).Result()

	if err != nil {
		if sc.ctx.Err() != nil {
			return
		}
		// No pending entries or group doesn't have pending yet.
		if err == redis.Nil {
			return
		}
		sc.pushErr(fmt.Errorf("XPENDING %s: %w", sc.stream, err))
		return
	}

	// Step 2: Collect IDs of messages that have been idle longer than
	// the visibility timeout.
	var ids []string
	for _, p := range pending {
		if p.Idle >= visibility {
			ids = append(ids, p.ID)
		}
	}
	if len(ids) == 0 {
		return
	}

	// Step 3: Batch XCLAIM.
	claimed, err := sc.client.XClaim(ctx, &redis.XClaimArgs{
		Stream:   sc.stream,
		Group:    sc.group,
		Consumer: consumerName,
		MinIdle:  visibility,
		Messages: ids,
	}).Result()

	if err != nil {
		if sc.ctx.Err() != nil {
			return
		}
		sc.pushErr(fmt.Errorf("XCLAIM %s: %w", sc.stream, err))
		return
	}

	for _, msg := range claimed {
		m := &Message{
			ID:     msg.ID,
			Stream: sc.stream,
			Values: msg.Values,
		}
		select {
		case sc.msgCh <- m:
		case <-sc.ctx.Done():
			return
		}
	}
}

// ---------------------------------------------------------------------------
// work — calls handler, then XACK on success
// ---------------------------------------------------------------------------

func (sc *StreamConsumer) work() {
	defer sc.wg.Done()

	for {
		select {
		case <-sc.ctx.Done():
			return
		case msg, ok := <-sc.msgCh:
			if !ok {
				return
			}
			// Re-check shutdown after receiving — Go's select picks
			// randomly between ctx.Done() and msgCh when both are
			// ready. Without this guard the handler would run during
			// shutdown and the subsequent ACK would fail because the
			// derived context is already canceled, causing duplicate
			// processing after restart.
			if sc.ctx.Err() != nil {
				return
			}
			if err := sc.handler(msg); err != nil {
				sc.pushErr(fmt.Errorf("handler %s [%s]: %w", sc.stream, msg.GetID(), err))
				// Not acking — message will become visible again after
				// VisibilityTimeout and be reclaimed.
				continue
			}
			// At-least-once: ACK only after successful callback.
			if err := sc.ack(msg); err != nil {
				sc.pushErr(fmt.Errorf("XACK %s [%s]: %w", sc.stream, msg.GetID(), err))
			}
		}
	}
}

func (sc *StreamConsumer) ack(msg storage.Messager) error {
	ctx, cancel := context.WithTimeout(sc.ctx, 3*time.Second)
	defer cancel()
	_, err := sc.client.XAck(ctx, sc.stream, sc.group, msg.GetID()).Result()
	return err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// pushErr writes an error to the buffered error channel. It never blocks;
// if the channel is full the error is silently dropped.
func (sc *StreamConsumer) pushErr(err error) {
	select {
	case sc.errCh <- err:
	default:
		// Channel full — drop to avoid blocking the consumer.
	}
}

// ---------------------------------------------------------------------------
// Redis adapter — implements storage.AdapterQueue
// ---------------------------------------------------------------------------

// Redis implements storage.AdapterQueue using native Redis Streams.
type Redis struct {
	producer    *StreamProducer
	consumer    *StreamConsumer
	client      *redis.Client
	consumerCfg *ConsumerConfig

	// handler stores the registered consumer callback until Run() is called.
	handler storage.ConsumerFunc
	stream  string

	// runOnce prevents double-start — calling Run() more than once would
	// orphan the previous StreamConsumer's goroutines (goroutine leak).
	runOnce sync.Once

	// initErr stores the error from consumer creation in Run().
	// Callers can check this via InitError() after Run() returns.
	initErr error
}

// NewRedis creates a Redis-backed queue adapter.
//
// In this stage a single *redis.Client is used for both producer and
// consumer. Part 2 will introduce the three-client architecture.
func NewRedis(client *redis.Client, producerCfg *ProducerConfig, consumerCfg *ConsumerConfig) (*Redis, error) {
	if client == nil {
		return nil, fmt.Errorf("queue: redis client must not be nil")
	}
	r := &Redis{
		client:      client,
		producer:    NewStreamProducer(client, producerCfg),
		consumerCfg: consumerCfg,
	}
	return r, nil
}

func (*Redis) String() string {
	return "redis"
}

func (r *Redis) Append(message storage.Messager) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := message.GetStream()
	if stream == "" {
		stream = r.stream
	}
	values := message.GetValues()
	if values == nil {
		values = make(map[string]interface{})
	}

	id, err := r.producer.Send(ctx, stream, values)
	if err != nil {
		return err
	}
	message.SetID(id)
	return nil
}

func (r *Redis) Register(name string, f storage.ConsumerFunc) {
	r.stream = name
	r.handler = f
}

func (r *Redis) Run() {
	r.runOnce.Do(func() {
		if r.stream == "" || r.handler == nil {
			r.initErr = fmt.Errorf("queue: Register() not called before Run()")
			return
		}
		// Create the consumer now that we have the stream name.
		consumer, err := NewStreamConsumer(r.client, r.stream, r.stream, r.consumerCfg, r.handler)
		if err != nil {
			r.initErr = fmt.Errorf("queue: failed to create stream consumer for %q: %w", r.stream, err)
			// Log instead of panic — consumer creation failure (e.g. Redis
			// unreachable) should not crash the service.
			fmt.Fprintf(os.Stderr, "%v\n", r.initErr)
			return
		}
		r.consumer = consumer
		r.consumer.Run()
	})
}

// InitError returns the error from the most recent Run() call, or nil if
// the consumer started successfully. Since AdapterQueue.Run() has no return
// value, callers use this to detect startup failures:
//
//	r.Run()
//	if err := r.InitError(); err != nil { ... }
func (r *Redis) InitError() error {
	return r.initErr
}

func (r *Redis) Shutdown() {
	if r.consumer != nil {
		r.consumer.Shutdown()
	}
}
