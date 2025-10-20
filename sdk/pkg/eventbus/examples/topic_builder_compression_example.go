package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// 演示 TopicBuilder 的压缩配置功能
// 运行方式：go run topic_builder_compression_example.go

func mainCompression() {
	ctx := context.Background()

	// 模拟 EventBus（实际使用时替换为真实的 Kafka EventBus）
	var bus eventbus.EventBus // 这里需要实际的 EventBus 实例

	fmt.Println("🎯 TopicBuilder 压缩配置示例")
	fmt.Println(strings.Repeat("=", 80))

	// ========== 示例1：使用预设配置（已包含 Snappy 压缩） ==========
	fmt.Println("\n📦 示例1：使用预设配置（已包含 Snappy 压缩）")
	fmt.Println(strings.Repeat("-", 80))

	builder1 := eventbus.NewTopicBuilder("orders-high-throughput").
		ForHighThroughput() // 预设已包含 snappy 压缩

	options1 := builder1.GetOptions()
	fmt.Printf("压缩算法: %s\n", options1.Compression)
	fmt.Printf("压缩级别: %d\n", options1.CompressionLevel)
	fmt.Printf("分区数: %d\n", options1.Partitions)
	fmt.Printf("副本因子: %d\n", options1.ReplicationFactor)

	// ========== 示例2：禁用压缩 ==========
	fmt.Println("\n🚫 示例2：禁用压缩")
	fmt.Println(strings.Repeat("-", 80))

	builder2 := eventbus.NewTopicBuilder("orders-no-compression").
		ForHighThroughput().
		NoCompression() // 禁用压缩

	options2 := builder2.GetOptions()
	fmt.Printf("压缩算法: %s\n", options2.Compression)
	fmt.Printf("压缩级别: %d\n", options2.CompressionLevel)

	// ========== 示例3：使用 Snappy 压缩（快捷方法） ==========
	fmt.Println("\n⚡ 示例3：使用 Snappy 压缩（推荐）")
	fmt.Println(strings.Repeat("-", 80))

	builder3 := eventbus.NewTopicBuilder("orders-snappy").
		WithPartitions(10).
		WithReplication(3).
		SnappyCompression() // 快捷方法

	options3 := builder3.GetOptions()
	fmt.Printf("压缩算法: %s\n", options3.Compression)
	fmt.Printf("压缩级别: %d\n", options3.CompressionLevel)
	fmt.Println("✅ Snappy 提供了性能和压缩率的最佳平衡")

	// ========== 示例4：使用 GZIP 压缩（高压缩率） ==========
	fmt.Println("\n📦 示例4：使用 GZIP 压缩（高压缩率）")
	fmt.Println(strings.Repeat("-", 80))

	builder4 := eventbus.NewTopicBuilder("orders-gzip").
		WithPartitions(10).
		WithReplication(3).
		GzipCompression(9) // 级别9 = 最高压缩率

	options4 := builder4.GetOptions()
	fmt.Printf("压缩算法: %s\n", options4.Compression)
	fmt.Printf("压缩级别: %d\n", options4.CompressionLevel)
	fmt.Println("⚠️  GZIP 提供高压缩率，但 CPU 开销较大")

	// ========== 示例5：使用 LZ4 压缩（最快速度） ==========
	fmt.Println("\n🚀 示例5：使用 LZ4 压缩（最快速度）")
	fmt.Println(strings.Repeat("-", 80))

	builder5 := eventbus.NewTopicBuilder("orders-lz4").
		WithPartitions(10).
		WithReplication(3).
		Lz4Compression() // 最快速度

	options5 := builder5.GetOptions()
	fmt.Printf("压缩算法: %s\n", options5.Compression)
	fmt.Printf("压缩级别: %d\n", options5.CompressionLevel)
	fmt.Println("✅ LZ4 提供最快的压缩/解压速度")

	// ========== 示例6：使用 Zstd 压缩（最佳平衡，Kafka 2.1+） ==========
	fmt.Println("\n🎯 示例6：使用 Zstd 压缩（最佳平衡）")
	fmt.Println(strings.Repeat("-", 80))

	builder6 := eventbus.NewTopicBuilder("orders-zstd").
		WithPartitions(10).
		WithReplication(3).
		ZstdCompression(3) // 级别3 = 默认平衡

	options6 := builder6.GetOptions()
	fmt.Printf("压缩算法: %s\n", options6.Compression)
	fmt.Printf("压缩级别: %d\n", options6.CompressionLevel)
	fmt.Println("✅ Zstd 提供最佳的压缩率和性能平衡（需要 Kafka 2.1+）")

	// ========== 示例7：使用通用方法设置压缩 ==========
	fmt.Println("\n🔧 示例7：使用通用方法设置压缩")
	fmt.Println(strings.Repeat("-", 80))

	builder7 := eventbus.NewTopicBuilder("orders-custom").
		WithPartitions(10).
		WithReplication(3).
		WithCompression("snappy").
		WithCompressionLevel(6)

	options7 := builder7.GetOptions()
	fmt.Printf("压缩算法: %s\n", options7.Compression)
	fmt.Printf("压缩级别: %d\n", options7.CompressionLevel)

	// ========== 示例8：预设 + 覆盖压缩配置 ==========
	fmt.Println("\n🔄 示例8：预设 + 覆盖压缩配置")
	fmt.Println(strings.Repeat("-", 80))

	builder8 := eventbus.NewTopicBuilder("orders-override").
		ForHighThroughput(). // 预设包含 snappy 压缩
		GzipCompression(6)   // 覆盖为 gzip 压缩

	options8 := builder8.GetOptions()
	fmt.Printf("压缩算法: %s（覆盖后）\n", options8.Compression)
	fmt.Printf("压缩级别: %d（覆盖后）\n", options8.CompressionLevel)
	fmt.Printf("分区数: %d（保留预设）\n", options8.Partitions)

	// ========== 示例9：完整的生产环境配置 ==========
	fmt.Println("\n🏭 示例9：完整的生产环境配置")
	fmt.Println(strings.Repeat("-", 80))

	builder9 := eventbus.NewTopicBuilder("orders-production").
		WithPartitions(15).
		WithReplication(3).
		SnappyCompression().
		WithRetention(7 * 24 * time.Hour).
		WithMaxSize(5 * 1024 * 1024 * 1024). // 5GB
		WithDescription("Production orders topic with snappy compression")

	options9 := builder9.GetOptions()
	fmt.Printf("主题: orders-production\n")
	fmt.Printf("分区数: %d\n", options9.Partitions)
	fmt.Printf("副本因子: %d\n", options9.ReplicationFactor)
	fmt.Printf("压缩算法: %s\n", options9.Compression)
	fmt.Printf("压缩级别: %d\n", options9.CompressionLevel)
	fmt.Printf("保留时间: %v\n", options9.RetentionTime)
	fmt.Printf("最大大小: %d GB\n", options9.MaxSize/(1024*1024*1024))
	fmt.Printf("描述: %s\n", options9.Description)

	// ========== 示例10：压缩算法对比 ==========
	fmt.Println("\n📊 示例10：压缩算法对比")
	fmt.Println(strings.Repeat("-", 80))

	compressionComparison := []struct {
		name        string
		algorithm   string
		level       int
		speed       string
		ratio       string
		cpuCost     string
		recommended string
	}{
		{"None", "none", 0, "最快", "无", "无", "低延迟场景"},
		{"LZ4", "lz4", 0, "极快", "低", "极低", "高吞吐量场景"},
		{"Snappy", "snappy", 6, "快", "中", "低", "生产环境推荐"},
		{"GZIP", "gzip", 6, "中", "高", "高", "存储优先场景"},
		{"Zstd", "zstd", 3, "快", "高", "中", "最佳平衡（Kafka 2.1+）"},
	}

	fmt.Printf("%-10s | %-10s | %-6s | %-8s | %-8s | %-8s | %-20s\n",
		"算法", "压缩算法", "级别", "速度", "压缩率", "CPU开销", "推荐场景")
	fmt.Println(strings.Repeat("-", 100))

	for _, comp := range compressionComparison {
		fmt.Printf("%-10s | %-10s | %-6d | %-8s | %-8s | %-8s | %-20s\n",
			comp.name, comp.algorithm, comp.level, comp.speed, comp.ratio, comp.cpuCost, comp.recommended)
	}

	// ========== 性能建议 ==========
	fmt.Println("\n💡 性能建议")
	fmt.Println(strings.Repeat("=", 80))

	fmt.Println("\n🎯 选择压缩算法的原则：")
	fmt.Println("   1. 网络带宽受限 → 选择高压缩率算法（gzip, zstd）")
	fmt.Println("   2. CPU 资源受限 → 选择低 CPU 开销算法（lz4, snappy）")
	fmt.Println("   3. 存储成本高 → 选择高压缩率算法（gzip, zstd）")
	fmt.Println("   4. 延迟敏感 → 选择快速算法（lz4, snappy）或不压缩")
	fmt.Println("   5. 平衡场景 → 选择 snappy（生产环境推荐）")

	fmt.Println("\n📊 压缩效果估算（文本/JSON 数据）：")
	fmt.Println("   • None:   1.0x（无压缩）")
	fmt.Println("   • LZ4:    2-3x（快速，低压缩率）")
	fmt.Println("   • Snappy: 2-4x（平衡）")
	fmt.Println("   • GZIP:   5-10x（高压缩率）")
	fmt.Println("   • Zstd:   5-12x（最佳平衡）")

	fmt.Println("\n⚠️  注意事项：")
	fmt.Println("   1. 压缩算法一旦设置，建议不要频繁更改")
	fmt.Println("   2. 不同压缩算法的消息可以共存于同一 topic")
	fmt.Println("   3. 消费者会自动解压，无需额外配置")
	fmt.Println("   4. 二进制数据（图片、视频）压缩效果有限")
	fmt.Println("   5. 已压缩的数据（如 gzip 文件）不建议再次压缩")

	fmt.Println("\n✅ 压缩配置示例演示完成！")
	fmt.Println(strings.Repeat("=", 80))

	// 注意：实际使用时需要调用 Build() 方法
	// err := builder9.Build(ctx, bus)
	// if err != nil {
	//     fmt.Printf("❌ 构建失败: %v\n", err)
	// }

	_ = ctx
	_ = bus
}
