package outbox

import "fmt"

// TopicMapper Topic 映射器接口
// 将聚合类型映射到 EventBus Topic
// 由业务微服务实现，定义业务特定的映射规则
type TopicMapper interface {
	// GetTopic 根据聚合类型获取 Topic
	//
	// 参数：
	//   aggregateType: 聚合类型（例如："Archive", "Media", "User"）
	//
	// 返回：
	//   string: Topic 名称（例如："archive-events", "media-events"）
	//
	// 注意：
	//   - 如果聚合类型未映射，应该返回默认 Topic 或错误
	//   - Topic 名称应该符合 EventBus 的命名规范
	GetTopic(aggregateType string) string
}

// MapBasedTopicMapper 基于 Map 的 TopicMapper 实现
// 提供简单的映射表实现
type MapBasedTopicMapper struct {
	// mapping 聚合类型到 Topic 的映射表
	mapping map[string]string

	// defaultTopic 默认 Topic（当聚合类型未映射时使用）
	defaultTopic string
}

// NewMapBasedTopicMapper 创建基于 Map 的 TopicMapper
//
// 参数：
//   mapping: 聚合类型到 Topic 的映射表
//   defaultTopic: 默认 Topic（可选，空字符串表示无默认值）
//
// 示例：
//
//	mapper := NewMapBasedTopicMapper(map[string]string{
//	    "Archive": "archive-events",
//	    "Media":   "media-events",
//	    "User":    "user-events",
//	}, "default-events")
func NewMapBasedTopicMapper(mapping map[string]string, defaultTopic string) TopicMapper {
	return &MapBasedTopicMapper{
		mapping:      mapping,
		defaultTopic: defaultTopic,
	}
}

// GetTopic 实现 TopicMapper 接口
func (m *MapBasedTopicMapper) GetTopic(aggregateType string) string {
	if topic, ok := m.mapping[aggregateType]; ok {
		return topic
	}
	// 如果未找到映射，返回默认 Topic
	if m.defaultTopic != "" {
		return m.defaultTopic
	}
	// 如果没有默认 Topic，返回聚合类型本身（转换为小写）
	return fmt.Sprintf("%s-events", aggregateType)
}

// PrefixTopicMapper 基于前缀的 TopicMapper
// 自动为聚合类型添加前缀和后缀
type PrefixTopicMapper struct {
	// prefix Topic 前缀（例如："jxt"）
	prefix string

	// suffix Topic 后缀（例如："events"）
	suffix string

	// separator 分隔符（例如："."）
	separator string
}

// NewPrefixTopicMapper 创建基于前缀的 TopicMapper
//
// 参数：
//   prefix: Topic 前缀
//   suffix: Topic 后缀
//   separator: 分隔符
//
// 示例：
//
//	mapper := NewPrefixTopicMapper("jxt", "events", ".")
//	// Archive -> jxt.archive.events
//	// Media -> jxt.media.events
func NewPrefixTopicMapper(prefix, suffix, separator string) TopicMapper {
	return &PrefixTopicMapper{
		prefix:    prefix,
		suffix:    suffix,
		separator: separator,
	}
}

// GetTopic 实现 TopicMapper 接口
func (p *PrefixTopicMapper) GetTopic(aggregateType string) string {
	parts := []string{}
	if p.prefix != "" {
		parts = append(parts, p.prefix)
	}
	parts = append(parts, aggregateType)
	if p.suffix != "" {
		parts = append(parts, p.suffix)
	}

	result := ""
	for i, part := range parts {
		if i > 0 && p.separator != "" {
			result += p.separator
		}
		result += part
	}
	return result
}

// FuncTopicMapper 函数式 TopicMapper
// 允许使用函数作为 TopicMapper 实现
type FuncTopicMapper func(aggregateType string) string

// GetTopic 实现 TopicMapper 接口
func (f FuncTopicMapper) GetTopic(aggregateType string) string {
	return f(aggregateType)
}

// StaticTopicMapper 静态 TopicMapper
// 所有聚合类型都映射到同一个 Topic
type StaticTopicMapper struct {
	topic string
}

// NewStaticTopicMapper 创建静态 TopicMapper
//
// 参数：
//   topic: 固定的 Topic 名称
//
// 示例：
//
//	mapper := NewStaticTopicMapper("all-events")
//	// 所有聚合类型都映射到 "all-events"
func NewStaticTopicMapper(topic string) TopicMapper {
	return &StaticTopicMapper{topic: topic}
}

// GetTopic 实现 TopicMapper 接口
func (s *StaticTopicMapper) GetTopic(aggregateType string) string {
	return s.topic
}

// ChainTopicMapper 链式 TopicMapper
// 依次尝试多个 TopicMapper，直到找到非空 Topic
type ChainTopicMapper struct {
	mappers []TopicMapper
}

// NewChainTopicMapper 创建链式 TopicMapper
//
// 参数：
//   mappers: TopicMapper 列表
//
// 示例：
//
//	mapper := NewChainTopicMapper(
//	    customMapper,
//	    defaultMapper,
//	)
func NewChainTopicMapper(mappers ...TopicMapper) TopicMapper {
	return &ChainTopicMapper{mappers: mappers}
}

// GetTopic 实现 TopicMapper 接口
func (c *ChainTopicMapper) GetTopic(aggregateType string) string {
	for _, mapper := range c.mappers {
		if topic := mapper.GetTopic(aggregateType); topic != "" {
			return topic
		}
	}
	return ""
}

// DefaultTopicMapper 默认 TopicMapper
// 使用简单的命名规则：{aggregateType}-events
var DefaultTopicMapper = FuncTopicMapper(func(aggregateType string) string {
	return fmt.Sprintf("%s-events", aggregateType)
})

