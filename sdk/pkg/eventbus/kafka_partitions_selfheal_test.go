package eventbus

import (
	"context"
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 本文件原是「分区自愈（create_or_update 自动扩容）」的回归测试。
//
// 方案改动5（docs/analysis/redpanda主题创建优化方案_v2.md §三 改动5 / §九）已【移除】
// jxt-core 的分区扩容能力（CreatePartitions）——分区正确性改由 infra bootstrap 双遍
// 断言 + redpanda healthcheck (CHECK_ONLY) 独占收敛，jxt-core 退化为「不创建、不扩容、
// 不断言分区数」，只负责连上去、应用留存期/压缩配置、收发。
//
// 因此本文件现在守护的是【相反】的不变量——「绝不扩容」：确保将来不会有人重新引入
// 运行期扩容（对已被订阅的 topic 扩分区会触发消费组再平衡，方案 §八 硬约束红线）。
// 同时覆盖改动5 的 AUTO_CREATE_TOPICS 门禁与改动6 的 WaitForTopologyReady 哨兵 helper。

// createPartitionsCall 记录一次 CreatePartitions 调用的入参（生产代码已不再调用，
// mock 仍保留以便断言「零调用」）。
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

	createTopicErr  error
	createTopicCalls []string

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

func (m *mockClusterAdmin) CreateTopic(topic string, detail *sarama.TopicDetail, validateOnly bool) error {
	m.createTopicCalls = append(m.createTopicCalls, topic)
	return m.createTopicErr
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

// TestEnsureKafkaTopicIdempotent_NeverExpandsPartitions 守护改动5 的「绝不扩容」不变量：
// 即便实际 1 分区、期望 8 分区，update 分支也【绝不】调用 CreatePartitions——
// 分区由 infra bootstrap 收敛，jxt-core 运行期扩容会触发再平衡（方案 §八 红线）。
func TestEnsureKafkaTopicIdempotent_NeverExpandsPartitions(t *testing.T) {
	admin := &mockClusterAdmin{
		topicMetadata: metaWithPartitions("t-expand", 1, 1), // 线上主题停在 1 分区
	}
	k := newBusWithAdmin(admin)
	opts := DefaultTopicOptions()
	opts.Partitions = 8

	err := k.ensureKafkaTopicIdempotent(context.Background(), "t-expand", opts, true)
	require.NoError(t, err, "under-partitioned topic is not an error; partitions are bootstrap-managed")

	assert.Empty(t, admin.createPartitionsCalls, "must NOT call CreatePartitions (expansion removed by 改动5)")
}

// TestEnsureKafkaTopicIdempotent_NoOpWhenAtTarget 守边界：实际 == 期望时不调用 CreatePartitions。
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
// CreatePartitions（既不扩也不缩；Kafka 不支持缩减分区）。
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

// TestConfigureTopic_DoesNotSelfHealPartitionsOnRestart 端到端守护改动5：
// 模拟服务重启——本地 topicConfigs 缓存为空，Kafka 上主题已以 1 分区存在，策略 create_or_update。
// ConfigureTopic 必须走完链路但【绝不】触发 CreatePartitions 扩容。
func TestConfigureTopic_DoesNotSelfHealPartitionsOnRestart(t *testing.T) {
	admin := &mockClusterAdmin{
		topicMetadata: metaWithPartitions("evidence.test.events", 1, 1),
	}
	k := newBusWithAdmin(admin)
	// 故意不预存 topicConfigs —— 模拟进程重启后的空缓存

	opts := DefaultTopicOptions()
	opts.Partitions = 8

	err := k.ConfigureTopic(context.Background(), "evidence.test.events", opts)
	require.NoError(t, err)

	assert.Empty(t, admin.createPartitionsCalls,
		"create_or_update must NOT expand an existing under-partitioned topic (改动5: expansion removed)")
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

// --- 改动5：AUTO_CREATE_TOPICS 门禁（生产路径默认不建 topic）---

// TestCreateKafkaTopic_RefusesWithoutAutoCreate 默认（未设 AUTO_CREATE_TOPICS）时，
// createKafkaTopic 必须直接 fail-fast，绝不触碰 admin.CreateTopic。
func TestCreateKafkaTopic_RefusesWithoutAutoCreate(t *testing.T) {
	// 显式确保未设置（其它测试用例可能已 t.Setenv，但 Go 测试串行执行且 t.Setenv 自动还原）
	t.Setenv("AUTO_CREATE_TOPICS", "")
	admin := &mockClusterAdmin{}
	k := newBusWithAdmin(admin)

	err := k.createKafkaTopic("t-new", DefaultTopicOptions())
	require.Error(t, err, "must refuse to create topic when AUTO_CREATE_TOPICS is unset (生产路径不建 topic)")
	assert.Empty(t, admin.createTopicCalls, "must not reach admin.CreateTopic when gate is closed")
}

// TestCreateKafkaTopic_CreatesWithAutoCreateEnabled 显式 AUTO_CREATE_TOPICS=1（开发/CI）
// 时，createKafkaTopic 放行建新，调用一次 admin.CreateTopic。
func TestCreateKafkaTopic_CreatesWithAutoCreateEnabled(t *testing.T) {
	t.Setenv("AUTO_CREATE_TOPICS", "1")
	admin := &mockClusterAdmin{}
	k := newBusWithAdmin(admin)

	err := k.createKafkaTopic("t-dev", DefaultTopicOptions())
	require.NoError(t, err, "must create topic when AUTO_CREATE_TOPICS=1 (开发开关)")
	require.Len(t, admin.createTopicCalls, 1, "must call admin.CreateTopic exactly once")
	assert.Equal(t, "t-dev", admin.createTopicCalls[0])
}

// TestEnsureKafkaTopicIdempotent_MissingTopicRefusesWithoutAutoCreate 守门禁接线：
// 主题缺失时，ensureKafkaTopicIdempotent 经 createKafkaTopic 的门禁返回错误（而非静默建出）。
func TestEnsureKafkaTopicIdempotent_MissingTopicRefusesWithoutAutoCreate(t *testing.T) {
	t.Setenv("AUTO_CREATE_TOPICS", "")
	// 主题缺失：DescribeTopics 返回非空切片 + per-topic Err（真实 sarama 的缺失主题表现）
	admin := &mockClusterAdmin{
		topicMetadata: []*sarama.TopicMetadata{
			{Name: "ghost", Err: sarama.ErrUnknownTopicOrPartition},
		},
	}
	k := newBusWithAdmin(admin)

	err := k.ensureKafkaTopicIdempotent(context.Background(), "ghost", DefaultTopicOptions(), true)
	require.Error(t, err, "missing topic must fail-fast instead of being silently created")
	assert.Empty(t, admin.createTopicCalls, "gate must block creation in production path")
}

// TestAutoCreateTopicsEnabled 守开关解析：仅 1/true（大小写不敏感）为开，其余（含空/非法）一律关。
func TestAutoCreateTopicsEnabled(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"0":     false,
		"false": false,
		"False": false,
		"no":    false, // ParseBool 不认 no → false（fail-safe）
		"2":     false, // 非法 → false
		"1":     true,
		"true":  true,
		"TRUE":  true,
	}
	for val, want := range cases {
		t.Run("env="+val, func(t *testing.T) {
			t.Setenv("AUTO_CREATE_TOPICS", val)
			assert.Equal(t, want, autoCreateTopicsEnabled(), "AUTO_CREATE_TOPICS=%q", val)
		})
	}
}

// --- 改动6：WaitForTopologyReady 哨兵 helper ---

// TestWaitForTopologyReady 守就绪门禁 helper：
//   - kafka 总线 + 标志存在 → nil；
//   - kafka 总线 + 标志缺失 → error；
//   - 非 kafka 总线（memory）→ nil（自动放行）。
func TestWaitForTopologyReady(t *testing.T) {
	t.Run("kafka bus: ready when marker exists", func(t *testing.T) {
		admin := &mockClusterAdmin{
			topicMetadata: metaWithPartitions(TopologyReadyTopic, 1, 1),
		}
		k := newBusWithAdmin(admin)

		assert.NoError(t, WaitForTopologyReady(context.Background(), k))
	})

	t.Run("kafka bus: not ready when marker missing", func(t *testing.T) {
		admin := &mockClusterAdmin{
			topicMetadata: []*sarama.TopicMetadata{
				{Name: TopologyReadyTopic, Err: sarama.ErrUnknownTopicOrPartition},
			},
		}
		k := newBusWithAdmin(admin)

		err := WaitForTopologyReady(context.Background(), k)
		require.Error(t, err, "missing marker must fail-fast")
		assert.Contains(t, err.Error(), "topology not ready")
	})

	t.Run("non-kafka bus: pass-through (nil)", func(t *testing.T) {
		// 未实现 TopicPartitionInfo 的总线（NATS/memory 等）→ 类型断言 ok=false → 自动放行，
		// 避免影响单测/NATS 部署。用 nil EventBus 接口模拟「不实现该能力」的分支。
		var bus EventBus
		assert.NoError(t, WaitForTopologyReady(context.Background(), bus))
	})
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

	t.Run("error when topic missing (empty metadata slice)", func(t *testing.T) {
		// 防御性路径：某些实现/未来版本可能对缺失主题返回空切片。
		// 真实 sarama 返回的是非空切片 + per-topic Err，由下面那个用例覆盖。
		admin := &mockClusterAdmin{} // topicMetadata=nil、describeErr=nil → 空元数据
		k := newBusWithAdmin(admin)

		_, err := k.GetTopicPartitions(context.Background(), "missing")
		assert.Error(t, err, "missing topic must return error (not 0)")
	})

	t.Run("error when broker reports ErrUnknownTopicOrPartition", func(t *testing.T) {
		// 真实 sarama 对不存在的主题不返回顶层 error，而是返回长度为 1 的切片，
		// 其中 metadata[0].Err == ErrUnknownTopicOrPartition、Partitions 为空。
		// 这正是 P2 修复要覆盖的路径：旧实现仅判 len==0 会漏判，返回 (0, nil)。
		admin := &mockClusterAdmin{
			topicMetadata: []*sarama.TopicMetadata{
				{Name: "ghost", Err: sarama.ErrUnknownTopicOrPartition},
			},
		}
		k := newBusWithAdmin(admin)

		got, err := k.GetTopicPartitions(context.Background(), "ghost")
		assert.Error(t, err, "broker-reported missing topic must surface as error (not 0,nil)")
		assert.Equal(t, int32(0), got, "must not return a partition count for a missing topic")
	})

	t.Run("error when DescribeTopics fails", func(t *testing.T) {
		admin := &mockClusterAdmin{describeErr: assert.AnError}
		k := newBusWithAdmin(admin)

		_, err := k.GetTopicPartitions(context.Background(), "t")
		assert.Error(t, err, "describe failure must surface as error")
	})

	t.Run("error when admin not initialized", func(t *testing.T) {
		// 未注入 admin(getAdmin 返回 error)——覆盖之前漏掉的分支
		k := &kafkaEventBus{logger: zap.NewNop()}
		_, err := k.GetTopicPartitions(context.Background(), "any-topic")
		assert.Error(t, err, "admin not initialized must surface as error")
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
