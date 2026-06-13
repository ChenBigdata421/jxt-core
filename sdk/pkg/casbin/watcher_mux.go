package mycasbin

// CasbinWatcherMux replaces per-tenant Redis watchers with a single PSUBSCRIBE
// connection (Client #3). New tenants auto-match the wildcard pattern with zero
// additional connection overhead.
//
// Architecture:
//
//	Client #3 (PSUBSCRIBE /casbin/tenant/*)  — 1 connection for all tenants
//	Client #1 (PUBLISH /casbin/tenant/{id})  — shared pool
//
// Wire-compatible with go-admin-team/redis-watcher/v2 MSG payload for rolling
// upgrade: instances running the old per-tenant watcher and instances running
// the mux can coexist in the same Redis.
import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// --------------------------------------------------------------------------
// Wire-compatible MSG types (matches go-admin-team/redis-watcher/v2 exactly)
// --------------------------------------------------------------------------

// UpdateType mirrors redis-watcher/v2 UpdateType string constants.
type UpdateType string

const (
	Update                        UpdateType = "Update"
	UpdateForAddPolicy            UpdateType = "UpdateForAddPolicy"
	UpdateForRemovePolicy         UpdateType = "UpdateForRemovePolicy"
	UpdateForRemoveFilteredPolicy UpdateType = "UpdateForRemoveFilteredPolicy"
	UpdateForSavePolicy           UpdateType = "UpdateForSavePolicy"
	UpdateForAddPolicies          UpdateType = "UpdateForAddPolicies"
	UpdateForRemovePolicies       UpdateType = "UpdateForRemovePolicies"
	UpdateForUpdatePolicy         UpdateType = "UpdateForUpdatePolicy"
	UpdateForUpdatePolicies       UpdateType = "UpdateForUpdatePolicies"
)

// MSG is the wire payload, identical to redis-watcher/v2 MSG struct.
// JSON field names are implicit (exported Go fields) so the encoding is
// compatible.
type MSG struct {
	Method      UpdateType `json:"Method"`
	ID          string     `json:"ID"`
	Sec         string     `json:"Sec"`
	Ptype       string     `json:"Ptype"`
	OldRule     []string   `json:"OldRule"`
	OldRules    [][]string `json:"OldRules"`
	NewRule     []string   `json:"NewRule"`
	NewRules    [][]string `json:"NewRules"`
	FieldIndex  int        `json:"FieldIndex"`
	FieldValues []string   `json:"FieldValues"`
}

// --------------------------------------------------------------------------
// Channel naming
// --------------------------------------------------------------------------

const casbinChannelPrefix = "/casbin/tenant/"

func casbinChannel(tenantID int) string {
	return fmt.Sprintf("%s%d", casbinChannelPrefix, tenantID)
}

func casbinChannelPattern() string {
	return casbinChannelPrefix + "*"
}

// parseTenantIDFromChannel extracts the tenant ID from a channel name.
// Returns (id, true) on success, or (0, false) for malformed input (no panic).
// The bool distinguishes a malformed channel from a legitimate tenant ID 0
// (the deprecated Setup() path creates a tenant-0 enforcer), which a plain
// sentinel value cannot.
func parseTenantIDFromChannel(channel string) (int, bool) {
	if !strings.HasPrefix(channel, casbinChannelPrefix) {
		return 0, false
	}
	idStr := channel[len(casbinChannelPrefix):]
	if idStr == "" {
		return 0, false
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, false
	}
	return id, true
}

// --------------------------------------------------------------------------
// Enforcer lookup injection (avoids circular import: mycasbin -> runtime)
// --------------------------------------------------------------------------

// getEnforcerFn is set via SetEnforcerLookup by runtime.Application during init.
var getEnforcerFn func(tenantID int) *casbin.SyncedEnforcer

// SetEnforcerLookup sets the function used to look up tenant enforcers.
// Called by runtime during Application initialization.
func SetEnforcerLookup(fn func(tenantID int) *casbin.SyncedEnforcer) {
	getEnforcerFn = fn
}

