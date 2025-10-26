package eventbus

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	jxtjson "github.com/ChenBigdata421/jxt-core/sdk/pkg/json"
)

// 统一健康检查主题常量
const (
	// DefaultHealthCheckTopic jxt-core 统一管理的健康检查主题
	DefaultHealthCheckTopic = "jxt-core-health-check"

	// 按事件总线类型的健康检查主题
	KafkaHealthCheckTopic  = "jxt-core-kafka-health-check"
	NATSHealthCheckTopic   = "jxt-core-nats-health-check"
	MemoryHealthCheckTopic = "jxt-core-memory-health-check"
)

// HealthCheckMessage 标准化的健康检查消息
type HealthCheckMessage struct {
	MessageID    string            `json:"messageId"`
	Timestamp    time.Time         `json:"timestamp"`
	Source       string            `json:"source"`       // 微服务名称
	EventBusType string            `json:"eventBusType"` // kafka/nats/memory
	Version      string            `json:"version"`      // jxt-core版本
	Metadata     map[string]string `json:"metadata"`
}

// CreateHealthCheckMessage 创建标准健康检查消息
func CreateHealthCheckMessage(source, eventBusType string) *HealthCheckMessage {
	return &HealthCheckMessage{
		MessageID:    generateHealthCheckMessageID(),
		Timestamp:    time.Now(),
		Source:       source,
		EventBusType: eventBusType,
		Version:      GetJXTCoreVersion(),
		Metadata:     make(map[string]string),
	}
}

// ToBytes 序列化为字节数组
func (h *HealthCheckMessage) ToBytes() ([]byte, error) {
	// 确保 Metadata 不为 nil
	if h.Metadata == nil {
		h.Metadata = make(map[string]string)
	}
	return jxtjson.Marshal(h)
}

// FromBytes 从字节数组反序列化
func (h *HealthCheckMessage) FromBytes(data []byte) error {
	return jxtjson.Unmarshal(data, h)
}

// IsValid 验证消息是否有效
func (h *HealthCheckMessage) IsValid() bool {
	if h.MessageID == "" || h.Source == "" || h.EventBusType == "" {
		return false
	}

	// 检查时间戳是否在合理范围内（避免时钟偏移问题）
	now := time.Now()
	if h.Timestamp.After(now.Add(1*time.Minute)) ||
		h.Timestamp.Before(now.Add(-5*time.Minute)) {
		return false
	}

	return true
}

// IsExpired 检查消息是否过期
func (h *HealthCheckMessage) IsExpired(ttl time.Duration) bool {
	return time.Since(h.Timestamp) > ttl
}

// SetMetadata 设置元数据
func (h *HealthCheckMessage) SetMetadata(key, value string) {
	if h.Metadata == nil {
		h.Metadata = make(map[string]string)
	}
	h.Metadata[key] = value
}

// GetMetadata 获取元数据
func (h *HealthCheckMessage) GetMetadata(key string) (string, bool) {
	if h.Metadata == nil {
		return "", false
	}
	value, exists := h.Metadata[key]
	return value, exists
}

// generateHealthCheckMessageID 生成健康检查消息ID
func generateHealthCheckMessageID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return fmt.Sprintf("hc-%d-%s", time.Now().UnixNano(), hex.EncodeToString(bytes))
}

// GetJXTCoreVersion 获取 jxt-core 版本
func GetJXTCoreVersion() string {
	// 这里应该从构建信息获取，暂时使用硬编码
	return "2.0.0"
}

// GetHealthCheckTopic 根据事件总线类型获取健康检查主题
func GetHealthCheckTopic(eventBusType string) string {
	switch eventBusType {
	case "kafka":
		return KafkaHealthCheckTopic
	case "nats":
		return NATSHealthCheckTopic
	case "memory":
		return MemoryHealthCheckTopic
	default:
		return DefaultHealthCheckTopic
	}
}

// HealthCheckMessageValidator 健康检查消息验证器
type HealthCheckMessageValidator struct {
	MaxMessageAge  time.Duration // 最大消息年龄
	RequiredFields []string      // 必需字段
}

