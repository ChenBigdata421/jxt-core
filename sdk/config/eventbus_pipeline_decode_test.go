package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件钉死「死开关」修复的真正落点：YAML 的 consumer.pipeline.{enabled,windowSize}
// 必须经 viper（键 lowercase + mapstructure 嵌套解码）实际落到 config.ConsumerConfig.Pipeline。
//
// eventbus 包内的 struct→struct 转换测试（convertUserConfigToInternalKafkaConfig）只能证明
// Go 结构体之间的贯通，证明不了「YAML 键不再因无 mapstructure 目标而被静默丢弃」这一本
// 修复的核心主张。config.Setup（config.go:56）走的就是 v.Unmarshal 这条路径；这里用独立的
// viper.New() 复现同一解码行为（不污染全局 AppConfig，不触网）。tag 改名 / 嵌套解码被破坏时
// 本用例必须红，而非跟随 struct 用例一起假绿。

// TestEventBusKafkaPipeline_DecodeRoundTrip 启用写法 pipeline:{enabled:true,windowSize:4}
// 必须端到端解码到 ConsumerConfig.Pipeline。
func TestEventBusKafkaPipeline_DecodeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	// 仅放 pipeline 相关键；刻意不含 sessionTimeout 等 duration 字段，避免引入
	// time.Duration 解码依赖，把断言聚焦在「嵌套 pipeline 段是否被解码」这一死开关机制上。
	content := []byte(`
kafka:
  brokers:
    - "localhost:9092"
  consumer:
    groupId: "g-decode-roundtrip"
    pipeline:
      enabled: true
      windowSize: 4
`)
	require.NoError(t, os.WriteFile(path, content, 0o644))

	v := viper.New()
	v.SetConfigFile(path)
	require.NoError(t, v.ReadInConfig())

	var eb EventBusConfig
	require.NoError(t, v.Unmarshal(&eb))

	assert.True(t, eb.Kafka.Consumer.Pipeline.Enabled,
		"死开关回归：YAML pipeline.enabled 必须真实解码到 ConsumerConfig.Pipeline，而非被静默丢弃")
	assert.Equal(t, 4, eb.Kafka.Consumer.Pipeline.WindowSize,
		"windowSize 必须真实解码（viper lowercase + mapstructure tag 嵌套匹配）")
}

// TestEventBusKafkaPipeline_DecodeDefaultOff 缺省 pipeline 段 → Enabled=false、WindowSize=0（非破坏 no-op）。
func TestEventBusKafkaPipeline_DecodeDefaultOff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	content := []byte(`
kafka:
  brokers:
    - "localhost:9092"
  consumer:
    groupId: "g-no-pipeline"
`)
	require.NoError(t, os.WriteFile(path, content, 0o644))

	v := viper.New()
	v.SetConfigFile(path)
	require.NoError(t, v.ReadInConfig())

	var eb EventBusConfig
	require.NoError(t, v.Unmarshal(&eb))

	assert.False(t, eb.Kafka.Consumer.Pipeline.Enabled, "absent pipeline must stay disabled (no-op default)")
	assert.Equal(t, 0, eb.Kafka.Consumer.Pipeline.WindowSize, "WindowSize 0 (runtime applyPipelineDefaults owns the default)")
}
