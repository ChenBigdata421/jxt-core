package eventbus

import (
	"context"
	"fmt"
	"os"
	"strconv"
)

// 本文件落地 redpanda 主题拓扑优化方案 v2 的两项服务侧收敛（jxt-core 层）：
	//
//   docs/analysis/redpanda主题创建优化方案_v2.md
//
// 1. 就绪哨兵共享 helper（改动6 / §十五，形态乙）：4 个应用服务启动早期复用
//    WaitForTopologyReady 做 metadata 只读查询，消除 4 份重复实现。
// 2. AUTO_CREATE_TOPICS 开关（改动5）：jxt-core 生产路径默认不建 topic——存在性与
//    分区数由 infra bootstrap 独占收敛；仅开发/单测/CI 显式置 1 才放行建新。

// TopologyReadyTopic 是 redpanda topology 就绪哨兵专用 topic 名（方案 §十五 / 改动6）。
//
// 由 infrastructure/redpanda/topics-bootstrap.sh 在双遍断言通过后创建（1 分区，
// 不在 DESIRED 业务清单内）；bootstrap 开头先删（规则0）、断言全过才重建（规则1），
// 故其语义为「最近一次 bootstrap 成功收敛」。
//
// 服务启动时通过 WaitForTopologyReady 对其做 metadata 只读查询：
//
//	标志存在 ⟹ 最近一次 bootstrap exit 0 ⟹ 全部业务主题已在目标分区
//
// ——因此服务无需、也不应再持有分区数副本做断言（方案 §十四/§十五）。
const TopologyReadyTopic = "jxt.topology.ready"

// WaitForTopologyReady 在订阅/配置之前【单次】检查 redpanda topology 就绪哨兵（形态乙）。
//
// 实现走 metadata 只读查询（GetTopicPartitions），禁止 produce/consume 触发式探测——
// auto_create 若意外开启，触发式探测会把标志 topic 自建出来，使服务「自己骗自己」（方案 E3）。
//
// 返回：
//   - nil：标志存在（最近一次 bootstrap 成功），或总线非 Kafka（memory/NATS，未实现
//     TopicPartitionInfo）——后者自动放行，避免影响单测 / NATS 部署。
//   - 非 nil error：标志缺失（bootstrap 未成功完成）。调用方应 fail-fast（exit 1），
//     重试交给 orchestrator（Docker restart: on-failure 自带指数退避）。
//
// 本函数不在内部循环重试——重试由 orchestrator 负责（方案 §十五），进程内退避会掩盖
// bootstrap 失败（响亮的错 > 沉默的错）。
func WaitForTopologyReady(ctx context.Context, bus EventBus) error {
	querier, ok := bus.(TopicPartitionInfo)
	if !ok {
		// 非 Kafka 总线（memory/NATS）无 topology 概念，自动放行。
		return nil
	}
	if _, err := querier.GetTopicPartitions(ctx, TopologyReadyTopic); err != nil {
		return fmt.Errorf("redpanda topology not ready（%s 缺失；bootstrap 是否跑成功？）: %w", TopologyReadyTopic, err)
	}
	return nil
}

// WaitForTopicsExist 对给定 topics 做启动期存在性断言（方案 v4 §七）：任一在 redpanda 中
// 不存在即返回指名道姓的 error，提示补进 topics-manifest.sh 的 TOPICS 并重跑 bootstrap。
//
// 走 metadata 只读查询（GetTopicPartitions），禁止 produce/consume 触发式探测——
// auto_create 若意外开启，触发式探测会把 topic 自建出来（方案 E3）。
//
// 返回 nil 表示全部存在，或总线非 Kafka（memory/NATS，未实现 TopicPartitionInfo，
// 自动放行，避免影响单测 / NATS 部署）；返回非 nil error 表示存在缺失，调用方应 fail-fast。
//
// 与 WaitForTopologyReady 互补：后者证「最近一次 bootstrap 成功」（全局就绪），
// 本函数证「我要订阅的 topic 都在」（应用层自检，针对 manifest TOPICS ↔ 代码订阅集的
// 跨语言残留）。两者叠加使用。原 4 服务 waitTopologyReady 内重复的断言循环已收敛至此。
func WaitForTopicsExist(ctx context.Context, bus EventBus, topics []string) error {
	querier, ok := bus.(TopicPartitionInfo)
	if !ok {
		return nil // 非 Kafka 总线（memory/NATS）无 topic 存在性概念，自动放行。
	}
	var missing []string
	for _, t := range topics {
		if _, err := querier.GetTopicPartitions(ctx, t); err != nil {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("以下 topic 在 redpanda 中不存在，请补进 infrastructure/redpanda/topics-manifest.sh 的 TOPICS 并重跑 bootstrap: %v", missing)
	}
	return nil
}

// autoCreateTopicsEnabled 读取 AUTO_CREATE_TOPICS 开关（方案改动5）。
//
// 默认 false（生产路径不建 topic，由 infra bootstrap 独占建新）；仅本地开发/单测/CI
// 等不起 bootstrap 的场景显式置 1 开启。每次现读，便于测试与运行时切换；
// 非法值（含空）统一按 false 处理——fail-safe，宁可拒建也不要意外建。
func autoCreateTopicsEnabled() bool {
	v, err := strconv.ParseBool(os.Getenv("AUTO_CREATE_TOPICS"))
	return err == nil && v
}
