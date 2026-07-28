package reliable

// 指标名常量（§8.4/§10，M13 规范）。必须与 §10 告警表逐一对应。
const (
	MetricDeadLetterTotal         = "consumption_dead_letter_total"
	MetricRecordFailureTotal      = "consumption_record_failure_total"
	MetricAnomalyTotal            = "consumption_anomaly_total"
	MetricPartitionStalledSecs    = "consumption_partition_stalled_seconds"
	MetricPendingCount            = "consumption_pending_count"
	MetricReplayBlockedCount      = "consumption_replay_blocked_count"
	MetricOutboxDeadLetteredTotal = "outbox_dead_lettered_total"
)

// 标签常量（M13）。
const (
	LabelHandler   = "handler"
	LabelTenant    = "tenant"
	LabelErrClass  = "error_class"
	LabelKind      = "kind"
	LabelStatus    = "status"
	LabelTopic     = "topic"
	LabelPartition = "partition"
)

// ConsumptionMetrics 由服务用自有 prometheus registry 实现（core 只定义接口，J2 不引 prometheus）。
type ConsumptionMetrics interface {
	IncDeadLetter(handlerID HandlerID, class ErrorClass)
	IncRecordFailure(handlerID HandlerID)
	IncAnomaly(kind string, handlerID HandlerID)
	IncReplayBlocked(handlerID HandlerID)
	SetPending(status Status, handlerID HandlerID, tenantID int, n int64)
}

// NoOpMetrics 是零实现，供未接入指标的服务/测试使用（不得作为生产默认）。
type NoOpMetrics struct{}

func (NoOpMetrics) IncDeadLetter(HandlerID, ErrorClass)      {}
func (NoOpMetrics) IncRecordFailure(HandlerID)               {}
func (NoOpMetrics) IncAnomaly(string, HandlerID)             {}
func (NoOpMetrics) IncReplayBlocked(HandlerID)               {}
func (NoOpMetrics) SetPending(Status, HandlerID, int, int64) {}

// Alerter 把「需人工立刻看」的事件推给服务侧告警通道（§10 P1）。
type Alerter interface {
	AlertPoison(handlerID HandlerID, eventID string, cause error)
	AlertRecordFailure(handlerID HandlerID, cause error)
	AlertAnomaly(kind string, handlerID HandlerID, detail string)
}

// NoOpAlerter 零实现（测试用）。
type NoOpAlerter struct{}

func (NoOpAlerter) AlertPoison(HandlerID, string, error)   {}
func (NoOpAlerter) AlertRecordFailure(HandlerID, error)    {}
func (NoOpAlerter) AlertAnomaly(string, HandlerID, string) {}
