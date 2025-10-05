package eventbus

import "time"

// ========== Keyed-Worker 池默认配置 ==========

const (
	// DefaultKeyedWorkerCount 默认Worker数量
	// 1024个Worker可以提供良好的并发度，同时避免过多的goroutine开销
	// 适用于大多数场景，可以根据实际负载调整
	DefaultKeyedWorkerCount = 1024

	// DefaultKeyedQueueSize 默认每个Worker的队列大小
	// 1000个消息的队列可以缓冲短时间的流量突发
	// 避免队列过大导致内存溢出，同时提供足够的缓冲空间
	DefaultKeyedQueueSize = 1000

	// DefaultKeyedWaitTimeout 默认入队等待超时时间
	// 200ms的超时时间可以快速失败，避免长时间阻塞
	// 允许调用方及时感知背压并采取措施（如限流、降级）
	DefaultKeyedWaitTimeout = 200 * time.Millisecond
)

// ========== 健康检查默认配置 ==========

const (
	// DefaultHealthCheckInterval 默认健康检查间隔
	// 30秒的间隔可以及时发现问题，同时避免过于频繁的检查
	DefaultHealthCheckInterval = 30 * time.Second

	// DefaultHealthCheckTimeout 默认健康检查超时时间
	// 10秒的超时时间足够完成一次健康检查，同时避免长时间等待
	DefaultHealthCheckTimeout = 10 * time.Second

	// DefaultHealthCheckRetryMax 默认健康检查最大重试次数
	// 3次重试可以避免偶发性失败导致的误报
	DefaultHealthCheckRetryMax = 3

	// DefaultHealthCheckRetryInterval 默认健康检查重试间隔
	// 5秒的重试间隔给系统足够的恢复时间
	DefaultHealthCheckRetryInterval = 5 * time.Second
)

// ========== 积压检测默认配置 ==========

const (
	// DefaultBacklogCheckInterval 默认积压检测间隔
	// 30秒的间隔可以及时发现积压，同时避免过于频繁的检查
	DefaultBacklogCheckInterval = 30 * time.Second

	// DefaultBacklogLagThreshold 默认消息积压数量阈值
	// 1000条消息的积压阈值适用于大多数场景
	// 超过此阈值表示消费速度跟不上生产速度
	DefaultBacklogLagThreshold = 1000

	// DefaultBacklogTimeThreshold 默认消息积压时间阈值
	// 5分钟的时间阈值表示消息等待处理的最大时间
	// 超过此阈值表示系统可能存在性能问题
	DefaultBacklogTimeThreshold = 5 * time.Minute

	// DefaultBacklogRecoveryThreshold 默认积压恢复阈值
	// 100条消息的恢复阈值表示积压已基本消除
	// 低于此阈值表示系统恢复正常
	DefaultBacklogRecoveryThreshold = 100

	// DefaultPublisherBacklogMaxQueueDepth 默认发送端最大队列深度
	// 1000条消息的队列深度可以缓冲短时间的流量突发
	DefaultPublisherBacklogMaxQueueDepth = 1000

	// DefaultPublisherBacklogMaxLatency 默认发送端最大延迟
	// 5秒的最大延迟表示消息发送的可接受延迟
	// 超过此延迟表示发送端可能存在性能问题
	DefaultPublisherBacklogMaxLatency = 5 * time.Second

	// DefaultPublisherBacklogRateThreshold 默认发送端速率阈值
	// 500条/秒的速率阈值适用于大多数场景
	// 超过此速率表示发送端可能过载
	DefaultPublisherBacklogRateThreshold = 500.0
)

// ========== 重连配置 ==========