// --------------------------------------------------------------------------
// CasbinWatcherMux — singleton
// --------------------------------------------------------------------------

// dispatchWorkers is the size of the worker pool that processes incoming
// policy messages. One worker would re-introduce head-of-line blocking (a slow
// LoadPolicy on one tenant stalls every other tenant and, past go-redis's 60s
// send timeout, drops messages). A bounded pool isolates tenants while capping
// goroutine growth.
//
// Each worker owns a dedicated queue and a tenant is pinned to a single worker
// via tenantID % dispatchWorkers. This preserves per-tenant message ordering
// (incremental Self{Add,Remove,Update}Policy ops are applied in publish order,
// so policy state cannot diverge across instances) while still isolating slow
// tenants from each other.
const dispatchWorkers = 8

// dispatchBufferSize bounds each worker's pending-message queue. Large enough to
// absorb policy-change bursts; bounded to avoid unbounded memory growth if a
// worker falls behind.
const dispatchBufferSize = 256

// shardForTenant maps a tenant to its dedicated worker index. Using a
// non-negative modulo keeps the index valid even for negative tenant IDs.
func shardForTenant(tenantID int) int {
	return ((tenantID % dispatchWorkers) + dispatchWorkers) % dispatchWorkers
}

// CasbinWatcherMux is a singleton that holds a single PSUBSCRIBE connection
// (Client #3) and dispatches incoming messages to the correct tenant enforcer.
type CasbinWatcherMux struct {
	pubClient  *redis.Client // Client #1 — shared pool for PUBLISH
	subClient  *redis.Client // Client #3 — dedicated subscriber

	localID string // UUID for self-ignore

	ctx    context.Context
	cancel context.CancelFunc

	// dispatchChs is a per-worker set of bounded queues. A slow LoadPolicy on one
	// tenant does not block the single subscriber goroutine (which would stall
	// every other tenant and, past go-redis's 60s send timeout, drop pub/sub
	// messages — silent stale policy for a security authorization system). Each
	// tenant is pinned to dispatchChs[tenantID % dispatchWorkers] so messages for
	// the same tenant are processed strictly in order by a single worker.
	dispatchChs []chan *redis.Message
	dispatchWG  sync.WaitGroup // tracks the worker goroutines for clean shutdown

	wg sync.WaitGroup // tracks the listen goroutine for clean shutdown

	mu     sync.Mutex
	closed bool
}

// watcherMux is the package-level singleton, initialised by InitCasbinWatcherMux.
// Uses atomic.Pointer for lock-free reads that are safe across goroutines without
// requiring callers to participate in sync.Once.Do — the atomic Store provides the
// happens-before guarantee that plain variable assignment does not.
var watcherMux atomic.Pointer[CasbinWatcherMux]

// watcherMuxOnce ensures InitCasbinWatcherMux runs exactly once.
var watcherMuxOnce sync.Once

// loadWatcherMux returns the current mux singleton (nil if not initialised).
func loadWatcherMux() *CasbinWatcherMux { return watcherMux.Load() }

// InitCasbinWatcherMux initialises the global CasbinWatcherMux singleton and
// starts the background listener goroutine. It is safe to call multiple times
// from concurrent goroutines (subsequent calls after the first are no-ops).
//
// Parameters:
//   - pubClient: Client #1 from config.GetRedisClient() for PUBLISH
//   - subClient: Client #3 from config.EnsureSubscriberClient() for PSUBSCRIBE
func InitCasbinWatcherMux(pubClient, subClient *redis.Client) {
	watcherMuxOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		mux := &CasbinWatcherMux{
			pubClient:   pubClient,
			subClient:   subClient,
			localID:     uuid.New().String(),
			ctx:         ctx,
			cancel:      cancel,
			dispatchChs: make([]chan *redis.Message, dispatchWorkers),
		}
		for i := range mux.dispatchChs {
			mux.dispatchChs[i] = make(chan *redis.Message, dispatchBufferSize)
		}

		watcherMux.Store(mux)

		// Start one worker per shard. Each worker drains only its own queue so a
		// tenant's messages are processed in order by a single goroutine.
		for i := 0; i < dispatchWorkers; i++ {
			mux.dispatchWG.Add(1)
			go mux.dispatchWorker(mux.dispatchChs[i])
		}

		// Start background listener
		mux.wg.Add(1)
		go mux.listen()
		logger.Infof("CasbinWatcherMux initialised, localID=%s", mux.localID)
	})
}

