// Package store 定义 reliable 持久化的方言中立抽象：Row 领域 model 与 Store 接口。
//
// 依赖 gorm 仅用于接口签名（M14：显式传 *gorm.DB，不定义 context key）。GORM 实现在 store/gormshared
// （共享，跨方言）；方言特定的 migration SQL 与 driver classifier 在 store/mysql / store/postgres。
//
// §3.3 关键约束：TryClaim 不接收 *gorm.DB（NewStore 派生独立 session，构造期保证）；
// 其余 Mark*/Schedule/Discard/AdvanceDue/MoveToDeadLetter 显式接收调用方 *gorm.DB，可加入业务事务。
//
// §2.4：Store 是 event_consumption 的唯一写入者——scheduler/lease 一律经 Store 方法，禁止 raw SQL 绕过。
package store