const (
	// DefaultMaxReconnectAttempts 默认最大重连次数
	// 10次重连尝试可以应对大多数临时性故障
	DefaultMaxReconnectAttempts = 10

	// DefaultReconnectInitialBackoff 默认初始重连退避时间
	// 1秒的初始退避时间避免立即重连导致的资源浪费
	DefaultReconnectInitialBackoff = 1 * time.Second

	// DefaultReconnectMaxBackoff 默认最大重连退避时间
	// 30秒的最大退避时间避免长时间等待
	DefaultReconnectMaxBackoff = 30 * time.Second

	// DefaultReconnectBackoffFactor 默认重连退避因子
	// 2.0的退避因子实现指数退避（1s, 2s, 4s, 8s, 16s, 30s）
	DefaultReconnectBackoffFactor = 2.0

	// DefaultReconnectFailureThreshold 默认触发重连的连续失败次数
	// 3次连续失败后触发重连，避免偶发性失败导致的频繁重连
	DefaultReconnectFailureThreshold = 3
)

// ========== Kafka 默认配置 ==========

const (
	// DefaultKafkaHealthCheckInterval 默认Kafka健康检查间隔
	// 5分钟的间隔适用于生产环境，避免过于频繁的检查
	DefaultKafkaHealthCheckInterval = 5 * time.Minute

	// DefaultKafkaProducerRequiredAcks 默认生产者确认级别
	// 1表示等待leader确认，平衡性能和可靠性
	DefaultKafkaProducerRequiredAcks = 1

	// DefaultKafkaProducerFlushFrequency 默认生产者刷新频率
	// 500ms的刷新频率可以批量发送消息，提高吞吐量
	DefaultKafkaProducerFlushFrequency = 500 * time.Millisecond

	// DefaultKafkaProducerFlushMessages 默认生产者刷新消息数
	// 100条消息的批量大小可以提高吞吐量，同时避免延迟过高
	DefaultKafkaProducerFlushMessages = 100

	// DefaultKafkaProducerRetryMax 默认生产者最大重试次数
	// 3次重试可以应对临时性故障
	DefaultKafkaProducerRetryMax = 3

	// DefaultKafkaProducerTimeout 默认生产者超时时间
	// 10秒的超时时间足够完成消息发送
	DefaultKafkaProducerTimeout = 10 * time.Second

	// DefaultKafkaProducerBatchSize 默认生产者批量大小（字节）
	// 16KB的批量大小可以提高吞吐量
	DefaultKafkaProducerBatchSize = 16384

	// DefaultKafkaProducerBufferSize 默认生产者缓冲区大小（字节）
	// 32KB的缓冲区大小可以缓冲多个批次
	DefaultKafkaProducerBufferSize = 32768

	// DefaultKafkaConsumerSessionTimeout 默认消费者会话超时时间
	// 30秒的会话超时时间适用于大多数场景
	DefaultKafkaConsumerSessionTimeout = 30 * time.Second

	// DefaultKafkaConsumerHeartbeatInterval 默认消费者心跳间隔
	// 3秒的心跳间隔可以及时检测消费者状态
	DefaultKafkaConsumerHeartbeatInterval = 3 * time.Second

	// DefaultKafkaConsumerMaxProcessingTime 默认消费者最大处理时间
	// 5分钟的最大处理时间适用于复杂的业务逻辑
	DefaultKafkaConsumerMaxProcessingTime = 5 * time.Minute

	// DefaultKafkaConsumerFetchMinBytes 默认消费者最小拉取字节数
	// 1字节表示立即返回，避免等待
	DefaultKafkaConsumerFetchMinBytes = 1

	// DefaultKafkaConsumerFetchMaxBytes 默认消费者最大拉取字节数
	// 1MB的最大拉取字节数可以批量拉取消息
	DefaultKafkaConsumerFetchMaxBytes = 1048576

	// DefaultKafkaConsumerFetchMaxWait 默认消费者最大等待时间
	// 500ms的最大等待时间可以及时拉取消息
	DefaultKafkaConsumerFetchMaxWait = 500 * time.Millisecond
)

// ========== NATS 默认配置 ==========

