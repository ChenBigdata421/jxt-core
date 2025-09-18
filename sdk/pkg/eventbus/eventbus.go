package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
)

// eventBusManager 事件总线管理器
type eventBusManager struct {
	config            *EventBusConfig
	publisher         Publisher
	subscriber        Subscriber
	metrics           *Metrics
	healthStatus      *HealthStatus
	reconnectCallback func(ctx context.Context) error
	mu                sync.RWMutex
	closed            bool
}

// NewEventBus 创建新的事件总线实例
func NewEventBus(config *EventBusConfig) (EventBus, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	manager := &eventBusManager{
		config: config,
		metrics: &Metrics{
			LastHealthCheck:   time.Now(),
			HealthCheckStatus: "unknown",
		},
		healthStatus: &HealthStatus{
			Status:    "initializing",
			LastCheck: time.Now(),
			Metrics:   Metrics{},
			Details:   make(map[string]interface{}),
		},
	}

	// 根据配置类型创建具体实现
	switch config.Type {
	case "kafka":
		return manager.initKafka()
	case "nats":
		return manager.initNATS()
	case "memory":
		return manager.initMemory()
	default:
		return nil, fmt.Errorf("unsupported eventbus type: %s", config.Type)
	}
}

// Publish 发布消息
func (m *eventBusManager) Publish(ctx context.Context, topic string, message []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("eventbus is closed")
	}

	if m.publisher == nil {
		return fmt.Errorf("publisher not initialized")
	}

	// 更新指标
	start := time.Now()
	err := m.publisher.Publish(ctx, topic, message)

	m.updateMetrics(err == nil, true, time.Since(start))

	if err != nil {
		logger.Error("Failed to publish message", "topic", topic, "error", err)
		return fmt.Errorf("failed to publish message to topic %s: %w", topic, err)
	}

	logger.Debug("Message published successfully", "topic", topic, "size", len(message))
	return nil
}

// Subscribe 订阅消息
func (m *eventBusManager) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("eventbus is closed")
	}

	if m.subscriber == nil {
		return fmt.Errorf("subscriber not initialized")
	}

	// 包装处理器以更新指标
	wrappedHandler := func(ctx context.Context, message []byte) error {
		start := time.Now()
		err := handler(ctx, message)
		m.updateMetrics(err == nil, false, time.Since(start))

		if err != nil {
			logger.Error("Message handler failed", "topic", topic, "error", err)
		} else {
			logger.Debug("Message processed successfully", "topic", topic, "size", len(message))
		}

		return err
	}

	err := m.subscriber.Subscribe(ctx, topic, wrappedHandler)
	if err != nil {
		logger.Error("Failed to subscribe to topic", "topic", topic, "error", err)
		return fmt.Errorf("failed to subscribe to topic %s: %w", topic, err)
	}

	logger.Info("Successfully subscribed to topic", "topic", topic)
	return nil
}

// HealthCheck 健康检查
func (m *eventBusManager) HealthCheck(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("eventbus is closed")
	}

	// 更新健康检查时间
	m.metrics.LastHealthCheck = time.Now()
	m.healthStatus.LastCheck = time.Now()

	// 执行具体的健康检查逻辑
	var errors []string

	// 检查发布器
	if m.publisher != nil {
		// 这里可以添加具体的发布器健康检查逻辑
		// 例如：发送测试消息到健康检查topic
	}

	// 检查订阅器
	if m.subscriber != nil {
		// 这里可以添加具体的订阅器健康检查逻辑
	}

	// 更新健康状态
	if len(errors) == 0 {
		m.healthStatus.Status = "healthy"
		m.metrics.HealthCheckStatus = "healthy"
	} else {
		m.healthStatus.Status = "unhealthy"
		m.metrics.HealthCheckStatus = "unhealthy"
		m.healthStatus.Errors = errors
		return fmt.Errorf("health check failed: %v", errors)
	}

	m.healthStatus.Metrics = *m.metrics
	logger.Debug("Health check completed", "status", m.healthStatus.Status)
	return nil
}

// Close 关闭连接
func (m *eventBusManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	var errors []error

	// 关闭发布器
	if m.publisher != nil {
		if err := m.publisher.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close publisher: %w", err))
		}
	}

	// 关闭订阅器
	if m.subscriber != nil {
		if err := m.subscriber.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close subscriber: %w", err))
		}
	}

	m.closed = true

	if len(errors) > 0 {
		return fmt.Errorf("errors during close: %v", errors)
	}

	logger.Info("EventBus closed successfully")
	return nil
}

// RegisterReconnectCallback 注册重连回调
func (m *eventBusManager) RegisterReconnectCallback(callback func(ctx context.Context) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reconnectCallback = callback
	logger.Info("Reconnect callback registered")
	return nil
}

// GetMetrics 获取指标
func (m *eventBusManager) GetMetrics() Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.metrics
}

// GetHealthStatus 获取健康状态
func (m *eventBusManager) GetHealthStatus() HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.healthStatus
}

// updateMetrics 更新指标
func (m *eventBusManager) updateMetrics(success bool, isPublish bool, duration time.Duration) {
	if isPublish {
		if success {
			m.metrics.MessagesPublished++
		} else {
			m.metrics.PublishErrors++
		}
	} else {
		if success {
			m.metrics.MessagesConsumed++
		} else {
			m.metrics.ConsumeErrors++
		}
	}
}

