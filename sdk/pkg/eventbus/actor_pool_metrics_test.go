package eventbus

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoOpActorPoolMetricsCollector 测试 NoOp 收集器
func TestNoOpActorPoolMetricsCollector(t *testing.T) {
	collector := &NoOpActorPoolMetricsCollector{}
	require.NotNil(t, collector)

	// 所有方法都应该不panic
	collector.RecordMessageSent("actor-1")
	collector.RecordMessageProcessed("actor-1", true, 100*time.Millisecond)
	collector.RecordInboxDepth("actor-1", 10, 1000)
	collector.RecordActorRestarted("actor-1")
	collector.RecordDeadLetter("actor-1")
}

// TestPrometheusActorPoolMetricsCollector_MessageSent 测试消息发送指标
func TestPrometheusActorPoolMetricsCollector_MessageSent(t *testing.T) {
	collector := NewPrometheusActorPoolMetricsCollector("test_message_sent")

	// 记录消息发送
	collector.RecordMessageSent("actor-1")
	collector.RecordMessageSent("actor-1")
	collector.RecordMessageSent("actor-2")

	// 验证指标
	count := testutil.ToFloat64(collector.messagesSent.WithLabelValues("actor-1"))
	assert.Equal(t, float64(2), count)

	count = testutil.ToFloat64(collector.messagesSent.WithLabelValues("actor-2"))
	assert.Equal(t, float64(1), count)
}

// TestPrometheusActorPoolMetricsCollector_MessageProcessed 测试消息处理指标
func TestPrometheusActorPoolMetricsCollector_MessageProcessed(t *testing.T) {
	collector := NewPrometheusActorPoolMetricsCollector("test_message_processed")

	// 记录成功处理
	collector.RecordMessageProcessed("actor-1", true, 100*time.Millisecond)
	collector.RecordMessageProcessed("actor-1", true, 200*time.Millisecond)

	// 记录失败处理
	collector.RecordMessageProcessed("actor-1", false, 50*time.Millisecond)

	// 验证成功计数
	successCount := testutil.ToFloat64(collector.messagesProcessed.WithLabelValues("actor-1", "true"))
	assert.Equal(t, float64(2), successCount)

	// 验证失败计数
	failureCount := testutil.ToFloat64(collector.messagesProcessed.WithLabelValues("actor-1", "false"))
	assert.Equal(t, float64(1), failureCount)
}

// TestPrometheusActorPoolMetricsCollector_InboxDepth 测试Inbox深度指标
func TestPrometheusActorPoolMetricsCollector_InboxDepth(t *testing.T) {
	collector := NewPrometheusActorPoolMetricsCollector("test_inbox_depth")

	// 记录Inbox深度
	collector.RecordInboxDepth("actor-1", 100, 1000)

	// 验证深度
	depth := testutil.ToFloat64(collector.inboxDepth.WithLabelValues("actor-1"))
	assert.Equal(t, float64(100), depth)

	// 验证利用率
	utilization := testutil.ToFloat64(collector.inboxUtilization.WithLabelValues("actor-1"))
	assert.Equal(t, 0.1, utilization) // 100/1000 = 0.1

	// 更新深度
	collector.RecordInboxDepth("actor-1", 500, 1000)
	depth = testutil.ToFloat64(collector.inboxDepth.WithLabelValues("actor-1"))
	assert.Equal(t, float64(500), depth)

	utilization = testutil.ToFloat64(collector.inboxUtilization.WithLabelValues("actor-1"))
	assert.Equal(t, 0.5, utilization) // 500/1000 = 0.5
}

// TestPrometheusActorPoolMetricsCollector_ActorRestarted 测试Actor重启指标
func TestPrometheusActorPoolMetricsCollector_ActorRestarted(t *testing.T) {
	collector := NewPrometheusActorPoolMetricsCollector("test_actor_restarted")

	// 记录重启
	collector.RecordActorRestarted("actor-1")
	collector.RecordActorRestarted("actor-1")
	collector.RecordActorRestarted("actor-2")

	// 验证指标
	count := testutil.ToFloat64(collector.actorRestarted.WithLabelValues("actor-1"))
	assert.Equal(t, float64(2), count)

	count = testutil.ToFloat64(collector.actorRestarted.WithLabelValues("actor-2"))
	assert.Equal(t, float64(1), count)
}

