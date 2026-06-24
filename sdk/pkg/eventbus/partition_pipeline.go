package eventbus

import (
	"context"
	"fmt"
	"time"

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

// poolSubmitter 把消息异步提交给 actor pool（生产=HollywoodActorPool，测试=fake）。
type poolSubmitter interface {
	ProcessMessage(ctx context.Context, msg *AggregateMessage) error
}

// offsetMarker 只暴露 MarkMessage（sarama.ConsumerGroupSession 的窄适配）。
type offsetMarker interface {
	MarkMessage(msg *sarama.ConsumerMessage, metadata string)
}

// DLQSender 异步 DLQ 投递（jxt-core 自定义接口；各服务把自己的 DLQService 适配进来）。
// Send 返回 false 表示投递失败 → 策略 A 阻塞前沿。
type DLQSender interface {
	Send(ctx context.Context, msg *sarama.ConsumerMessage) bool
}

// poisonAlerter 毒消息强告警（策略 A）。
type poisonAlerter interface {
	AlertPoisonMessage(msg *sarama.ConsumerMessage)
}

// noopAlerter 默认无操作告警器。
type noopAlerter struct{}

func (noopAlerter) AlertPoisonMessage(*sarama.ConsumerMessage) {}

// partitionPipeline 单分区内消费流水线协调器（全部可变状态由 run() 主 goroutine 持有）。
type partitionPipeline struct {
	cfg         PipelineConfig
	pool        poolSubmitter
	dlq         DLQSender // 可为 nil（无 DLQ 时 envelope 失败直接策略 A 阻塞）
	alert       poisonAlerter
	buildAggMsg func(*sarama.ConsumerMessage) *AggregateMessage // kafka.go 注入：构造含 Done 的 AggregateMessage
}

// newPartitionPipeline 构造协调器并分配两个 chan。
// ⭐ buffer 硬约束（设计 §4.3）：两个 chan 容量都必须 == windowSize，且与 windowSize 同行分配，防漂移。
func newPartitionPipeline(cfg PipelineConfig, sessionTimeout time.Duration) (*partitionPipeline, chan completion, chan dlqResult) {
	if err := cfg.validate(sessionTimeout); err != nil {
		panic(fmt.Sprintf("partition pipeline config invalid: %v", err))
	}
	compCh := make(chan completion, cfg.WindowSize)   // 同行分配，杜绝单独配置漂移
	dlqDoneCh := make(chan dlqResult, cfg.WindowSize) // 同行分配，杜绝单独配置漂移
	if cap(compCh) < cfg.WindowSize || cap(dlqDoneCh) < cfg.WindowSize {
		panic(fmt.Sprintf("buffer invariant violated: compCh=%d dlqDoneCh=%d windowSize=%d", cap(compCh), cap(dlqDoneCh), cfg.WindowSize))
	}
	return &partitionPipeline{cfg: cfg, alert: noopAlerter{}}, compCh, dlqDoneCh
}

// advanceFrontier 推进连续前缀，返回推进段最高位 msg（供 mark-once）。仅主 goroutine 调用，纯 map 操作。
// 停于：未到达 / 未 settled / DLQ 进行中（dlqPending）/ 不可提交（commitable=false，策略 A 阻塞点）。
func advanceFrontier(inflight map[int64]*inflightEntry, frontier *int64) *sarama.ConsumerMessage {
	var last *sarama.ConsumerMessage
	for {
		fe, ok := inflight[*frontier]
		if !ok || !fe.settled || fe.dlqPending || !fe.commitable {
			break
		}
		last = fe.msg
		delete(inflight, *frontier)
		*frontier++
	}
	return last
}

// forwardCompletion 是每条消息的「无状态 bridge」：把该消息的 Done chan 搬进固定 compCh。
// ⭐ 决策 1-A：防止「done 已就绪 + ctx 同时触发」时 select 随机取 ctx.Done 而丢弃一个已完成的 completion。
//
//	做法：ctx 触发后再补一次非阻塞 drain——done 有值必捞起转发并置 canceled=true；done 空才放弃（留待重投递）。
//
// 调用契约：compCh 容量 >= 最大在飞数（构造期保证），故 compCh <- 永不阻塞。
func forwardCompletion(ctx context.Context, off int64, done <-chan error, compCh chan<- completion) {
	select {
	case err := <-done:
		// ⭐ 决策 1-A：done 与 ctx.Done 同时就绪时，外层 select 可能随机取到本分支。
		//   只要 ctx 已取消，就按「取消后补捞」语义标记 canceled=true（与 ctx.Done 分支一致）。
		canceled := false
		if ctx.Err() != nil {
			canceled = true
		}
		compCh <- completion{offset: off, err: err, canceled: canceled} // 正常完成（或竞态补捞）
	case <-ctx.Done():
		select { // ⭐ 非阻塞补捞：堵住「已知完成却被随机丢弃」的竞态窗口
		case err := <-done:
			compCh <- completion{offset: off, err: err, canceled: true} // ctx 取消后补捞 → canceled=true
		default:
			// done 仍空（handler 还在跑）→ 放弃转发，该消息从 frontier 重投递；同时防止 bridge 在 hung handler 上永久驻留
		}
	}
}

// sendDLQ 在独立于 session 的 ctx 上异步投递 DLQ，结果经 dlqDoneCh 回送（事件循环零网络 I/O）。
// ⭐ 用 context.Background() + dlqTimeout：rebalance/关闭不打断在飞 DLQ（设计 §4.3/T15），
//
//	避免「ctx 取消 → ok=false → 误告警毒消息」竞态。代价：DLQ goroutine 生命周期由 dlqTimeout 界定，
//	dlqTimeout × windowSize 上界保证有界、不泄漏。
//
// 若 p.dlq == nil（未配置 DLQ），直接回送 ok=false（→ 策略 A 阻塞 + 告警）。
func sendDLQ(p *partitionPipeline, off int64, msg *sarama.ConsumerMessage, dlqDoneCh chan<- dlqResult) {
	go func() {
		if p.dlq == nil {
			dlqDoneCh <- dlqResult{offset: off, ok: false}
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), p.cfg.DLQTimeout)
		defer cancel()
		ok := p.dlq.Send(ctx, msg) // 网络 I/O 在事件循环外，自带退避重试由实现负责
		dlqDoneCh <- dlqResult{offset: off, ok: ok}
	}()
}

