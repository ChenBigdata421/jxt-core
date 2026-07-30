// Package lease 周期**观测**过期 PROCESSING 租约（§3.2）。
//
// **D20：观测与再占位分离**。runner 只触发 store.ObserveExpiredLeases（扫描 + 幂等记
// LEASE_ORPHAN，D14 批量），**不修改任何行的 status 与 ownership**：租约孤儿行 payload IS NULL，
// 五状态里没有能表示「无主、可再占位、无 payload」的状态，清 ownership 会直接撞死
// chk_processing_owner。重新占位的唯一路径是 TryClaim 发现 lease_expires_at < now 时的内联 CAS
// 续占（写入新 claim_id，约束全程成立，旧 token 失效）——与 spec §3.2「payload IS NULL 靠
// broker 重投」一致。
//
// 服务的调度触发（全局有界 worker pool）在服务侧编排，core 不按租户常驻 goroutine（§8.5）。
package lease
