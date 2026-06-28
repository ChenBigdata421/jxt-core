package eventbus

import (
	"context"
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 本文件是分区自愈（create_or_update 扩容）的回归测试，覆盖两个历史缺口：
//   - 缺口1：getActualTopicConfig 从不填充 actualConfig.Partitions，导致
//     compareTopicOptions 的 `actual.Partitions > 0` 守卫恒为假，分区漂移检测失明。
//   - 缺口2：ensureKafkaTopicIdempotent 的 update 分支只 AlterConfig，从不 CreatePartitions，
//     即使检测到漂移也无人扩容。
//
// 通过把 mockClusterAdmin 注入 k.admin（无需真实 broker）直接驱动这两条路径。

// createPartitionsCall 记录一次 CreatePartitions 调用的入参。
type createPartitionsCall struct {
	topic        string
	count        int32
	validateOnly bool
}

// mockClusterAdmin 仅覆盖 topic-config 路径用到的方法。
// 其余方法通过嵌入 sarama.ClusterAdmin（nil 接口）满足接口；本测试不会触达它们。
type mockClusterAdmin struct {
	sarama.ClusterAdmin

	topicMetadata   []*sarama.TopicMetadata
	describeErr     error
	configEntries   []sarama.ConfigEntry
	describeCfgErr  error
	alterConfigErr  error
	createPartsErr  error
	createPartsNils int // 记录 assignment 传入 nil 的次数（断言契约）

	createPartitionsCalls []createPartitionsCall
}

func (m *mockClusterAdmin) DescribeTopics(topics []string) ([]*sarama.TopicMetadata, error) {
	return m.topicMetadata, m.describeErr
}

func (m *mockClusterAdmin) DescribeConfig(resource sarama.ConfigResource) ([]sarama.ConfigEntry, error) {
	return m.configEntries, m.describeCfgErr
}

func (m *mockClusterAdmin) AlterConfig(resourceType sarama.ConfigResourceType, name string, entries map[string]*string, validateOnly bool) error {
	return m.alterConfigErr
}

func (m *mockClusterAdmin) CreatePartitions(topic string, count int32, assignment [][]int32, validateOnly bool) error {
	if assignment == nil {
		m.createPartsNils++
	}
	m.createPartitionsCalls = append(m.createPartitionsCalls, createPartitionsCall{
		topic:        topic,
		count:        count,
		validateOnly: validateOnly,
	})
	return m.createPartsErr
}

// newBusWithAdmin 构造一个仅注入 admin 的 kafkaEventBus，logger/strategy 取安全默认值。
func newBusWithAdmin(admin sarama.ClusterAdmin) *kafkaEventBus {
	k := &kafkaEventBus{
		logger:                zap.NewNop(),
		topicConfigStrategy:   StrategyCreateOrUpdate,
		topicConfigOnMismatch: TopicConfigMismatchAction{LogLevel: "warn", FailFast: false},
	}
	k.admin.Store(admin)
	return k
}

// metaWithPartitions 构造 N 个分区、每个分区 R 个副本的 TopicMetadata。
func metaWithPartitions(topic string, partitions, replicas int) []*sarama.TopicMetadata {
	pms := make([]*sarama.PartitionMetadata, 0, partitions)
	for i := 0; i < partitions; i++ {
		pms = append(pms, &sarama.PartitionMetadata{Replicas: make([]int32, replicas)})
	}
	return []*sarama.TopicMetadata{{Name: topic, Partitions: pms}}
}

// TestGetActualTopicConfig_ReportsPartitionCount 回归缺口1：
// getActualTopicConfig 必须把实际分区数填进 Partitions，否则后续比较永远检测不到漂移。
func TestGetActualTopicConfig_ReportsPartitionCount(t *testing.T) {
	admin := &mockClusterAdmin{
		topicMetadata: metaWithPartitions("t-actual", 3, 1),
	}
	k := newBusWithAdmin(admin)

	cfg, err := k.getActualTopicConfig(context.Background(), "t-actual")
	require.NoError(t, err)

	assert.Equal(t, 3, cfg.Partitions, "must report actual partition count (Gap 1: previously always 0)")
	assert.Equal(t, 1, cfg.Replicas, "replica count must still be read correctly")
}

// TestEnsureKafkaTopicIdempotent_ExpandsPartitions 回归缺口2：
// 实际 1 分区、期望 8 分区时，update 分支必须调用 CreatePartitions 扩容到 8。
func TestEnsureKafkaTopicIdempotent_ExpandsPartitions(t *testing.T) {
	admin := &mockClusterAdmin{
		topicMetadata: metaWithPartitions("t-expand", 1, 1), // 线上主题停在 1 分区
	}
	k := newBusWithAdmin(admin)
	opts := DefaultTopicOptions()
	opts.Partitions = 8

	err := k.ensureKafkaTopicIdempotent(context.Background(), "t-expand", opts, true)
	require.NoError(t, err)

	require.Len(t, admin.createPartitionsCalls, 1, "must call CreatePartitions exactly once")
	c := admin.createPartitionsCalls[0]
	assert.Equal(t, "t-expand", c.topic)
	assert.Equal(t, int32(8), c.count, "count must be the target TOTAL (8), not the delta (7)")
	assert.False(t, c.validateOnly, "must actually apply the expansion, not validate-only")
	assert.Equal(t, 1, admin.createPartsNils, "assignment must be nil so the broker auto-assigns replicas")
}

// TestEnsureKafkaTopicIdempotent_NoOpWhenAtTarget 守边界：实际 == 期望时不调用 CreatePartitions
// （Kafka 对“保持不变”的请求会返回错误）。
func TestEnsureKafkaTopicIdempotent_NoOpWhenAtTarget(t *testing.T) {
	admin := &mockClusterAdmin{
		topicMetadata: metaWithPartitions("t-same", 8, 1),
	}
	k := newBusWithAdmin(admin)
	opts := DefaultTopicOptions()
	opts.Partitions = 8

	err := k.ensureKafkaTopicIdempotent(context.Background(), "t-same", opts, true)
	require.NoError(t, err)

	assert.Empty(t, admin.createPartitionsCalls, "must not call CreatePartitions when already at target")
}

// TestEnsureKafkaTopicIdempotent_NoShrinkOnShrink 守边界：实际 > 期望时仅告警，绝不调用
// CreatePartitions 缩分区（Kafka 不支持缩减分区）。
func TestEnsureKafkaTopicIdempotent_NoShrinkOnShrink(t *testing.T) {
	admin := &mockClusterAdmin{
		topicMetadata: metaWithPartitions("t-shrink", 16, 1),
	}
	k := newBusWithAdmin(admin)
	opts := DefaultTopicOptions()
	opts.Partitions = 8 // 配置 8 分区，实际主题被手动扩到 16：actual > configured

	err := k.ensureKafkaTopicIdempotent(context.Background(), "t-shrink", opts, true)
	require.NoError(t, err)

	assert.Empty(t, admin.createPartitionsCalls, "must not call CreatePartitions on shrink (Kafka cannot shrink)")
}

// TestEnsureKafkaTopicIdempotent_CreatePartitionsErrorDoesNotAbort 守健壮性：
// CreatePartitions 失败时不能中断整体配置流程（与 AlterConfig 失败处理一致）。
func TestEnsureKafkaTopicIdempotent_CreatePartitionsErrorDoesNotAbort(t *testing.T) {
	admin := &mockClusterAdmin{
		topicMetadata:  metaWithPartitions("t-fail", 1, 1),
		createPartsErr: assert.AnError,
	}
	k := newBusWithAdmin(admin)
	opts := DefaultTopicOptions()
	opts.Partitions = 8

	err := k.ensureKafkaTopicIdempotent(context.Background(), "t-fail", opts, true)
	require.NoError(t, err, "CreatePartitions failure must not abort the configure flow")
	require.Len(t, admin.createPartitionsCalls, 1)
}

// TestConfigureTopic_SelfHealsPartitionsOnRestart 端到端回归（调度接线）：
// 模拟服务重启——本地 topicConfigs 缓存为空，Kafka 上主题已以 1 分区存在，
// 策略 create_or_update。ConfigureTopic 必须走完整链路并触发 CreatePartitions 扩容。
// 这正是原始 bug 报告描述的场景："后续每次启动都走已存在分支，永远卡在 1 分区"。
func TestConfigureTopic_SelfHealsPartitionsOnRestart(t *testing.T) {
	admin := &mockClusterAdmin{
		topicMetadata: metaWithPartitions("evidence.test.events", 1, 1),
	}
	k := newBusWithAdmin(admin)
	// 故意不预存 topicConfigs —— 模拟进程重启后的空缓存

	opts := DefaultTopicOptions()
	opts.Partitions = 8

	err := k.ConfigureTopic(context.Background(), "evidence.test.events", opts)
	require.NoError(t, err)

	require.Len(t, admin.createPartitionsCalls, 1,
		"create_or_update must expand an existing under-partitioned topic on restart")
	c := admin.createPartitionsCalls[0]
	assert.Equal(t, "evidence.test.events", c.topic)
	assert.Equal(t, int32(8), c.count)
}

// TestConfigureTopic_CreateOnlyDoesNotExpand 守策略语义：
// StrategyCreateOnly 绝不修改已存在的主题，即便分区数不足也不扩容。
func TestConfigureTopic_CreateOnlyDoesNotExpand(t *testing.T) {
	admin := &mockClusterAdmin{
		topicMetadata: metaWithPartitions("evidence.co.events", 1, 1),
	}
	k := newBusWithAdmin(admin)
	k.topicConfigStrategy = StrategyCreateOnly

	opts := DefaultTopicOptions()
	opts.Partitions = 8

	err := k.ConfigureTopic(context.Background(), "evidence.co.events", opts)
	require.NoError(t, err)

	assert.Empty(t, admin.createPartitionsCalls,
		"create_only must not touch an existing topic (no expansion, no config update)")
}

// TestKafkaEventBus_GetTopicPartitions 验证 TopicPartitionInfo 能力:查询实际分区数。
func TestKafkaEventBus_GetTopicPartitions(t *testing.T) {
	t.Run("returns actual partition count", func(t *testing.T) {
		admin := &mockClusterAdmin{topicMetadata: metaWithPartitions("t-parts", 8, 1)}
		k := newBusWithAdmin(admin)

		got, err := k.GetTopicPartitions(context.Background(), "t-parts")
		require.NoError(t, err)
		assert.Equal(t, int32(8), got, "must report the actual partition count")
	})

	t.Run("error when topic missing", func(t *testing.T) {
		admin := &mockClusterAdmin{} // topicMetadata=nil、describeErr=nil → 空元数据
		k := newBusWithAdmin(admin)

		_, err := k.GetTopicPartitions(context.Background(), "missing")
		assert.Error(t, err, "missing topic must return error (not 0)")
	})

	t.Run("error when DescribeTopics fails", func(t *testing.T) {
		admin := &mockClusterAdmin{describeErr: assert.AnError}
		k := newBusWithAdmin(admin)

		_, err := k.GetTopicPartitions(context.Background(), "t")
		assert.Error(t, err, "describe failure must surface as error")
	})
}

// TestTopicPartitionInfo_InterfaceSatisfaction 验证可选接口归属:
// kafka 实现它,memory 不实现 —— 调用方据此决定是否做分区断言(memory 自动跳过)。
func TestTopicPartitionInfo_InterfaceSatisfaction(t *testing.T) {
	var kafkaBus *kafkaEventBus
	_, ok := interface{}(kafkaBus).(TopicPartitionInfo)
	assert.True(t, ok, "kafkaEventBus must implement TopicPartitionInfo")

	var memBus *memoryEventBus
	_, ok = interface{}(memBus).(TopicPartitionInfo)
	assert.False(t, ok, "memoryEventBus must NOT implement TopicPartitionInfo (optional capability)")
}
