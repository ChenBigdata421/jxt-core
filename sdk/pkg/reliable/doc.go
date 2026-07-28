// Package reliable 提供可靠消费的状态机内核与持久化抽象（opus5-RCC-v2 §1~§8）。
//
// 根包零第三方依赖（J2）：只含纯契约类型与纯函数。gorm/数据库驱动/prometheus/gin
// 全部在 store 子树与消费服务侧。
//
// 关键不变量（由 store/repotest 在 MySQL/PostgreSQL 双方言上验证）：
//   - 一张 event_consumption 表 = 幂等 + 死信 + 重投调度（M1）。
//   - 五状态单状态机：PROCESSING | SUCCEEDED | RETRY_SCHEDULED | DEAD_LETTER | DISCARDED（§3）。
//   - TryClaim 独立提交、构造期保证（NewStore 派生独立 session，§3.3）；Mark* 显式接收 *gorm.DB（M14）。
//   - claim_id（ClaimToken）校验让 0 行从「设计边界」变成「可判定异常」（M3/edge #6/#6b）。
package reliable
