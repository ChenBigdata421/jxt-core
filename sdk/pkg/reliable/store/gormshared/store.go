package gormshared

import (
	"math/rand/v2"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"gorm.io/gorm"
)

// GormStore 实现 store.Store（共享，跨方言）。D17。
// 方法按关注点分文件（review #18，原 765 行单文件按 section banner 拆分，零行为变更）：
// claim.go / mark.go / replay.go / gate.go / anomaly.go / read.go。本文件只放结构体、构造器与 ptr helpers。
type GormStore struct {
	claimDB    *gorm.DB                 // TryClaim 独立提交用（D16：NewStore 派生 NewDB session）
	markDB     *gorm.DB                 // Mark*/Schedule/Discard 等用调用方传入的 db/tx
	classifier reliable.ErrorClassifier // D3：dup 检测 + 第 2 级分类
	jitter     func() float64           // 退避 jitter 源，产出 [0,1)。默认真随机（见 NewStore）。
}

// NewStore 构造共享 GormStore。db 必须是 pooled（非事务）句柄（§3.3，D16）：
// claimDB = db.Session(&gorm.Session{NewDB:true}) 隔离 WithContext 条件；ConnPool 仍是底层池，
// 故只要 db 是 pooled，TryClaim 的 Create 即独立提交。
//
// **构造期 guard（本轮评审 F1/A3）**：已在 gorm v1.24.2 核实 Session{NewDB:true} 只置 clone 标志、
// 不改 ConnPool——若 db 是事务句柄，claimDB 继承 tx ConnPool、TryClaim 静默并入调用方事务，§3.3 三大失效
// 全部复活。故 NewStore 用 ConnPool 类型断言在构造期拒绝 tx 句柄（panic），把「构造期保证」从措辞变成可执行断言。
// 该断言用正向接口（txCommitter），不依赖具体 ConnPool 类型——**R2 修正**：round-1 的 `*sql.Tx` 类型断言在
// `gorm.Config{PrepareStmt:true}` 下漏掉 gorm 的 `*PreparedStmtTX` 包装 → guard 失效、tx 句柄静默过关。
// 正向断言覆盖 `*sql.Tx` / `*PreparedStmtTX` 及任何实现 Commit/Rollback 的 tx 包装；D23 锁版 gorm，升版须重验。
type txCommitter interface {
	Commit() error
	Rollback() error
}

func NewStore(db *gorm.DB, classifier reliable.ErrorClassifier) *GormStore {
	if db == nil {
		panic("reliable: NewStore requires non-nil pooled *gorm.DB")
	}
	if classifier == nil {
		panic("reliable: NewStore requires non-nil classifier")
	}
	if db.Statement != nil {
		// pooled ConnPool（*sql.DB / *sql.Conn / *PreparedStmtDB）不实现 Commit/Rollback；
		// tx ConnPool（*sql.Tx / *PreparedStmtTX）实现 → panic。覆盖 PrepareStmt 包装（R2）。
		if _, ok := db.Statement.ConnPool.(txCommitter); ok {
			panic("reliable: NewStore requires a pooled (non-transaction) *gorm.DB — ConnPool implements Commit/Rollback (tx handle, incl. PrepareStmt-wrapped); TryClaim must independent-commit (§3.3, F1/A3, R2)")
		}
	}
	return &GormStore{
		claimDB:    db.Session(&gorm.Session{NewDB: true}),
		markDB:     db,
		classifier: classifier,
		// jitter 用 math/rand/v2 的真随机 [0,1)（v2 自动播种，无需 Seed）。
		// D4 原稿是确定性 hash（rowID*31 + attempt*... %100）：只产 100 个桶，且对连续 rowID 在
		// attempt=1 高度相关——DB blip 后 N 条同时首重试会聚到少数桶形成惊群，而非铺开。
		// Backoff 仍是纯函数（jitter 由调用方传入），这里只是把 jitter 源换成真随机；测试可经
		// Backoff 的 jitterFraction 形参注入固定值断言退避数学，不依赖本字段的随机性。
		jitter: rand.Float64,
	}
}

// 编译期保证 GormStore 实现 store.Store。
var _ store.Store = (*GormStore)(nil)

// —— ptr helpers（claim.go / mark.go 等用）——
func ptrInt32(v int32) *int32 { return &v }
func ptrInt64(v int64) *int64 { return &v }
func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
