package eventbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/IBM/sarama"
	"go.uber.org/zap"
)

// kafkaEventBus Kafka事件总线实现
// 企业级增强版本，集成积压检测、流量控制、聚合处理等特性
type kafkaEventBus struct {
	config   *config.KafkaConfig
	producer sarama.SyncProducer
	consumer sarama.Consumer
	client   sarama.Client
	admin    sarama.ClusterAdmin
	logger   *zap.Logger
	mu       sync.RWMutex
	closed   bool

	// 企业级特性
	backlogDetector           *BacklogDetector
	rateLimiter               *RateLimiter
	aggregateProcessorManager *AggregateProcessorManager
	recoveryMode              atomic.Bool

	// 统计信息
	publishedMessages atomic.Int64
	consumedMessages  atomic.Int64
	errorCount        atomic.Int64
}

// NewKafkaEventBus 创建企业级Kafka事件总线
func NewKafkaEventBus(cfg *config.KafkaConfig) (EventBus, error) {
	if cfg == nil {
		return nil, fmt.Errorf("kafka config cannot be nil")
	}

	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers cannot be empty")
	}

	// 创建Sarama配置
	saramaConfig := sarama.NewConfig()
	if err := configureSarama(saramaConfig, cfg); err != nil {
		return nil, fmt.Errorf("failed to configure sarama: %w", err)
	}

	// 创建客户端
	client, err := sarama.NewClient(cfg.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka client: %w", err)
	}

	// 创建生产者
	producer, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create kafka producer: %w", err)
	}

	// 创建消费者
	consumer, err := sarama.NewConsumerFromClient(client)
	if err != nil {
		producer.Close()
		client.Close()
		return nil, fmt.Errorf("failed to create kafka consumer: %w", err)
	}

	// 创建管理客户端
	admin, err := sarama.NewClusterAdminFromClient(client)
	if err != nil {
		consumer.Close()
		producer.Close()
		client.Close()
		return nil, fmt.Errorf("failed to create kafka admin: %w", err)
	}

	// 创建EventBus实例
	eventBus := &kafkaEventBus{
		config:   cfg,
		producer: producer,
		consumer: consumer,
		client:   client,
		admin:    admin,
		logger:   logger.Logger,
	}

	// 初始化企业级特性
	if err := eventBus.initEnterpriseFeatures(); err != nil {
		eventBus.Close()
		return nil, fmt.Errorf("failed to initialize enterprise features: %w", err)
	}

	return eventBus, nil
}

// configureSarama 配置Sarama
func configureSarama(config *sarama.Config, cfg *config.KafkaConfig) error {
	// 生产者配置
	config.Producer.RequiredAcks = sarama.RequiredAcks(cfg.Producer.RequiredAcks)
	config.Producer.Timeout = cfg.Producer.Timeout
	config.Producer.Retry.Max = cfg.Producer.RetryMax
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true

	// 设置压缩算法
	switch cfg.Producer.Compression {
	case "gzip":
		config.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		config.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		config.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		config.Producer.Compression = sarama.CompressionZSTD
	default:
		config.Producer.Compression = sarama.CompressionNone
	}

	// 幂等性配置
	config.Producer.Idempotent = cfg.Producer.Idempotent
	if cfg.Producer.Idempotent {
		config.Producer.RequiredAcks = sarama.WaitForAll
		config.Producer.Retry.Max = 5
		config.Net.MaxOpenRequests = 1
	}

	// 批处理配置
	if cfg.Producer.FlushFrequency > 0 {
		config.Producer.Flush.Frequency = cfg.Producer.FlushFrequency
	}
	if cfg.Producer.FlushMessages > 0 {
		config.Producer.Flush.Messages = cfg.Producer.FlushMessages
	}
	if cfg.Producer.FlushBytes > 0 {
		config.Producer.Flush.Bytes = cfg.Producer.FlushBytes
	}

	// 消费者配置
	config.Consumer.Group.Session.Timeout = cfg.Consumer.SessionTimeout
	config.Consumer.Group.Heartbeat.Interval = cfg.Consumer.HeartbeatInterval
	config.Consumer.MaxProcessingTime = cfg.Consumer.MaxProcessingTime
	config.Consumer.Fetch.Min = int32(cfg.Consumer.FetchMinBytes)
	config.Consumer.Fetch.Max = int32(cfg.Consumer.FetchMaxBytes)
	config.Consumer.MaxWaitTime = cfg.Consumer.FetchMaxWait

	// 设置偏移量重置策略
	switch cfg.Consumer.AutoOffsetReset {
	case "earliest":
		config.Consumer.Offsets.Initial = sarama.OffsetOldest
	case "latest":
		config.Consumer.Offsets.Initial = sarama.OffsetNewest
	default:
		config.Consumer.Offsets.Initial = sarama.OffsetNewest
	}

	// 网络配置
	if cfg.Net.DialTimeout > 0 {
		config.Net.DialTimeout = cfg.Net.DialTimeout
	}
	if cfg.Net.ReadTimeout > 0 {
		config.Net.ReadTimeout = cfg.Net.ReadTimeout
	}
	if cfg.Net.WriteTimeout > 0 {
		config.Net.WriteTimeout = cfg.Net.WriteTimeout
	}

	// 版本配置
	config.Version = sarama.V2_6_0_0

	return nil
}