// TestPrometheusActorPoolMetricsCollector_DeadLetter 测试死信指标
func TestPrometheusActorPoolMetricsCollector_DeadLetter(t *testing.T) {
	collector := NewPrometheusActorPoolMetricsCollector("test_dead_letter")

	// 记录死信
	collector.RecordDeadLetter("actor-1")
	collector.RecordDeadLetter("actor-1")
	collector.RecordDeadLetter("actor-2")

	// 验证指标
	count := testutil.ToFloat64(collector.deadLetters.WithLabelValues("actor-1"))
	assert.Equal(t, float64(2), count)

	count = testutil.ToFloat64(collector.deadLetters.WithLabelValues("actor-2"))
	assert.Equal(t, float64(1), count)
}

// TestPrometheusActorPoolMetricsCollector_MultipleActors 测试多Actor场景
func TestPrometheusActorPoolMetricsCollector_MultipleActors(t *testing.T) {
	collector := NewPrometheusActorPoolMetricsCollector("test_multiple_actors")

	// 模拟多个Actor处理消息
	actors := []string{"actor-1", "actor-2", "actor-3"}
	for _, actorID := range actors {
		for i := 0; i < 10; i++ {
			collector.RecordMessageSent(actorID)
			collector.RecordMessageProcessed(actorID, true, 100*time.Millisecond)
		}
	}

	// 验证每个Actor的指标
	for _, actorID := range actors {
		sentCount := testutil.ToFloat64(collector.messagesSent.WithLabelValues(actorID))
		assert.Equal(t, float64(10), sentCount)

		processedCount := testutil.ToFloat64(collector.messagesProcessed.WithLabelValues(actorID, "true"))
		assert.Equal(t, float64(10), processedCount)
	}
}

// TestPrometheusActorPoolMetricsCollector_Namespace 测试命名空间
func TestPrometheusActorPoolMetricsCollector_Namespace(t *testing.T) {
	collector := NewPrometheusActorPoolMetricsCollector("test_custom_namespace")
	require.NotNil(t, collector)
	assert.Equal(t, "test_custom_namespace", collector.namespace)
}

// TestPrometheusActorPoolMetricsCollector_ConcurrentAccess 测试并发访问
func TestPrometheusActorPoolMetricsCollector_ConcurrentAccess(t *testing.T) {
	collector := NewPrometheusActorPoolMetricsCollector("test_concurrent_access")

	// 并发记录指标
	const goroutines = 10
	const iterations = 100

	done := make(chan bool, goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			actorID := "actor-1"
			for i := 0; i < iterations; i++ {
				collector.RecordMessageSent(actorID)
				collector.RecordMessageProcessed(actorID, true, 100*time.Millisecond)
				collector.RecordInboxDepth(actorID, i, 1000)
			}
			done <- true
		}(g)
	}

	// 等待所有goroutine完成
	for g := 0; g < goroutines; g++ {
		<-done
	}

	// 验证总计数
	sentCount := testutil.ToFloat64(collector.messagesSent.WithLabelValues("actor-1"))
	assert.Equal(t, float64(goroutines*iterations), sentCount)

	processedCount := testutil.ToFloat64(collector.messagesProcessed.WithLabelValues("actor-1", "true"))
	assert.Equal(t, float64(goroutines*iterations), processedCount)
}

// TestPrometheusActorPoolMetricsCollector_EdgeCases 测试边界情况
func TestPrometheusActorPoolMetricsCollector_EdgeCases(t *testing.T) {
	collector := NewPrometheusActorPoolMetricsCollector("test_edge_cases")

	t.Run("零延迟", func(t *testing.T) {
		collector.RecordMessageProcessed("actor-1", true, 0)
		// 不应该panic
	})

	t.Run("零容量", func(t *testing.T) {
		collector.RecordInboxDepth("actor-1", 0, 0)
		// 不应该panic，利用率应该为0
		utilization := testutil.ToFloat64(collector.inboxUtilization.WithLabelValues("actor-1"))
		assert.Equal(t, float64(0), utilization)
	})

	t.Run("深度超过容量", func(t *testing.T) {
		collector.RecordInboxDepth("actor-1", 1500, 1000)
		// 利用率应该>1
		utilization := testutil.ToFloat64(collector.inboxUtilization.WithLabelValues("actor-1"))
		assert.Equal(t, 1.5, utilization)
	})

	t.Run("空ActorID", func(t *testing.T) {
		collector.RecordMessageSent("")
		collector.RecordMessageProcessed("", true, 100*time.Millisecond)
		// 不应该panic
	})
}