// NewHealthCheckMessageValidator 创建健康检查消息验证器
func NewHealthCheckMessageValidator() *HealthCheckMessageValidator {
	return &HealthCheckMessageValidator{
		MaxMessageAge:  DefaultBacklogTimeThreshold, // 使用默认积压时间阈值
		RequiredFields: []string{"messageId", "timestamp", "source", "eventBusType", "version"},
	}
}

// Validate 验证健康检查消息
func (v *HealthCheckMessageValidator) Validate(msg *HealthCheckMessage) error {
	if msg == nil {
		return fmt.Errorf("health check message cannot be nil")
	}

	// 验证必需字段
	if msg.MessageID == "" {
		return fmt.Errorf("messageId is required")
	}
	if msg.Source == "" {
		return fmt.Errorf("source is required")
	}
	if msg.EventBusType == "" {
		return fmt.Errorf("eventBusType is required")
	}
	if msg.Version == "" {
		return fmt.Errorf("version is required")
	}

	// 验证时间戳
	if msg.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	// 验证消息年龄
	if msg.IsExpired(v.MaxMessageAge) {
		return fmt.Errorf("message is too old: %v", time.Since(msg.Timestamp))
	}

	// 验证事件总线类型
	validTypes := []string{"kafka", "nats", "memory"}
	isValidType := false
	for _, validType := range validTypes {
		if msg.EventBusType == validType {
			isValidType = true
			break
		}
	}
	if !isValidType {
		return fmt.Errorf("invalid eventBusType: %s", msg.EventBusType)
	}

	return nil
}

// HealthCheckMessageBuilder 健康检查消息构建器
type HealthCheckMessageBuilder struct {
	message *HealthCheckMessage
}

// NewHealthCheckMessageBuilder 创建健康检查消息构建器
func NewHealthCheckMessageBuilder(source, eventBusType string) *HealthCheckMessageBuilder {
	return &HealthCheckMessageBuilder{
		message: CreateHealthCheckMessage(source, eventBusType),
	}
}

// WithMetadata 添加元数据
func (b *HealthCheckMessageBuilder) WithMetadata(key, value string) *HealthCheckMessageBuilder {
	b.message.SetMetadata(key, value)
	return b
}

// WithCheckType 设置检查类型
func (b *HealthCheckMessageBuilder) WithCheckType(checkType string) *HealthCheckMessageBuilder {
	b.message.SetMetadata("checkType", checkType)
	return b
}

// WithInstanceID 设置实例ID
func (b *HealthCheckMessageBuilder) WithInstanceID(instanceID string) *HealthCheckMessageBuilder {
	b.message.SetMetadata("instanceId", instanceID)
	return b
}

// WithEnvironment 设置环境
func (b *HealthCheckMessageBuilder) WithEnvironment(env string) *HealthCheckMessageBuilder {
	b.message.SetMetadata("environment", env)
	return b
}

// Build 构建消息
func (b *HealthCheckMessageBuilder) Build() *HealthCheckMessage {
	return b.message
}

// HealthCheckMessageParser 健康检查消息解析器
type HealthCheckMessageParser struct {
	validator *HealthCheckMessageValidator
}

// NewHealthCheckMessageParser 创建健康检查消息解析器
func NewHealthCheckMessageParser() *HealthCheckMessageParser {
	return &HealthCheckMessageParser{
		validator: NewHealthCheckMessageValidator(),
	}
}

// Parse 解析健康检查消息
func (p *HealthCheckMessageParser) Parse(data []byte) (*HealthCheckMessage, error) {
	var msg HealthCheckMessage
	if err := jxtjson.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal health check message: %w", err)
	}

	if err := p.validator.Validate(&msg); err != nil {
		return nil, fmt.Errorf("health check message validation failed: %w", err)
	}

	return &msg, nil
}

// ParseWithoutValidation 解析健康检查消息（不验证）
func (p *HealthCheckMessageParser) ParseWithoutValidation(data []byte) (*HealthCheckMessage, error) {
	var msg HealthCheckMessage
	if err := jxtjson.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal health check message: %w", err)
	}

	return &msg, nil
}