// kafkaConsumerHandler Kafka消费者处理器
type kafkaConsumerHandler struct {
	eventBus *kafkaEventBus
	handler  MessageHandler
	topic    string
}

// Setup 消费者组设置
func (h *kafkaConsumerHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

// Cleanup 消费者组清理
func (h *kafkaConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 消费消息
func (h *kafkaConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			// 处理消息
			if err := h.processMessage(session.Context(), message); err != nil {
				h.eventBus.logger.Error("Failed to process message",
					zap.String("topic", message.Topic),
					zap.Int32("partition", message.Partition),
					zap.Int64("offset", message.Offset),
					zap.Error(err))
				h.eventBus.errorCount.Add(1)
			} else {
				h.eventBus.consumedMessages.Add(1)
				session.MarkMessage(message, "")
			}

		case <-session.Context().Done():
			return nil
		}
	}
}

// processMessage 处理单个消息
func (h *kafkaConsumerHandler) processMessage(ctx context.Context, message *sarama.ConsumerMessage) error {
	// 流量控制
	if h.eventBus.rateLimiter != nil {
		if err := h.eventBus.rateLimiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit error: %w", err)
		}
	}

	// 检查是否需要聚合处理
	if h.eventBus.aggregateProcessorManager != nil {
		// 提取聚合ID（这里简化为使用消息key）
		aggregateID := string(message.Key)
		if aggregateID != "" {
			// 创建聚合消息
			aggMsg := &AggregateMessage{
				Topic:       message.Topic,
				Partition:   message.Partition,
				Offset:      message.Offset,
				Key:         message.Key,
				Value:       message.Value,
				Headers:     make(map[string][]byte),
				Timestamp:   message.Timestamp,
				AggregateID: aggregateID,
				Context:     ctx,
				Done:        make(chan error, 1),
			}

			// 复制headers
			for _, header := range message.Headers {
				aggMsg.Headers[string(header.Key)] = header.Value
			}

			// 发送到聚合处理器
			if err := h.eventBus.aggregateProcessorManager.ProcessMessage(ctx, aggMsg); err != nil {
				return err
			}

			// 等待处理结果
			select {
			case err := <-aggMsg.Done:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// 直接处理消息
	return h.handler(ctx, message.Value)
}

// initEnterpriseFeatures 初始化企业级特性
func (k *kafkaEventBus) initEnterpriseFeatures() error {
	// 初始化积压检测器
	if k.config.BacklogDetection.Enabled {
		k.backlogDetector = NewBacklogDetector(
			k.client,
			k.admin,
			k.config.Consumer.GroupID,
			BacklogDetectionConfig{
				MaxLagThreshold:  k.config.BacklogDetection.MaxLagThreshold,
				MaxTimeThreshold: k.config.BacklogDetection.MaxTimeThreshold,
				CheckInterval:    k.config.BacklogDetection.CheckInterval,
			},
		)
	}

	// 初始化流量控制器
	if k.config.RateLimit.Enabled {
		k.rateLimiter = NewRateLimiter(RateLimitConfig{
			Enabled:       k.config.RateLimit.Enabled,
			RatePerSecond: k.config.RateLimit.RatePerSecond,
			BurstSize:     k.config.RateLimit.BurstSize,
		})
	}

	// 初始化聚合处理器管理器
	// 注意：聚合处理器管理器将在Subscribe时按需创建
	// 这里只是预留初始化位置

	k.logger.Info("Enterprise features initialized successfully")
	return nil
}

// Kafka EventBus 实现

// Publish 发布消息
func (k *kafkaEventBus) Publish(ctx context.Context, topic string, message []byte) error {
	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.closed {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 流量控制
	if k.rateLimiter != nil {
		if err := k.rateLimiter.Wait(ctx); err != nil {
			k.errorCount.Add(1)
			return fmt.Errorf("rate limit error: %w", err)
		}
	}

	// 创建Kafka消息
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(message),
	}

	// 发送消息
	partition, offset, err := k.producer.SendMessage(msg)
	if err != nil {
		k.errorCount.Add(1)
		k.logger.Error("Failed to publish message",
			zap.String("topic", topic),
			zap.Error(err))
		return fmt.Errorf("failed to publish message: %w", err)
	}

	k.publishedMessages.Add(1)
	k.logger.Debug("Message published successfully",
		zap.String("topic", topic),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset))

	return nil
}

// Subscribe 订阅消息
func (k *kafkaEventBus) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	k.mu.RLock()
	defer k.mu.RUnlock()

	if k.closed {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 创建消费者组
	consumerGroup, err := sarama.NewConsumerGroupFromClient(k.config.Consumer.GroupID, k.client)
	if err != nil {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	// 创建消费者处理器
	consumerHandler := &kafkaConsumerHandler{
		eventBus: k,
		handler:  handler,
		topic:    topic,
	}

	// 启动消费循环
	go func() {
		defer consumerGroup.Close()

		for {
			select {
			case <-ctx.Done():
				k.logger.Info("Consumer context cancelled", zap.String("topic", topic))
				return
			default:
				if err := consumerGroup.Consume(ctx, []string{topic}, consumerHandler); err != nil {
					k.logger.Error("Consumer group consume error",
						zap.String("topic", topic),
						zap.Error(err))
					time.Sleep(time.Second) // 避免快速重试
				}
			}
		}
	}()

	k.logger.Info("Subscribed to topic", zap.String("topic", topic), zap.String("groupID", k.config.Consumer.GroupID))
	return nil
}

// HealthCheck 健康检查
func (k *kafkaEventBus) HealthCheck(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return fmt.Errorf("kafka eventbus is closed")
	}

	// 检查客户端连接
	if !k.client.Closed() {
		// 尝试获取broker信息
		brokers := k.client.Brokers()
		if len(brokers) == 0 {
			return fmt.Errorf("no available brokers")
		}

		// 检查至少一个broker是否可达
		for _, broker := range brokers {
			if connected, _ := broker.Connected(); connected {
				k.logger.Debug("Kafka eventbus health check passed",
					zap.Int("brokers", len(brokers)),
					zap.String("connectedBroker", broker.Addr()))
				return nil
			}
		}

		return fmt.Errorf("no connected brokers available")
	}

	return fmt.Errorf("kafka client is closed")
}

// Close 关闭连接
func (k *kafkaEventBus) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.closed {
		return nil
	}

	var errors []error

	// 关闭聚合处理器管理器
	if k.aggregateProcessorManager != nil {
		k.aggregateProcessorManager.Stop()
	}

	// 关闭管理客户端
	if k.admin != nil {
		if err := k.admin.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka admin: %w", err))
		}
	}

	// 关闭消费者
	if k.consumer != nil {
		if err := k.consumer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka consumer: %w", err))
		}
	}

	// 关闭生产者
	if k.producer != nil {
		if err := k.producer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka producer: %w", err))
		}
	}

	// 关闭客户端
	if k.client != nil {
		if err := k.client.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close kafka client: %w", err))
		}
	}

	k.closed = true
	k.logger.Info("Kafka eventbus closed successfully")

	if len(errors) > 0 {
		return fmt.Errorf("errors during kafka eventbus close: %v", errors)
	}

	return nil
}

// RegisterReconnectCallback 注册重连回调
func (k *kafkaEventBus) RegisterReconnectCallback(callback func(ctx context.Context) error) error {
	// Kafka EventBus 不需要重连回调，因为sarama会自动处理重连
	k.logger.Info("Kafka eventbus does not require reconnect callback")
	return nil
}
