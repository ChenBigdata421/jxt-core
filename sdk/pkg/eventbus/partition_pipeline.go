package eventbus

import (
	"github.com/IBM/sarama"
)

// commitDecision 是 decideCommitable 的纯逻辑判定结果（无网络 I/O）。
type commitDecision int

const (
	commitSuccess      commitDecision = iota // → commitable=true
	commitRegularFail                        // 普通消息失败 → commitable=true（at-most-once，丢弃）
	commitEnvelopeFail                       // Envelope 终态失败 → 异步 DLQ
)

// inflightEntry 一条在飞消息的协调状态（仅主 goroutine 访问）。
type inflightEntry struct {
	msg        *sarama.ConsumerMessage
	isEnvelope bool
	settled    bool // handler 已完成（已拿到结果）
	dlqPending bool // Envelope 终态失败、异步 DLQ 进行中
	commitable bool // 可推进前沿（成功 / 普通失败 / DLQ 成功）；纯失败 Envelope=false
}

// completion 是 handler 完成事件（经 bridge 搬运进 compCh）。
type completion struct {
	offset   int64
	err      error
	canceled bool // ⭐ 决策 1-A：bridge 在 ctx.Done 后补捞转发时置 true；据此判定「取消、留待重投递」，不嗅探错误类型
}

// dlqResult 是异步 DLQ 投递的回送结果。
type dlqResult struct {
	offset int64
	ok     bool // DLQ 投递成功=true（→commitable）；失败=false（→策略 A 阻塞前沿 + 告警）
}

// decideCommitable 只做纯逻辑判定（无网络 I/O）。
func decideCommitable(e *inflightEntry, err error) commitDecision {
	if err == nil {
		return commitSuccess
	}
	if e.isEnvelope {
		return commitEnvelopeFail
	}
	return commitRegularFail
}

// 占位：partitionPipeline 主体在后续任务补全。这里先保证文件可编译。
type partitionPipeline struct {
	cfg PipelineConfig
}
