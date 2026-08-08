// Package gate 提供 aggregate-gate 的获取/释放机制（§6.2.1）。
//
// 它是 acquire-or-error + 保证 release 的单一拥有者，刻意不夹带 ordering 与让路策略：
// replay 是 gate-then-claim（见 replay/scheduler.go processOne），live 将是 claim-then-gate（F3-bis）——
// 各 caller 自决。helper 只负责「抢到 gate 就保证释放」这条机制。
//
// 为什么独立成包（而非放进根 reliable 包）：store 依赖 reliable（类型定义在根包），若 reliable 反过来
// 直接持有 acquire/release 逻辑就会 import store → 成环。本包作为叶子，可同时 import reliable 与 store，
// 给根包留出无环的 import 图（J2/cycle 不变量，见 gates_test.go 的 TestGate_RootPackageNoCycleImports）。
package gate

import (
	"context"
	"errors"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"gorm.io/gorm"
)

// ReleaseTimeout 是释放 aggregate gate 用的独立 ctx 超时。release 是纯清理，**不得继承业务 ctx**——
// tickTimeout fire / 上层取消时业务 ctx 已 done，ReleaseAggregateGate 的 DELETE 会随 ctx 失败 →
// gate 残留到 TTL、卡住同聚合重放（P2）。独立短超时让清理与业务生命周期解耦。
//
// 这是该超时的唯一规范定义：原 replay 包内的 gateReleaseTimeout 局部常量已迁入此处并导出，
// 让 replay 与（未来的）live 路径共享同一数值，避免两处漂移。
const ReleaseTimeout = 3 * time.Second

// Acquire 获取 (tenant,type,id) 的 DB lease 并返回一个 release 闭包。
//
// 成功时返回 (release, nil)：caller 必须 defer 调用 release()。release() 返回 ReleaseAggregateGate 的
// 错误（透传给 caller），这正是 replay 用来 raise REPLAY_GATE_RELEASE_FAILED 的属性——因此 caller
// **不得** `_ =` 吞掉它的返回值。release 内部用独立的 context.WithTimeout(context.Background(),
// ReleaseTimeout)，不继承业务 ctx，故业务 ctx 取消后 release 仍能成功。
//
// key.Empty() 时既不 acquire 也不 release：返回一个 non-nil 的 no-op release + nil err——
// 这样即使 caller 忘了自检 key.Empty()，也不会拿到「err==nil 却什么都没抢到、release 还会按空 token 删」
// 的危险组合（Store 对空 key 返回 ("",nil)，凭空 token 删除会误伤）。
//
// acquire 失败（contended 或 DB 错误）时返回 (nil, err)：caller 不持有任何 release，无需 defer。
// 用 IsContention(err) 区分「让路（他人持租约）」与真失败——前者 caller 选择让路策略
// （replay: IncReplayBlocked+skip；live: MarkFailed(retryable)+ACK），后者走错误处理。
//
// helper 不决定 MarkFailed(retryable) 这类策略，那是 caller 的职责（见包注释）。
func Acquire(ctx context.Context, st store.Store, db *gorm.DB, key reliable.AggregateGateKey,
	holder string, ttl time.Duration) (release func() error, err error) {

	// 空聚合身份（通知类事件，无串行约束）→ 跳过 gate。返回 no-op release 保证 caller 的 defer 安全。
	if key.Empty() {
		return func() error { return nil }, nil
	}

	token, gerr := st.AcquireAggregateGate(ctx, db, key, holder, ttl)
	if gerr != nil {
		// 抢不到（ErrRetryLater=他人持租约）或真 DB 失败：不持有 lease，没有东西要释放。
		return nil, gerr
	}

	// release 是 release-ctx 纪律的唯一拥有者：独立短超时 ctx + 透传 DELETE 错误。
	return func() error {
		relCtx, relCancel := context.WithTimeout(context.Background(), ReleaseTimeout)
		defer relCancel()
		return st.ReleaseAggregateGate(relCtx, db, token)
	}, nil
}

// IsContention 报告 err 是否为「gate 被他人持有」的让路信号（F9）。
//
// 等价于 errors.Is(err, reliable.ErrRetryLater)（支持 Unwrap 链）。集中此谓词避免各 caller
// 重复推导「让路哨兵是哪个」——replay 的 acquire 失败分支与未来的 live 路径共用同一判定。
func IsContention(err error) bool {
	return errors.Is(err, reliable.ErrRetryLater)
}
