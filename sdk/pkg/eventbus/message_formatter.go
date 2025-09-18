package eventbus

import (
	"fmt"
	"strconv"
	"time"
)

// DefaultMessageFormatter 默认消息格式化器
type DefaultMessageFormatter struct{}

// FormatMessage 格式化消息
func (f *DefaultMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
	msg := NewMessage(uuid, payload)
	
	// 提取聚合ID
	aggID := f.ExtractAggregateID(aggregateID)
	if aggID != "" {
		msg.Metadata["aggregateID"] = aggID
	}
	
	// 添加时间戳
	msg.Metadata["timestamp"] = time.Now().Format(time.RFC3339)
	
	return msg, nil
}

// ExtractAggregateID 提取聚合ID
func (f *DefaultMessageFormatter) ExtractAggregateID(aggregateID interface{}) string {
	if aggregateID == nil {
		return ""
	}
	
	switch id := aggregateID.(type) {
	case string:
		return id
	case int64:
		return strconv.FormatInt(id, 10)
	case int:
		return strconv.Itoa(id)
	case int32:
		return strconv.FormatInt(int64(id), 10)
	case uint64:
		return strconv.FormatUint(id, 10)
	case uint32:
		return strconv.FormatUint(uint64(id), 10)
	case uint:
		return strconv.FormatUint(uint64(id), 10)
	default:
		return fmt.Sprintf("%v", id)
	}
}

// SetMetadata 设置元数据
func (f *DefaultMessageFormatter) SetMetadata(msg *Message, metadata map[string]string) error {
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]string)
	}
	
	for k, v := range metadata {
		msg.Metadata[k] = v
	}
	
	return nil
}

// EvidenceMessageFormatter 业务特定的消息格式化器（用于 evidence-management）
type EvidenceMessageFormatter struct {
	DefaultMessageFormatter
}

// FormatMessage 格式化消息（evidence-management 特定）
func (f *EvidenceMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
	msg, err := f.DefaultMessageFormatter.FormatMessage(uuid, aggregateID, payload)
	if err != nil {
		return nil, err
	}
	
	// 业务特定的元数据字段名（evidence-management 使用 aggregate_id）
	if aggID := f.ExtractAggregateID(aggregateID); aggID != "" {
		msg.Metadata["aggregate_id"] = aggID // 保持与现有代码兼容
		// 同时保留标准字段名以便兼容
		msg.Metadata["aggregateID"] = aggID
	}
	
	return msg, nil
}

// JSONMessageFormatter JSON格式的消息格式化器
type JSONMessageFormatter struct {
	DefaultMessageFormatter
	IncludeHeaders bool // 是否包含头部信息
}

// FormatMessage 格式化为JSON消息
func (f *JSONMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
	msg, err := f.DefaultMessageFormatter.FormatMessage(uuid, aggregateID, payload)
	if err != nil {
		return nil, err
	}
	
	// 添加JSON特定的元数据
	msg.Metadata["contentType"] = "application/json"
	msg.Metadata["encoding"] = "utf-8"
	
	if f.IncludeHeaders {
		msg.Metadata["messageFormat"] = "json"
		msg.Metadata["schemaVersion"] = "1.0"
	}
	
	return msg, nil
}

// ProtobufMessageFormatter Protobuf格式的消息格式化器
type ProtobufMessageFormatter struct {
	DefaultMessageFormatter
	SchemaRegistry string // Schema注册中心地址
}

// FormatMessage 格式化为Protobuf消息
func (f *ProtobufMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
	msg, err := f.DefaultMessageFormatter.FormatMessage(uuid, aggregateID, payload)
	if err != nil {
		return nil, err
	}
	
	// 添加Protobuf特定的元数据
	msg.Metadata["contentType"] = "application/x-protobuf"
	msg.Metadata["messageFormat"] = "protobuf"
	
	if f.SchemaRegistry != "" {
		msg.Metadata["schemaRegistry"] = f.SchemaRegistry
	}
	
	return msg, nil
}

// AvroMessageFormatter Avro格式的消息格式化器
type AvroMessageFormatter struct {
	DefaultMessageFormatter
	SchemaID       string // Schema ID
	SchemaRegistry string // Schema注册中心地址
}

// FormatMessage 格式化为Avro消息
func (f *AvroMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
	msg, err := f.DefaultMessageFormatter.FormatMessage(uuid, aggregateID, payload)
	if err != nil {
		return nil, err
	}
	
	// 添加Avro特定的元数据
	msg.Metadata["contentType"] = "application/avro"
	msg.Metadata["messageFormat"] = "avro"
	
	if f.SchemaID != "" {
		msg.Metadata["schemaId"] = f.SchemaID
	}
	
	if f.SchemaRegistry != "" {
		msg.Metadata["schemaRegistry"] = f.SchemaRegistry
	}
	
	return msg, nil
}

// CloudEventMessageFormatter CloudEvents格式的消息格式化器
type CloudEventMessageFormatter struct {
	DefaultMessageFormatter
	Source      string // 事件源
	EventType   string // 事件类型
	SpecVersion string // CloudEvents规范版本
}

