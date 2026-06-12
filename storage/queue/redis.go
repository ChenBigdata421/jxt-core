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
	VisibilityTimeout int   `mapstructure:"visibilityTimeout"` // seconds
	BlockingTimeout   int   `mapstructure:"blockingTimeout"`   // seconds
	ReclaimInterval   int   `mapstructure:"reclaimInterval"`   // seconds
	BufferSize        int   `mapstructure:"bufferSize"`
	Concurrency       int   `mapstructure:"concurrency"`
	MaxDeliveries     int64 `mapstructure:"maxDeliveries"` // max reclaim attempts; 0 = unlimited
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
	// Copy to avoid mutating the caller's struct when applying defaults.
	cp := *cfg
	cfg = &cp
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
// The channel is buffered and never blocks the consumer — when full,
// errors are silently dropped. Callers MUST drain this channel (e.g.
// with a dedicated goroutine that logs/alerts) or errors will be lost.
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

// newMessageFromXMessage creates a queue.Message from a go-redis XMessage.
func newMessageFromXMessage(xmsg redis.XMessage, stream string) *Message {
	return &Message{
		id:     xmsg.ID,
		stream: stream,
		values: xmsg.Values,
	}
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
				m := newMessageFromXMessage(msg, sc.stream)
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
	// NOTE: Count is capped at BufferSize. If the backlog exceeds this,
	// each reclaim tick processes one batch and the next tick continues.
	// This creates a reclaim throughput ceiling of roughly
	//   BufferSize / ReclaimInterval messages/second.
	// This is intentional — reclaim is a safety net, not the primary path.
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
	// the visibility timeout. Messages that exceed MaxDeliveries are
	// ACKed (removed from pending) and reported as poison.
	var ids []string
	for _, p := range pending {
		if p.Idle < visibility {
			continue
		}
		// Poison message detection: if the message has been reclaimed
		// more than MaxDeliveries times, discard it to break the
		// infinite retry loop. RetryCount starts at 1 on first delivery.
		if sc.cfg.MaxDeliveries > 0 && p.RetryCount > sc.cfg.MaxDeliveries {
			if err := sc.ackByID(p.ID); err != nil {
				sc.pushErr(fmt.Errorf("poison ACK %s [%s]: %w", sc.stream, p.ID, err))
			} else {
				sc.pushErr(fmt.Errorf("poison message discarded: stream=%s id=%s deliveries=%d",
					sc.stream, p.ID, p.RetryCount))
			}
			continue
		}
		ids = append(ids, p.ID)
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
		m := newMessageFromXMessage(msg, sc.stream)
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
	// Use context.Background() so ACK can succeed even after Shutdown()
	// cancels sc.ctx. If we derived from sc.ctx, every ACK after cancel()
	// would fail with context.Canceled, leaving messages un-ACKed and
	// causing duplicate processing after restart.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := sc.client.XAck(ctx, sc.stream, sc.group, msg.GetID()).Result()
	return err
}

// ackByID ACKs a message by its string ID. Used for poison-message
// disposal where we don't have a storage.Messager.
func (sc *StreamConsumer) ackByID(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := sc.client.XAck(ctx, sc.stream, sc.group, id).Result()
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
	producer       *StreamProducer
	consumer       *StreamConsumer
	client         *redis.Client // producer client (shared, non-blocking)
	consumerClient *redis.Client // consumer client (separate, for blocking ops)
	consumerCfg    *ConsumerConfig

	// handler stores the registered consumer callback until Run() is called.
	handler storage.ConsumerFunc
	stream  string

	// runOnce prevents double-start — calling Run() more than once would
	// orphan the previous StreamConsumer's goroutines (goroutine leak).
	runOnce sync.Once

	// mu protects r.consumer for concurrent Run()/Shutdown() access.
	mu sync.RWMutex

	// initErr stores the error from consumer creation in Run().
	// Callers can check this via InitError() after Run() returns.
	initErr error
}

// NewRedis creates a Redis-backed queue adapter using a single client for
// both producer and consumer. For the three-client architecture, use
// NewRedisWithConsumer instead.
func NewRedis(client *redis.Client, producerCfg *ProducerConfig, consumerCfg *ConsumerConfig) (*Redis, error) {
	if client == nil {
		return nil, fmt.Errorf("queue: redis client must not be nil")
	}
	r := &Redis{
		client:         client,
		consumerClient: client, // same client for both
		producer:       NewStreamProducer(client, producerCfg),
		consumerCfg:    consumerCfg,
	}
	return r, nil
}

// NewRedisWithConsumer creates a Redis-backed queue adapter with separate
// clients for producer and consumer. The producerClient is used for XADD
// (non-blocking), the consumerClient is used for XREADGROUP (blocking).
func NewRedisWithConsumer(producerClient, consumerClient *redis.Client, producerCfg *ProducerConfig, consumerCfg *ConsumerConfig) (*Redis, error) {
	if producerClient == nil {
		return nil, fmt.Errorf("queue: producer redis client must not be nil")
	}
	if consumerClient == nil {
		return nil, fmt.Errorf("queue: consumer redis client must not be nil")
	}
	r := &Redis{
		client:         producerClient,
		consumerClient: consumerClient,
		producer:       NewStreamProducer(producerClient, producerCfg),
		consumerCfg:    consumerCfg,
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
		consumer, err := NewStreamConsumer(r.consumerClient, r.stream, r.stream, r.consumerCfg, r.handler)
		if err != nil {
			r.initErr = fmt.Errorf("queue: failed to create stream consumer for %q: %w", r.stream, err)
			// Log instead of panic — consumer creation failure (e.g. Redis
			// unreachable) should not crash the service.
			fmt.Fprintf(os.Stderr, "%v\n", r.initErr)
			return
		}
		r.mu.Lock()
		r.consumer = consumer
		r.mu.Unlock()
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
	r.mu.RLock()
	c := r.consumer
	r.mu.RUnlock()
	if c != nil {
		c.Shutdown()
	}
}
