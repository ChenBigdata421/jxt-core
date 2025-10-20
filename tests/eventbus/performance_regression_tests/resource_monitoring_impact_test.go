package performance_tests

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/stretchr/testify/require"
)

// TestResourceMonitoringImpact 测试资源监控对性能的影响
func TestResourceMonitoringImpact(t *testing.T) {
	logger.Setup()

	messageCount := 5000
	topicCount := 5

	t.Run("Kafka-WithMonitoring", func(t *testing.T) {
		testKafkaWithMonitoring(t, messageCount, topicCount, true)
	})

	t.Run("Kafka-WithoutMonitoring", func(t *testing.T) {
		testKafkaWithMonitoring(t, messageCount, topicCount, false)
	})

	t.Run("NATS-WithMonitoring", func(t *testing.T) {
		testNATSWithMonitoring(t, messageCount, topicCount, true)
	})

	t.Run("NATS-WithoutMonitoring", func(t *testing.T) {
		testNATSWithMonitoring(t, messageCount, topicCount, false)
	})
}

func testKafkaWithMonitoring(t *testing.T, messageCount int, topicCount int, enableMonitoring bool) {
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("test.monitor.kafka.topic%d", i+1)
	}

	cfg := &eventbus.KafkaConfig{
		Brokers:  []string{"localhost:29094"},
		ClientID: "monitor-kafka",
		Producer: eventbus.ProducerConfig{
			RequiredAcks:   1,
			FlushFrequency: 10 * time.Millisecond,
			FlushMessages:  100,
			Timeout:        30 * time.Second,
			// 注意：压缩配置已从 Producer 级别移到 Topic 级别，通过 TopicBuilder 配置
			FlushBytes:      1048576,
			RetryMax:        3,
			BatchSize:       16384,
			BufferSize:      8388608,
			MaxMessageBytes: 1048576,
		},
		Consumer: eventbus.ConsumerConfig{
			GroupID:            "monitor-kafka-group",
			SessionTimeout:     10 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  30 * time.Second,
			AutoOffsetReset:    "earliest",
			EnableAutoCommit:   true,
			AutoCommitInterval: 1 * time.Second,
			FetchMaxWait:       100 * time.Millisecond,
			MaxPollRecords:     500,
		},
		Net: eventbus.NetConfig{
			DialTimeout:  10 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
	}

	bus, err := eventbus.NewKafkaEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	var received int64

	for _, topic := range topics {
		topicName := topic
		err = bus.Subscribe(ctx, topicName, func(ctx context.Context, message []byte) error {
			atomic.AddInt64(&received, 1)
			return nil
		})
		require.NoError(t, err)
	}

	time.Sleep(2 * time.Second)

	// 发送消息
	startTime := time.Now()
	var sendWg sync.WaitGroup
	messagesPerTopic := messageCount / topicCount
	var peakGoroutines, peakMemory int64

	for topicIdx, topic := range topics {
		sendWg.Add(1)
		go func(topicName string, idx int) {
			defer sendWg.Done()

			for i := 0; i < messagesPerTopic; i++ {
				message := []byte(fmt.Sprintf("Message %d from topic %d", i, idx))
				_ = bus.Publish(ctx, topicName, message)

				if enableMonitoring {
					// 每条消息都监控资源
					currentGoroutines := int64(runtime.NumGoroutine())
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					currentMemory := int64(m.Alloc / 1024 / 1024)

					if currentGoroutines > atomic.LoadInt64(&peakGoroutines) {
						atomic.StoreInt64(&peakGoroutines, currentGoroutines)
					}
					if currentMemory > atomic.LoadInt64(&peakMemory) {
						atomic.StoreInt64(&peakMemory, currentMemory)
					}
				}
			}
		}(topic, topicIdx)
	}

	sendWg.Wait()
	duration := time.Since(startTime)
	throughput := float64(messageCount) / duration.Seconds()

	monitoringStatus := "无监控"
	if enableMonitoring {
		monitoringStatus = "有监控"
	}

	t.Logf("📊 Kafka %s: 耗时=%.2fs, 吞吐量=%.2f msg/s, 峰值协程=%d, 峰值内存=%dMB",
		monitoringStatus, duration.Seconds(), throughput, peakGoroutines, peakMemory)
}

func testNATSWithMonitoring(t *testing.T, messageCount int, topicCount int, enableMonitoring bool) {
	topics := make([]string, topicCount)
	for i := 0; i < topicCount; i++ {
		topics[i] = fmt.Sprintf("test.monitor.nats.topic%d", i+1)
	}

	cfg := &eventbus.NATSConfig{
		URLs:     []string{"nats://localhost:4223"},
		ClientID: "monitor-nats",
		JetStream: eventbus.JetStreamConfig{
			Enabled:        true,
			PublishTimeout: 30 * time.Second,
			AckWait:        30 * time.Second,
			MaxDeliver:     3,
			Stream: eventbus.StreamConfig{
				Name:      "TEST_MONITOR_NATS",
				Subjects:  []string{"test.monitor.nats.>"},
				Storage:   "memory",
				Retention: "limits",
				MaxAge:    24 * time.Hour,
				MaxBytes:  1024 * 1024 * 1024,
				MaxMsgs:   1000000,
				Replicas:  1,
				Discard:   "old",
			},
			Consumer: eventbus.NATSConsumerConfig{
				DurableName:   "monitor-nats-consumer",
				DeliverPolicy: "all",
				AckPolicy:     "explicit",
				ReplayPolicy:  "instant",
				MaxAckPending: 10000,
				MaxWaiting:    512,
				MaxDeliver:    3,
			},
		},
	}

	bus, err := eventbus.NewNATSEventBus(cfg)
	require.NoError(t, err)
	defer bus.Close()

	ctx := context.Background()
	var received int64

	for _, topic := range topics {
		topicName := topic
		err = bus.Subscribe(ctx, topicName, func(ctx context.Context, message []byte) error {
			atomic.AddInt64(&received, 1)
			return nil
		})
		require.NoError(t, err)
	}

	time.Sleep(2 * time.Second)

	// 发送消息
	startTime := time.Now()
	var sendWg sync.WaitGroup
	messagesPerTopic := messageCount / topicCount
	var peakGoroutines, peakMemory int64

	for topicIdx, topic := range topics {
		sendWg.Add(1)
		go func(topicName string, idx int) {
			defer sendWg.Done()

			for i := 0; i < messagesPerTopic; i++ {
				message := []byte(fmt.Sprintf("Message %d from topic %d", i, idx))
				_ = bus.Publish(ctx, topicName, message)

				if enableMonitoring {
					// 每条消息都监控资源
					currentGoroutines := int64(runtime.NumGoroutine())
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					currentMemory := int64(m.Alloc / 1024 / 1024)

					if currentGoroutines > atomic.LoadInt64(&peakGoroutines) {
						atomic.StoreInt64(&peakGoroutines, currentGoroutines)
					}
					if currentMemory > atomic.LoadInt64(&peakMemory) {
						atomic.StoreInt64(&peakMemory, currentMemory)
					}
				}
			}
		}(topic, topicIdx)
	}

	sendWg.Wait()
	duration := time.Since(startTime)
	throughput := float64(messageCount) / duration.Seconds()

	monitoringStatus := "无监控"
	if enableMonitoring {
		monitoringStatus = "有监控"
	}

	t.Logf("📊 NATS %s: 耗时=%.2fs, 吞吐量=%.2f msg/s, 峰值协程=%d, 峰值内存=%dMB",
		monitoringStatus, duration.Seconds(), throughput, peakGoroutines, peakMemory)
}