// ShutdownCasbinWatcherMux stops the mux listener and releases resources.
// It is safe to call when the mux is not initialised (no-op).
func ShutdownCasbinWatcherMux() {
	m := loadWatcherMux()
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.cancel()
	m.mu.Unlock()

	// Wait for the listen goroutine and the dispatch worker pool to fully
	// exit before returning, so callers can safely close Redis clients after
	// this returns. Workers see ctx.Done() and drain; in-flight LoadPolicy
	// calls finish naturally.
	m.wg.Wait()
	m.dispatchWG.Wait()
	logger.Infof("CasbinWatcherMux shut down")
}

// NewWatcher creates a per-tenant watcher handle that implements
// persist.Watcher + persist.WatcherEx + persist.UpdatableWatcher.
// The handle uses Client #1 for PUBLISH; lifecycle is managed by the mux.
func (m *CasbinWatcherMux) NewWatcher(tenantID int) persist.Watcher {
	return &casbinWatcher{
		mux:      m,
		tenantID: tenantID,
		channel:  casbinChannel(tenantID),
	}
}

// --------------------------------------------------------------------------
// Background listener
// --------------------------------------------------------------------------

// listen starts the PSUBSCRIBE loop with automatic reconnection.
func (m *CasbinWatcherMux) listen() {
	defer m.wg.Done()

	backoff := time.Second
	for {
		err := m.subscribeOnce(&backoff)
		if err == nil {
			// subscribeOnce returned nil only when context was cancelled — clean exit.
			return
		}

		m.mu.Lock()
		closed := m.closed
		m.mu.Unlock()
		if closed {
			return
		}

		logger.Errorf("CasbinWatcherMux subscription lost: %v, reconnecting in %v", err, backoff)

		select {
		case <-m.ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s cap
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

// subscribeOnce performs a single PSUBSCRIBE session. Returns nil when the
// context is cancelled (clean shutdown), or an error on unexpected disconnect.
// The backoff pointer is reset to time.Second after each successfully received
// message, so the next reconnection starts from the minimum delay.
func (m *CasbinWatcherMux) subscribeOnce(backoff *time.Duration) error {
	sub := m.subClient.PSubscribe(m.ctx, casbinChannelPattern())
	defer func() { _ = sub.Close() }()

	// Confirm subscription is active
	if err := sub.Ping(m.ctx); err != nil {
		return fmt.Errorf("PSUBSCRIBE ping failed: %w", err)
	}

	ch := sub.Channel()
	for {
		select {
		case <-m.ctx.Done():
			return nil // clean shutdown
		case msg, ok := <-ch:
			if !ok {
				return fmt.Errorf("subscription channel closed")
			}
			// Connection is healthy — reset backoff for next reconnection cycle.
			*backoff = time.Second
			// Pin the message to its tenant's worker so same-tenant messages stay
			// ordered. Unparseable channels fall back to shard 0 (handleMessage
			// logs the warning). On ctx cancellation we stop reading so shutdown is
			// not blocked behind a full queue.
			shard := 0
			if id, ok := parseTenantIDFromChannel(msg.Channel); ok {
				shard = shardForTenant(id)
			}
			select {
			case m.dispatchChs[shard] <- msg:
			case <-m.ctx.Done():
				return nil
			}
		}
	}
}

// dispatchWorker drains a single shard queue and processes its messages. Each
// worker runs handleMessage (which may issue a blocking LoadPolicy DB query) off
// the subscriber goroutine, so a slow tenant cannot stall the subscription or
// cause go-redis to drop pub/sub messages after its 60s send timeout. Because a
// tenant is always routed to the same worker, its messages are processed in the
// order they were published.
func (m *CasbinWatcherMux) dispatchWorker(ch <-chan *redis.Message) {
	defer m.dispatchWG.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			m.handleMessage(msg)
		}
	}
}

// handleMessage processes a single pub/sub message.
func (m *CasbinWatcherMux) handleMessage(msg *redis.Message) {
	tenantID, ok := parseTenantIDFromChannel(msg.Channel)
	if !ok {
		logger.Warnf("CasbinWatcherMux: cannot parse tenantID from channel %q", msg.Channel)
		return
	}

	// Deserialize MSG
	parsed := &MSG{}
	if err := json.Unmarshal([]byte(msg.Payload), parsed); err != nil {
		logger.Errorf("CasbinWatcherMux: failed to parse MSG payload: %v", err)
		return
	}

	// Self-ignore: skip messages published by our own instance
	if parsed.ID == m.localID {
		return
	}

	// Look up enforcer for this tenant
	if getEnforcerFn == nil {
		return
	}
	enforcer := getEnforcerFn(tenantID)
	if enforcer == nil {
		return
	}

	// Dispatch to the appropriate method on the enforcer, using the same
	// logic as redis-watcher/v2 DefaultUpdateCallback.
	dispatchToEnforcer(enforcer, parsed)
}

// dispatchToEnforcer mirrors redis-watcher/v2 DefaultUpdateCallback logic.
//
// Concurrency note: the Self* methods it calls are NOT individually locked by
// casbin — SyncedEnforcer does not override them, and addPolicyWithoutNotify
// reads/writes the model map without a lock (HasPolicy read → adapter call →
// model write). Safety depends on each enforcer having a single writer, which
// shardForTenant guarantees by routing a tenant to exactly one shard → one
// worker. Do not route two tenants that share an enforcer to different shards.
func dispatchToEnforcer(e casbin.IEnforcer, msg *MSG) {
	var res bool
	var err error

	switch msg.Method {
	case Update, UpdateForSavePolicy:
		err = e.LoadPolicy()
		res = true
	case UpdateForAddPolicy:
		res, err = e.SelfAddPolicy(msg.Sec, msg.Ptype, msg.NewRule)
	case UpdateForAddPolicies:
		res, err = e.SelfAddPolicies(msg.Sec, msg.Ptype, msg.NewRules)
	case UpdateForRemovePolicy:
		res, err = e.SelfRemovePolicy(msg.Sec, msg.Ptype, msg.NewRule)
	case UpdateForRemoveFilteredPolicy:
		res, err = e.SelfRemoveFilteredPolicy(msg.Sec, msg.Ptype, msg.FieldIndex, msg.FieldValues...)
	case UpdateForRemovePolicies:
		res, err = e.SelfRemovePolicies(msg.Sec, msg.Ptype, msg.NewRules)
	case UpdateForUpdatePolicy:
		res, err = e.SelfUpdatePolicy(msg.Sec, msg.Ptype, msg.OldRule, msg.NewRule)
	case UpdateForUpdatePolicies:
		res, err = e.SelfUpdatePolicies(msg.Sec, msg.Ptype, msg.OldRules, msg.NewRules)
	default:
		logger.Warnf("CasbinWatcherMux: unknown update type %q", msg.Method)
		return
	}

	if err != nil {
		logger.Errorf("CasbinWatcherMux: dispatch %s failed: %v", msg.Method, err)
	}
	if !res {
		logger.Warnf("CasbinWatcherMux: dispatch %s returned false", msg.Method)
	}
}

// publish serialises a MSG and publishes it to the tenant's channel via Client #1.
func (m *CasbinWatcherMux) publish(tenantID int, payload *MSG) error {
	if m == nil || m.pubClient == nil {
		return nil // graceful degradation: no Redis
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	payload.ID = m.localID
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal MSG: %w", err)
	}
	return m.pubClient.Publish(context.Background(), casbinChannel(tenantID), data).Err()
}

// --------------------------------------------------------------------------
// casbinWatcher — per-tenant handle implementing persist.Watcher interfaces
// --------------------------------------------------------------------------

// Compile-time interface checks.
var (
	_ persist.Watcher         = (*casbinWatcher)(nil)
	_ persist.WatcherEx       = (*casbinWatcher)(nil)
	_ persist.UpdatableWatcher = (*casbinWatcher)(nil)
)

// casbinWatcher is a lightweight per-tenant handle. It only PUBLISHes; the
// receive side is handled centrally by CasbinWatcherMux.listen().
type casbinWatcher struct {
	mux      *CasbinWatcherMux
	tenantID int
	channel  string
}

// SetUpdateCallback is a no-op: the mux handles callbacks centrally.
// It satisfies persist.Watcher.
func (w *casbinWatcher) SetUpdateCallback(func(string)) error {
	return nil
}

// Update publishes a full-reload notification for this tenant.
func (w *casbinWatcher) Update() error {
	return w.mux.publish(w.tenantID, &MSG{Method: Update})
}

// Close is a no-op: lifecycle is managed by the mux singleton.
func (w *casbinWatcher) Close() {
	// no-op
}

// ---------- persist.WatcherEx ----------

func (w *casbinWatcher) UpdateForAddPolicy(sec, ptype string, params ...string) error {
	return w.mux.publish(w.tenantID, &MSG{
		Method:  UpdateForAddPolicy,
		Sec:     sec,
		Ptype:   ptype,
		NewRule: params,
	})
}

func (w *casbinWatcher) UpdateForRemovePolicy(sec, ptype string, params ...string) error {
	return w.mux.publish(w.tenantID, &MSG{
		Method:  UpdateForRemovePolicy,
		Sec:     sec,
		Ptype:   ptype,
		NewRule: params,
	})
}

func (w *casbinWatcher) UpdateForRemoveFilteredPolicy(sec, ptype string, fieldIndex int, fieldValues ...string) error {
	return w.mux.publish(w.tenantID, &MSG{
		Method:      UpdateForRemoveFilteredPolicy,
		Sec:         sec,
		Ptype:       ptype,
		FieldIndex:  fieldIndex,
		FieldValues: fieldValues,
	})
}

func (w *casbinWatcher) UpdateForSavePolicy(model.Model) error {
	return w.mux.publish(w.tenantID, &MSG{
		Method: UpdateForSavePolicy,
	})
}

func (w *casbinWatcher) UpdateForAddPolicies(sec string, ptype string, rules ...[]string) error {
	return w.mux.publish(w.tenantID, &MSG{
		Method:   UpdateForAddPolicies,
		Sec:      sec,
		Ptype:    ptype,
		NewRules: rules,
	})
}

func (w *casbinWatcher) UpdateForRemovePolicies(sec string, ptype string, rules ...[]string) error {
	return w.mux.publish(w.tenantID, &MSG{
		Method:   UpdateForRemovePolicies,
		Sec:      sec,
		Ptype:    ptype,
		NewRules: rules,
	})
}

// ---------- persist.UpdatableWatcher ----------

func (w *casbinWatcher) UpdateForUpdatePolicy(sec string, ptype string, oldRule, newRule []string) error {
	return w.mux.publish(w.tenantID, &MSG{
		Method:  UpdateForUpdatePolicy,
		Sec:     sec,
		Ptype:   ptype,
		OldRule: oldRule,
		NewRule: newRule,
	})
}

func (w *casbinWatcher) UpdateForUpdatePolicies(sec string, ptype string, oldRules, newRules [][]string) error {
	return w.mux.publish(w.tenantID, &MSG{
		Method:   UpdateForUpdatePolicies,
		Sec:      sec,
		Ptype:    ptype,
		OldRules: oldRules,
		NewRules: newRules,
	})
}