// FormatMessage 格式化为CloudEvents消息
func (f *CloudEventMessageFormatter) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
	msg, err := f.DefaultMessageFormatter.FormatMessage(uuid, aggregateID, payload)
	if err != nil {
		return nil, err
	}
	
	// 添加CloudEvents特定的元数据
	msg.Metadata["ce-specversion"] = f.getSpecVersion()
	msg.Metadata["ce-type"] = f.getEventType()
	msg.Metadata["ce-source"] = f.getSource()
	msg.Metadata["ce-id"] = uuid
	msg.Metadata["ce-time"] = time.Now().Format(time.RFC3339)
	msg.Metadata["content-type"] = "application/json"
	
	if aggID := f.ExtractAggregateID(aggregateID); aggID != "" {
		msg.Metadata["ce-subject"] = aggID
	}
	
	return msg, nil
}

func (f *CloudEventMessageFormatter) getSpecVersion() string {
	if f.SpecVersion != "" {
		return f.SpecVersion
	}
	return "1.0"
}

func (f *CloudEventMessageFormatter) getEventType() string {
	if f.EventType != "" {
		return f.EventType
	}
	return "com.jxt.event"
}

func (f *CloudEventMessageFormatter) getSource() string {
	if f.Source != "" {
		return f.Source
	}
	return "jxt-core"
}

// MessageFormatterChain 消息格式化器链
type MessageFormatterChain struct {
	formatters []MessageFormatter
}

// NewMessageFormatterChain 创建消息格式化器链
func NewMessageFormatterChain(formatters ...MessageFormatter) *MessageFormatterChain {
	return &MessageFormatterChain{
		formatters: formatters,
	}
}

// FormatMessage 链式格式化消息
func (c *MessageFormatterChain) FormatMessage(uuid string, aggregateID interface{}, payload []byte) (*Message, error) {
	if len(c.formatters) == 0 {
		return NewMessage(uuid, payload), nil
	}
	
	// 使用第一个格式化器创建基础消息
	msg, err := c.formatters[0].FormatMessage(uuid, aggregateID, payload)
	if err != nil {
		return nil, err
	}
	
	// 依次应用其他格式化器的元数据
	for i := 1; i < len(c.formatters); i++ {
		tempMsg, err := c.formatters[i].FormatMessage(uuid, aggregateID, payload)
		if err != nil {
			return nil, err
		}
		
		// 合并元数据
		for k, v := range tempMsg.Metadata {
			msg.Metadata[k] = v
		}
	}
	
	return msg, nil
}

// ExtractAggregateID 提取聚合ID
func (c *MessageFormatterChain) ExtractAggregateID(aggregateID interface{}) string {
	if len(c.formatters) == 0 {
		return ""
	}
	return c.formatters[0].ExtractAggregateID(aggregateID)
}

// SetMetadata 设置元数据
func (c *MessageFormatterChain) SetMetadata(msg *Message, metadata map[string]string) error {
	if len(c.formatters) == 0 {
		return fmt.Errorf("no formatters in chain")
	}
	return c.formatters[0].SetMetadata(msg, metadata)
}

// MessageFormatterRegistry 消息格式化器注册表
type MessageFormatterRegistry struct {
	formatters map[string]MessageFormatter
}

// NewMessageFormatterRegistry 创建消息格式化器注册表
func NewMessageFormatterRegistry() *MessageFormatterRegistry {
	registry := &MessageFormatterRegistry{
		formatters: make(map[string]MessageFormatter),
	}
	
	// 注册默认格式化器
	registry.Register("default", &DefaultMessageFormatter{})
	registry.Register("evidence", &EvidenceMessageFormatter{})
	registry.Register("json", &JSONMessageFormatter{IncludeHeaders: true})
	registry.Register("protobuf", &ProtobufMessageFormatter{})
	registry.Register("avro", &AvroMessageFormatter{})
	registry.Register("cloudevents", &CloudEventMessageFormatter{})
	
	return registry
}

// Register 注册格式化器
func (r *MessageFormatterRegistry) Register(name string, formatter MessageFormatter) {
	r.formatters[name] = formatter
}

// Get 获取格式化器
func (r *MessageFormatterRegistry) Get(name string) (MessageFormatter, bool) {
	formatter, exists := r.formatters[name]
	return formatter, exists
}

// GetOrDefault 获取格式化器或返回默认格式化器
func (r *MessageFormatterRegistry) GetOrDefault(name string) MessageFormatter {
	if formatter, exists := r.formatters[name]; exists {
		return formatter
	}
	return &DefaultMessageFormatter{}
}

// List 列出所有注册的格式化器
func (r *MessageFormatterRegistry) List() []string {
	names := make([]string, 0, len(r.formatters))
	for name := range r.formatters {
		names = append(names, name)
	}
	return names
}

// 全局格式化器注册表
var GlobalFormatterRegistry = NewMessageFormatterRegistry()
