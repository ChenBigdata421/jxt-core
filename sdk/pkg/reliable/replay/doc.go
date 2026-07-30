// Package replay 实现 eligible-head 重放调度器（§6.2/§6.2.1）。
// 调度器本地调 handler（§6.3）：只重跑目标 handler、结果同步可观测、不破坏分区序。
// 每轮：FindEligibleHeads（聚合内最早未解决）→ ClaimForReplay（attempt+1）→ aggregate gate（若需要）
// → registry.Invoke → 三分支处置（经 Store.AdvanceDue/MoveToDeadLetter，D8 不绕过 §2.4）。
package replay