// initKafka 初始化Kafka事件总线
func (m *eventBusManager) initKafka() (EventBus, error) {
	// 转换配置类型
	kafkaConfig := &config.KafkaConfig{
		Brokers:             m.config.Kafka.Brokers,
		HealthCheckInterval: m.config.Kafka.HealthCheckInterval,
		Producer: config.ProducerConfig{
			RequiredAcks:    m.config.Kafka.Producer.RequiredAcks,
			Timeout:         m.config.Kafka.Producer.Timeout,
			Compression:     m.config.Kafka.Producer.Compression,
			MaxMessageBytes: 1000000, // 默认值
			FlushFrequency:  m.config.Kafka.Producer.FlushFrequency,
			FlushMessages:   m.config.Kafka.Producer.FlushMessages,
			FlushBytes:      1000000, // 默认值
			RetryMax:        m.config.Kafka.Producer.RetryMax,
			BatchSize:       m.config.Kafka.Producer.BatchSize,
			BufferSize:      m.config.Kafka.Producer.BufferSize,
			Idempotent:      false,  // 默认值
			PartitionerType: "hash", // 默认值
		},
		Consumer: config.ConsumerConfig{
			GroupID:            m.config.Kafka.Consumer.GroupID,
			AutoOffsetReset:    m.config.Kafka.Consumer.AutoOffsetReset,
			SessionTimeout:     m.config.Kafka.Consumer.SessionTimeout,
			HeartbeatInterval:  m.config.Kafka.Consumer.HeartbeatInterval,
			MaxProcessingTime:  m.config.Kafka.Consumer.MaxProcessingTime,
			FetchMinBytes:      m.config.Kafka.Consumer.FetchMinBytes,
			FetchMaxBytes:      m.config.Kafka.Consumer.FetchMaxBytes,
			FetchMaxWait:       m.config.Kafka.Consumer.FetchMaxWait,
			RebalanceStrategy:  "range",            // 默认值
			IsolationLevel:     "read_uncommitted", // 默认值
			MaxPollRecords:     500,                // 默认值
			EnableAutoCommit:   true,               // 默认值
			AutoCommitInterval: 1 * time.Second,    // 默认值
		},
		Security: config.SecurityConfig{
			Enabled:  m.config.Kafka.Security.Enabled,
			Protocol: m.config.Kafka.Security.Protocol,
			CertFile: m.config.Kafka.Security.CertFile,
			KeyFile:  m.config.Kafka.Security.KeyFile,
			CAFile:   m.config.Kafka.Security.CAFile,
			Username: m.config.Kafka.Security.Username,
			Password: m.config.Kafka.Security.Password,
		},
	}

	return NewKafkaEventBus(kafkaConfig)
}

// initNATS 初始化NATS事件总线
func (m *eventBusManager) initNATS() (EventBus, error) {
	// 转换配置类型
	natsConfig := &config.NATSConfig{
		URLs:                m.config.NATS.URLs,
		ClientID:            m.config.NATS.ClientID,
		MaxReconnects:       m.config.NATS.MaxReconnects,
		ReconnectWait:       m.config.NATS.ReconnectWait,
		ConnectionTimeout:   m.config.NATS.ConnectionTimeout,
		HealthCheckInterval: m.config.NATS.HealthCheckInterval,
		JetStream:           convertToConfigJetStream(m.config.NATS.JetStream),
		Security:            convertToConfigSecurity(m.config.NATS.Security),
	}

	return NewNATSEventBus(natsConfig)
}

// convertToConfigJetStream 转换JetStream配置到config包类型
func convertToConfigJetStream(cfg JetStreamConfig) config.JetStreamConfig {
	return config.JetStreamConfig{
		Enabled:        cfg.Enabled,
		Domain:         cfg.Domain,
		APIPrefix:      cfg.APIPrefix,
		PublishTimeout: cfg.PublishTimeout,
		AckWait:        cfg.AckWait,
		MaxDeliver:     cfg.MaxDeliver,
		Stream:         convertToConfigStream(cfg.Stream),
		Consumer:       convertToConfigConsumer(cfg.Consumer),
	}
}

// convertToConfigStream 转换流配置到config包类型
func convertToConfigStream(cfg StreamConfig) config.StreamConfig {
	return config.StreamConfig{
		Name:      cfg.Name,
		Subjects:  cfg.Subjects,
		Retention: cfg.Retention,
		Storage:   cfg.Storage,
		Replicas:  cfg.Replicas,
		MaxAge:    cfg.MaxAge,
		MaxBytes:  cfg.MaxBytes,
		MaxMsgs:   cfg.MaxMsgs,
		Discard:   cfg.Discard,
	}
}

// convertToConfigConsumer 转换消费者配置到config包类型
func convertToConfigConsumer(cfg NATSConsumerConfig) config.NATSConsumerConfig {
	return config.NATSConsumerConfig{
		DurableName:   cfg.DurableName,
		DeliverPolicy: cfg.DeliverPolicy,
		AckPolicy:     cfg.AckPolicy,
		ReplayPolicy:  cfg.ReplayPolicy,
		MaxAckPending: cfg.MaxAckPending,
		MaxWaiting:    cfg.MaxWaiting,
		MaxDeliver:    cfg.MaxDeliver,
		BackOff:       cfg.BackOff,
	}
}

// convertToConfigSecurity 转换安全配置到config包类型
func convertToConfigSecurity(cfg NATSSecurityConfig) config.NATSSecurityConfig {
	return config.NATSSecurityConfig{
		Enabled:    cfg.Enabled,
		Token:      cfg.Token,
		Username:   cfg.Username,
		Password:   cfg.Password,
		NKeyFile:   cfg.NKeyFile,
		CredFile:   cfg.CredFile,
		CertFile:   cfg.CertFile,
		KeyFile:    cfg.KeyFile,
		CAFile:     cfg.CAFile,
		SkipVerify: cfg.SkipVerify,
	}
}