// flush 在主循环 ctx.Done 后限时冲刷。同时 drain compCh 与 dlqDoneCh，提交可推进的连续前缀；
// 超时即返回，未 resolved 的留给 rebalance 重投递。冲刷期不新起 DLQ（Envelope 业务失败一律留待重投递）。
//
// ⭐ 决策 1-A / §4.6：据 completion.canceled 正向信号判定「取消」，不嗅探错误类型。
func flush(
	marker offsetMarker,
	inflight map[int64]*inflightEntry,
	frontier *int64,
	compCh <-chan completion,
	dlqDoneCh <-chan dlqResult,
	alert poisonAlerter,
	timeout time.Duration,
) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	unresolved := func() int { // 未 settled 或 DLQ 进行中
		n := 0
		for _, e := range inflight {
			if !e.settled || e.dlqPending {
				n++
			}
		}
		return n
	}
	commit := func() { // 推进连续前缀 + mark-once
		if last := advanceFrontier(inflight, frontier); last != nil {
			marker.MarkMessage(last, "")
		}
	}

drain:
	for unresolved() > 0 {
		select {
		case c := <-compCh:
			e := inflight[c.offset]
			if e == nil || e.settled {
				continue // nil 守卫 + 幂等
			}
			e.settled = true
			// ⭐ 区分 ctx 取消 vs 业务失败：用正向信号 c.canceled（不嗅探错误类型）；冲刷期 Envelope 失败不起 DLQ，留待重投递
			if c.canceled {
				e.commitable = false
			} else {
				switch decideCommitable(e, c.err) {
				case commitSuccess, commitRegularFail:
					e.commitable = true
				case commitEnvelopeFail:
					e.commitable = false // 下次 claim 再走正常异步 DLQ 路径
				}
			}
			commit()
		case r := <-dlqDoneCh: // 拆除前已起的 DLQ 在此收尾
			e := inflight[r.offset]
			if e == nil {
				continue
			}
			e.dlqPending = false
			e.commitable = r.ok
			if !r.ok {
				alert.AlertPoisonMessage(e.msg) // 策略 A 强告警
			}
			commit()
		case <-timer.C:
			break drain // 超时：未 resolved 的留待重投递
		}
	}
}

