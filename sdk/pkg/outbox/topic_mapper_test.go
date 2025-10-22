package outbox

import "testing"

func TestMapBasedTopicMapper(t *testing.T) {
	mapper := NewMapBasedTopicMapper(map[string]string{
		"Archive": "archive-events",
		"Media":   "media-events",
		"User":    "user-events",
	}, "default-events")

	tests := []struct {
		aggregateType string
		expectedTopic string
	}{
		{"Archive", "archive-events"},
		{"Media", "media-events"},
		{"User", "user-events"},
		{"Unknown", "default-events"},
	}

	for _, tt := range tests {
		t.Run(tt.aggregateType, func(t *testing.T) {
			topic := mapper.GetTopic(tt.aggregateType)
			if topic != tt.expectedTopic {
				t.Errorf("Expected topic '%s', got '%s'", tt.expectedTopic, topic)
			}
		})
	}
}

func TestMapBasedTopicMapper_NoDefault(t *testing.T) {
	mapper := NewMapBasedTopicMapper(map[string]string{
		"Archive": "archive-events",
	}, "")

	// 未映射的类型应该返回 "{aggregateType}-events"
	topic := mapper.GetTopic("Unknown")
	expected := "Unknown-events"
	if topic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, topic)
	}
}

func TestPrefixTopicMapper(t *testing.T) {
	mapper := NewPrefixTopicMapper("jxt", "events", ".")

	tests := []struct {
		aggregateType string
		expectedTopic string
	}{
		{"Archive", "jxt.Archive.events"},
		{"Media", "jxt.Media.events"},
		{"User", "jxt.User.events"},
	}

	for _, tt := range tests {
		t.Run(tt.aggregateType, func(t *testing.T) {
			topic := mapper.GetTopic(tt.aggregateType)
			if topic != tt.expectedTopic {
				t.Errorf("Expected topic '%s', got '%s'", tt.expectedTopic, topic)
			}
		})
	}
}

func TestPrefixTopicMapper_NoPrefix(t *testing.T) {
	mapper := NewPrefixTopicMapper("", "events", ".")

	topic := mapper.GetTopic("Archive")
	expected := "Archive.events"
	if topic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, topic)
	}
}

func TestPrefixTopicMapper_NoSuffix(t *testing.T) {
	mapper := NewPrefixTopicMapper("jxt", "", ".")

	topic := mapper.GetTopic("Archive")
	expected := "jxt.Archive"
	if topic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, topic)
	}
}

func TestPrefixTopicMapper_NoSeparator(t *testing.T) {
	mapper := NewPrefixTopicMapper("jxt", "events", "")

	topic := mapper.GetTopic("Archive")
	expected := "jxtArchiveevents"
	if topic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, topic)
	}
}

func TestFuncTopicMapper(t *testing.T) {
	mapper := FuncTopicMapper(func(aggregateType string) string {
		return "custom-" + aggregateType
	})

	topic := mapper.GetTopic("Archive")
	expected := "custom-Archive"
	if topic != expected {
		t.Errorf("Expected topic '%s', got '%s'", expected, topic)
	}
}

func TestStaticTopicMapper(t *testing.T) {
	mapper := NewStaticTopicMapper("all-events")

	tests := []string{"Archive", "Media", "User"}
	for _, aggregateType := range tests {
		t.Run(aggregateType, func(t *testing.T) {
			topic := mapper.GetTopic(aggregateType)
			if topic != "all-events" {
				t.Errorf("Expected topic 'all-events', got '%s'", topic)
			}
		})
	}
}

func TestChainTopicMapper(t *testing.T) {
	// 创建一个只返回空字符串的 mapper（用于测试链式）
	emptyMapper := FuncTopicMapper(func(aggregateType string) string {
		if aggregateType == "Archive" {
			return "archive-events"
		}
		return "" // 返回空字符串，让链继续
	})

	mapper2 := FuncTopicMapper(func(aggregateType string) string {
		if aggregateType == "Media" {
			return "media-events"
		}
		return ""
	})

	mapper3 := NewStaticTopicMapper("default-events")

	chain := NewChainTopicMapper(emptyMapper, mapper2, mapper3)

	tests := []struct {
		aggregateType string
		expectedTopic string
	}{
		{"Archive", "archive-events"}, // 从 mapper1
		{"Media", "media-events"},     // 从 mapper2
		{"User", "default-events"},    // 从 mapper3
	}

	for _, tt := range tests {
		t.Run(tt.aggregateType, func(t *testing.T) {
			topic := chain.GetTopic(tt.aggregateType)
			if topic != tt.expectedTopic {
				t.Errorf("Expected topic '%s', got '%s'", tt.expectedTopic, topic)
			}
		})
	}
}

func TestDefaultTopicMapper(t *testing.T) {
	tests := []struct {
		aggregateType string
		expectedTopic string
	}{
		{"Archive", "Archive-events"},
		{"Media", "Media-events"},
		{"User", "User-events"},
	}

	for _, tt := range tests {
		t.Run(tt.aggregateType, func(t *testing.T) {
			topic := DefaultTopicMapper.GetTopic(tt.aggregateType)
			if topic != tt.expectedTopic {
				t.Errorf("Expected topic '%s', got '%s'", tt.expectedTopic, topic)
			}
		})
	}
}
