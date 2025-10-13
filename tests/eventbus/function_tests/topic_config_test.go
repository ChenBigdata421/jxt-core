package function_tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// TestKafkaTopicConfiguration 测试 Kafka 主题配置
func TestKafkaTopicConfiguration(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.topic.config.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-topic-config-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 配置主题
	options := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		Replicas:        1,
		RetentionTime:   24 * time.Hour,
		MaxMessages:     10000,
	}

	err := bus.ConfigureTopic(ctx, topic, options)
	helper.AssertNoError(err, "ConfigureTopic should not return error")

	// 获取主题配置
	config, err := bus.GetTopicConfig(topic)
	helper.AssertNoError(err, "GetTopicConfig should not return error")
	helper.AssertEqual(eventbus.TopicPersistent, config.PersistenceMode, "PersistenceMode should match")

	// 列出已配置的主题
	topics := bus.ListConfiguredTopics()
	found := false
	for _, t := range topics {
		if t == topic {
			found = true
			break
		}
	}
	helper.AssertTrue(found, "Topic should be in configured topics list")

	t.Logf("✅ Kafka topic configuration test passed")
}

// TestNATSTopicConfiguration 测试 NATS 主题配置
func TestNATSTopicConfiguration(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.topic.config.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-topic-config-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_TOPIC_CONFIG_%d", helper.GetTimestamp()/1000))

	ctx := context.Background()

	// 配置主题
	options := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
		Replicas:        1,
		RetentionTime:   24 * time.Hour,
		MaxMessages:     10000,
	}

	err := bus.ConfigureTopic(ctx, topic, options)
	helper.AssertNoError(err, "ConfigureTopic should not return error")

	// 获取主题配置
	config, err := bus.GetTopicConfig(topic)
	helper.AssertNoError(err, "GetTopicConfig should not return error")
	helper.AssertEqual(eventbus.TopicPersistent, config.PersistenceMode, "PersistenceMode should match")

	// 列出已配置的主题
	topics := bus.ListConfiguredTopics()
	found := false
	for _, t := range topics {
		if t == topic {
			found = true
			break
		}
	}
	helper.AssertTrue(found, "Topic should be in configured topics list")

	t.Logf("✅ NATS topic configuration test passed")
}

// TestKafkaSetTopicPersistence 测试 Kafka 设置主题持久化
func TestKafkaSetTopicPersistence(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.persistence.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-persistence-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 设置持久化
	err := bus.SetTopicPersistence(ctx, topic, true)
	helper.AssertNoError(err, "SetTopicPersistence should not return error")

	// 验证配置
	config, err := bus.GetTopicConfig(topic)
	helper.AssertNoError(err, "GetTopicConfig should not return error")
	helper.AssertEqual(eventbus.TopicPersistent, config.PersistenceMode, "Should be persistent")

	t.Logf("✅ Kafka SetTopicPersistence test passed")
}

// TestNATSSetTopicPersistence 测试 NATS 设置主题持久化
func TestNATSSetTopicPersistence(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.persistence.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-persistence-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_PERSISTENCE_%d", helper.GetTimestamp()/1000))

	ctx := context.Background()

	// 设置持久化
	err := bus.SetTopicPersistence(ctx, topic, true)
	helper.AssertNoError(err, "SetTopicPersistence should not return error")

	// 验证配置
	config, err := bus.GetTopicConfig(topic)
	helper.AssertNoError(err, "GetTopicConfig should not return error")
	helper.AssertEqual(eventbus.TopicPersistent, config.PersistenceMode, "Should be persistent")

	t.Logf("✅ NATS SetTopicPersistence test passed")
}

// TestKafkaRemoveTopicConfig 测试 Kafka 移除主题配置
func TestKafkaRemoveTopicConfig(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.kafka.remove.config.%d", helper.GetTimestamp())
	helper.CreateKafkaTopics([]string{topic}, 3)

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-remove-config-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	ctx := context.Background()

	// 配置主题
	options := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
	}
	err := bus.ConfigureTopic(ctx, topic, options)
	helper.AssertNoError(err, "ConfigureTopic should not return error")

	// 移除配置
	err = bus.RemoveTopicConfig(topic)
	helper.AssertNoError(err, "RemoveTopicConfig should not return error")

	// 验证配置已移除
	topics := bus.ListConfiguredTopics()
	found := false
	for _, t := range topics {
		if t == topic {
			found = true
			break
		}
	}
	helper.AssertTrue(!found, "Topic should not be in configured topics list after removal")

	t.Logf("✅ Kafka RemoveTopicConfig test passed")
}

// TestNATSRemoveTopicConfig 测试 NATS 移除主题配置
func TestNATSRemoveTopicConfig(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	topic := fmt.Sprintf("test.nats.remove.config.%d", helper.GetTimestamp())
	clientID := fmt.Sprintf("nats-remove-config-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	defer helper.CleanupNATSStreams(fmt.Sprintf("TEST_NATS_REMOVE_CONFIG_%d", helper.GetTimestamp()/1000))

	ctx := context.Background()

	// 配置主题
	options := eventbus.TopicOptions{
		PersistenceMode: eventbus.TopicPersistent,
	}
	err := bus.ConfigureTopic(ctx, topic, options)
	helper.AssertNoError(err, "ConfigureTopic should not return error")

	// 移除配置
	err = bus.RemoveTopicConfig(topic)
	helper.AssertNoError(err, "RemoveTopicConfig should not return error")

	// 验证配置已移除
	topics := bus.ListConfiguredTopics()
	found := false
	for _, t := range topics {
		if t == topic {
			found = true
			break
		}
	}
	helper.AssertTrue(!found, "Topic should not be in configured topics list after removal")

	t.Logf("✅ NATS RemoveTopicConfig test passed")
}

// TestKafkaTopicConfigStrategy 测试 Kafka 主题配置策略
func TestKafkaTopicConfigStrategy(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	bus := helper.CreateKafkaEventBus(fmt.Sprintf("kafka-config-strategy-%d", helper.GetTimestamp()))
	defer helper.CloseEventBus(bus)

	// 设置策略
	bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)

	// 获取策略
	strategy := bus.GetTopicConfigStrategy()
	helper.AssertEqual(eventbus.StrategyCreateOrUpdate, strategy, "Strategy should match")

	// 设置其他策略
	bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)
	strategy = bus.GetTopicConfigStrategy()
	helper.AssertEqual(eventbus.StrategyValidateOnly, strategy, "Strategy should match")

	t.Logf("✅ Kafka topic config strategy test passed")
}

// TestNATSTopicConfigStrategy 测试 NATS 主题配置策略
func TestNATSTopicConfigStrategy(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	clientID := fmt.Sprintf("nats-config-strategy-%d", helper.GetTimestamp())
	bus := helper.CreateNATSEventBus(clientID)
	defer helper.CloseEventBus(bus)

	// 设置策略
	bus.SetTopicConfigStrategy(eventbus.StrategyCreateOrUpdate)

	// 获取策略
	strategy := bus.GetTopicConfigStrategy()
	helper.AssertEqual(eventbus.StrategyCreateOrUpdate, strategy, "Strategy should match")

	// 设置其他策略
	bus.SetTopicConfigStrategy(eventbus.StrategyValidateOnly)
	strategy = bus.GetTopicConfigStrategy()
	helper.AssertEqual(eventbus.StrategyValidateOnly, strategy, "Strategy should match")

	t.Logf("✅ NATS topic config strategy test passed")
}