// run 是 ConsumeClaim 的核心事件循环。全部可变状态（inflight/frontier）只在本 goroutine 修改，无锁。
// 背压惯用法：窗口满则 msgCh=nil，select 只处理完成事件，腾位后再读。
// ⭐ A1：compCh/dlqDoneCh 由调用方（newPartitionPipeline）分配并传入——单一分配点 = 单一 buffer 守护点，
//
//	避免「守护的是被丢弃的 chan、真正在用的无人校验」。
func (p *partitionPipeline) run(ctx context.Context, messages <-chan *sarama.ConsumerMessage, marker offsetMarker, compCh chan completion, dlqDoneCh chan dlqResult) error {
	inflight := map[int64]*inflightEntry{}
	var frontier int64 = -1 // 哨兵；收到首条消息时置为首条 offset
	closed := false         // messages chan 已关闭（claim 结束）：停读新消息，继续冲刷在飞项直到 inflight 清空

	for {
		var msgCh <-chan *sarama.ConsumerMessage = messages
		if closed || len(inflight) >= p.cfg.WindowSize {
			msgCh = nil // 背压（窗口满）或 claim 已结束：停读新消息
		}
		if closed && len(inflight) == 0 {
			return nil // claim 结束且在飞已全部 settle/推进：安全退出
		}

		select {
		case msg := <-msgCh:
			if msg == nil {
				closed = true // claim 结束；不立即 return：继续 drain 在飞完成/DLQ，避免丢弃未结算消息
				continue
			}
			if frontier < 0 {
				frontier = msg.Offset
			}
			aggMsg := p.buildAggMsg(msg)
			inflight[msg.Offset] = &inflightEntry{msg: msg, isEnvelope: aggMsg.IsEnvelope}
			_ = p.pool.ProcessMessage(ctx, aggMsg) // 异步，不等 Done
			// bridge：把这条的 Done 搬进 compCh（决策 1-A 非阻塞 drain）
			go forwardCompletion(ctx, msg.Offset, aggMsg.Done, compCh)

		case c := <-compCh:
			e := inflight[c.offset]
			if e == nil {
				continue
			}
			e.settled = true
			if c.canceled {
				e.commitable = false // ctx 取消：留待重投递
			} else {
				switch decideCommitable(e, c.err) {
				case commitSuccess, commitRegularFail:
					e.commitable = true
				case commitEnvelopeFail:
					e.dlqPending = true // 暂不可推进；异步 DLQ
					sendDLQ(p, c.offset, e.msg, dlqDoneCh)
				}
			}
			if last := advanceFrontier(inflight, &frontier); last != nil {
				marker.MarkMessage(last, "") // mark-once
			}

		case r := <-dlqDoneCh:
			e := inflight[r.offset]
			if e == nil {
				continue
			}
			e.dlqPending = false
			e.commitable = r.ok
			if !r.ok {
				p.alert.AlertPoisonMessage(e.msg) // 策略 A：DLQ 失败强告警、不 re-submit
			}
			if last := advanceFrontier(inflight, &frontier); last != nil {
				marker.MarkMessage(last, "")
			}

		case <-ctx.Done():
			flush(marker, inflight, &frontier, compCh, dlqDoneCh, p.alert, p.cfg.FlushTimeout)
			return nil
		}
	}
}