const (
	// DefaultNATSHealthCheckInterval 默认NATS健康检查间隔
	// 5分钟的间隔适用于生产环境
	DefaultNATSHealthCheckInterval = 5 * time.Minute

	// DefaultNATSMaxReconnects 默认NATS最大重连次数
	// 10次重连尝试可以应对大多数临时性故障
	DefaultNATSMaxReconnects = 10

	// DefaultNATSReconnectWait 默认NATS重连等待时间
	// 2秒的重连等待时间避免立即重连
	DefaultNATSReconnectWait = 2 * time.Second

	// DefaultNATSConnectionTimeout 默认NATS连接超时时间
	// 10秒的连接超时时间足够完成连接
	DefaultNATSConnectionTimeout = 10 * time.Second

	// DefaultNATSPublishTimeout 默认NATS发布超时时间
	// 5秒的发布超时时间足够完成消息发送
	DefaultNATSPublishTimeout = 5 * time.Second

	// DefaultNATSAckWait 默认NATS确认等待时间
	// 30秒的确认等待时间适用于大多数场景
	DefaultNATSAckWait = 30 * time.Second

	// DefaultNATSMaxDeliver 默认NATS最大投递次数
	// 3次投递尝试可以应对临时性故障
	DefaultNATSMaxDeliver = 3

	// DefaultNATSStreamMaxAge 默认NATS流最大保留时间
	// 24小时的保留时间适用于大多数业务场景
	DefaultNATSStreamMaxAge = 24 * time.Hour

	// DefaultNATSStreamMaxBytes 默认NATS流最大字节数
	// 100MB的最大字节数可以存储大量消息
	DefaultNATSStreamMaxBytes = 100 * 1024 * 1024

	// DefaultNATSStreamMaxMsgs 默认NATS流最大消息数
	// 10000条消息的最大数量适用于大多数场景
	DefaultNATSStreamMaxMsgs = 10000

	// DefaultNATSStreamReplicas 默认NATS流副本数
	// 1个副本适用于开发和测试环境，生产环境建议3个副本
	DefaultNATSStreamReplicas = 1

	// DefaultNATSConsumerMaxAckPending 默认NATS消费者最大未确认消息数
	// 100条未确认消息可以提高吞吐量
	DefaultNATSConsumerMaxAckPending = 100

	// DefaultNATSConsumerMaxWaiting 默认NATS消费者最大等待数
	// 500个等待请求可以支持高并发
	DefaultNATSConsumerMaxWaiting = 500
)

// ========== 指标收集默认配置 ==========

const (
	// DefaultMetricsCollectInterval 默认指标收集间隔
	// 30秒的收集间隔可以及时获取系统状态
	DefaultMetricsCollectInterval = 30 * time.Second
)

// ========== 流量控制默认配置 ==========

const (
	// DefaultRateLimit 默认速率限制（消息/秒）
	// 1000条/秒的速率限制适用于大多数场景
	DefaultRateLimit = 1000.0

	// DefaultRateBurst 默认突发流量限制
	// 2000条的突发限制允许短时间的流量突发
	DefaultRateBurst = 2000
)

// ========== 死信队列默认配置 ==========

const (
	// DefaultDeadLetterMaxRetries 默认死信队列最大重试次数
	// 3次重试可以应对临时性故障
	DefaultDeadLetterMaxRetries = 3
)

// ========== 其他默认配置 ==========

const (
	// DefaultTracingSampleRate 默认链路追踪采样率
	// 0.1表示10%的采样率，平衡性能和可观测性
	DefaultTracingSampleRate = 0.1

	// DefaultProcessTimeout 默认处理超时时间
	// 30秒的处理超时时间适用于大多数业务逻辑
	DefaultProcessTimeout = 30 * time.Second

	// DefaultMaxConcurrency 默认最大并发数
	// 10个并发可以提高吞吐量，同时避免资源耗尽
	DefaultMaxConcurrency = 10
)

