package main

import (
	"fmt"
	"strings"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
)

// 压缩功能验证示例
// 运行方式：go run compression_validation.go

func main() {
	fmt.Println("🎯 TopicBuilder 压缩功能验证")
	fmt.Println(strings.Repeat("=", 80))

	// 测试计数器
	totalTests := 0
	passedTests := 0

	// ========== 测试1：预设配置包含压缩 ==========
	fmt.Println("\n测试1：预设配置包含压缩")
	totalTests++
	builder1 := eventbus.NewTopicBuilder("test").ForHighThroughput()
	opts1 := builder1.GetOptions()
	if opts1.Compression == "snappy" && opts1.CompressionLevel == 6 {
		fmt.Println("  ✅ 高吞吐量预设包含 snappy 压缩")
		passedTests++
	} else {
		fmt.Printf("  ❌ 预期 snappy/6，实际 %s/%d\n", opts1.Compression, opts1.CompressionLevel)
	}

	// ========== 测试2：WithCompression 方法 ==========
	fmt.Println("\n测试2：WithCompression 方法")
	validAlgorithms := []string{"", "none", "gzip", "snappy", "lz4", "zstd"}
	for _, algo := range validAlgorithms {
		totalTests++
		builder := eventbus.NewTopicBuilder("test").WithCompression(algo)
		opts := builder.GetOptions()
		if opts.Compression == algo && builder.Validate() == nil {
			fmt.Printf("  ✅ 压缩算法 '%s' 验证通过\n", algo)
			passedTests++
		} else {
			fmt.Printf("  ❌ 压缩算法 '%s' 验证失败\n", algo)
		}
	}

	// ========== 测试3：无效压缩算法 ==========
	fmt.Println("\n测试3：无效压缩算法")
	totalTests++
	builder3 := eventbus.NewTopicBuilder("test").WithCompression("invalid")
	if err := builder3.Validate(); err != nil && strings.Contains(err.Error(), "invalid compression algorithm") {
		fmt.Println("  ✅ 正确检测到无效压缩算法")
		passedTests++
	} else {
		fmt.Println("  ❌ 未能检测到无效压缩算法")
	}

	// ========== 测试4：WithCompressionLevel 方法 ==========
	fmt.Println("\n测试4：WithCompressionLevel 方法")
	totalTests++
	builder4 := eventbus.NewTopicBuilder("test").WithCompressionLevel(6)
	opts4 := builder4.GetOptions()
	if opts4.CompressionLevel == 6 && builder4.Validate() == nil {
		fmt.Println("  ✅ 压缩级别设置成功")
		passedTests++
	} else {
		fmt.Println("  ❌ 压缩级别设置失败")
	}

	// ========== 测试5：无效压缩级别（负数） ==========
	fmt.Println("\n测试5：无效压缩级别（负数）")
	totalTests++
	builder5 := eventbus.NewTopicBuilder("test").WithCompressionLevel(-1)
	if err := builder5.Validate(); err != nil && strings.Contains(err.Error(), "compression level must be non-negative") {
		fmt.Println("  ✅ 正确检测到负数压缩级别")
		passedTests++
	} else {
		fmt.Println("  ❌ 未能检测到负数压缩级别")
	}

	// ========== 测试6：无效压缩级别（过大） ==========
	fmt.Println("\n测试6：无效压缩级别（过大）")
	totalTests++
	builder6 := eventbus.NewTopicBuilder("test").WithCompressionLevel(23)
	if err := builder6.Validate(); err != nil && strings.Contains(err.Error(), "compression level should not exceed 22") {
		fmt.Println("  ✅ 正确检测到过大的压缩级别")
		passedTests++
	} else {
		fmt.Println("  ❌ 未能检测到过大的压缩级别")
	}

	// ========== 测试7：NoCompression 快捷方法 ==========
	fmt.Println("\n测试7：NoCompression 快捷方法")
	totalTests++
	builder7 := eventbus.NewTopicBuilder("test").
		ForHighThroughput().
		NoCompression()
	opts7 := builder7.GetOptions()
	if opts7.Compression == "none" && opts7.CompressionLevel == 0 {
		fmt.Println("  ✅ NoCompression 方法正常")
		passedTests++
	} else {
		fmt.Printf("  ❌ 预期 none/0，实际 %s/%d\n", opts7.Compression, opts7.CompressionLevel)
	}

	// ========== 测试8：SnappyCompression 快捷方法 ==========
	fmt.Println("\n测试8：SnappyCompression 快捷方法")
	totalTests++
	builder8 := eventbus.NewTopicBuilder("test").SnappyCompression()
	opts8 := builder8.GetOptions()
	if opts8.Compression == "snappy" && opts8.CompressionLevel == 6 {
		fmt.Println("  ✅ SnappyCompression 方法正常")
		passedTests++
	} else {
		fmt.Printf("  ❌ 预期 snappy/6，实际 %s/%d\n", opts8.Compression, opts8.CompressionLevel)
	}

	// ========== 测试9：GzipCompression 快捷方法 ==========
	fmt.Println("\n测试9：GzipCompression 快捷方法")
	totalTests++
	builder9 := eventbus.NewTopicBuilder("test").GzipCompression(9)
	opts9 := builder9.GetOptions()
	if opts9.Compression == "gzip" && opts9.CompressionLevel == 9 && builder9.Validate() == nil {
		fmt.Println("  ✅ GzipCompression 方法正常")
		passedTests++
	} else {
		fmt.Printf("  ❌ 预期 gzip/9，实际 %s/%d\n", opts9.Compression, opts9.CompressionLevel)
	}

	// ========== 测试10：GzipCompression 无效级别 ==========
	fmt.Println("\n测试10：GzipCompression 无效级别")
	totalTests++
	builder10 := eventbus.NewTopicBuilder("test").GzipCompression(10)
	if err := builder10.Validate(); err != nil && strings.Contains(err.Error(), "gzip compression level must be 1-9") {
		fmt.Println("  ✅ 正确检测到无效的 GZIP 级别")
		passedTests++
	} else {
		fmt.Println("  ❌ 未能检测到无效的 GZIP 级别")
	}

	// ========== 测试11：Lz4Compression 快捷方法 ==========
	fmt.Println("\n测试11：Lz4Compression 快捷方法")
	totalTests++
	builder11 := eventbus.NewTopicBuilder("test").Lz4Compression()
	opts11 := builder11.GetOptions()
	if opts11.Compression == "lz4" && opts11.CompressionLevel == 0 {
		fmt.Println("  ✅ Lz4Compression 方法正常")
		passedTests++
	} else {
		fmt.Printf("  ❌ 预期 lz4/0，实际 %s/%d\n", opts11.Compression, opts11.CompressionLevel)
	}

	// ========== 测试12：ZstdCompression 快捷方法 ==========
	fmt.Println("\n测试12：ZstdCompression 快捷方法")
	totalTests++
	builder12 := eventbus.NewTopicBuilder("test").ZstdCompression(3)
	opts12 := builder12.GetOptions()
	if opts12.Compression == "zstd" && opts12.CompressionLevel == 3 && builder12.Validate() == nil {
		fmt.Println("  ✅ ZstdCompression 方法正常")
		passedTests++
	} else {
		fmt.Printf("  ❌ 预期 zstd/3，实际 %s/%d\n", opts12.Compression, opts12.CompressionLevel)
	}

	// ========== 测试13：ZstdCompression 无效级别 ==========
	fmt.Println("\n测试13：ZstdCompression 无效级别")
	totalTests++
	builder13 := eventbus.NewTopicBuilder("test").ZstdCompression(23)
	if err := builder13.Validate(); err != nil && strings.Contains(err.Error(), "zstd compression level must be 1-22") {
		fmt.Println("  ✅ 正确检测到无效的 Zstd 级别")
		passedTests++
	} else {
		fmt.Println("  ❌ 未能检测到无效的 Zstd 级别")
	}

	// ========== 测试14：压缩配置覆盖 ==========
	fmt.Println("\n测试14：压缩配置覆盖")
	totalTests++
	builder14 := eventbus.NewTopicBuilder("test").
		ForHighThroughput().
		GzipCompression(6)
	opts14 := builder14.GetOptions()
	if opts14.Compression == "gzip" && opts14.CompressionLevel == 6 && opts14.Partitions == 10 {
		fmt.Println("  ✅ 压缩配置覆盖成功，保留其他预设")
		passedTests++
	} else {
		fmt.Printf("  ❌ 压缩覆盖失败：%s/%d, 分区数=%d\n", opts14.Compression, opts14.CompressionLevel, opts14.Partitions)
	}

	// ========== 测试15：链式调用 ==========
	fmt.Println("\n测试15：链式调用")
	totalTests++
	builder15 := eventbus.NewTopicBuilder("test").
		WithPartitions(10).
		WithReplication(3).
		SnappyCompression()
	opts15 := builder15.GetOptions()
	if opts15.Partitions == 10 && opts15.ReplicationFactor == 3 && opts15.Compression == "snappy" {
		fmt.Println("  ✅ 链式调用正常")
		passedTests++
	} else {
		fmt.Println("  ❌ 链式调用失败")
	}

	// ========== 汇总结果 ==========
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("测试完成：%d/%d 通过\n", passedTests, totalTests)
	if passedTests == totalTests {
		fmt.Println("✅ 所有压缩功能测试通过！")
	} else {
		fmt.Printf("❌ %d 个测试失败\n", totalTests-passedTests)
	}
	fmt.Println(strings.Repeat("=", 80))

	// ========== 功能演示 ==========
	fmt.Println("\n📊 压缩算法对比")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-10s | %-10s | %-6s | %-8s | %-8s | %-8s\n",
		"算法", "压缩算法", "级别", "速度", "压缩率", "CPU开销")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-10s | %-10s | %-6s | %-8s | %-8s | %-8s\n",
		"None", "none", "-", "最快", "无", "无")
	fmt.Printf("%-10s | %-10s | %-6s | %-8s | %-8s | %-8s\n",
		"LZ4", "lz4", "-", "极快", "低", "极低")
	fmt.Printf("%-10s | %-10s | %-6s | %-8s | %-8s | %-8s\n",
		"Snappy", "snappy", "6", "快", "中", "低")
	fmt.Printf("%-10s | %-10s | %-6s | %-8s | %-8s | %-8s\n",
		"GZIP", "gzip", "1-9", "中", "高", "高")
	fmt.Printf("%-10s | %-10s | %-6s | %-8s | %-8s | %-8s\n",
		"Zstd", "zstd", "1-22", "快", "高", "中")
	fmt.Println(strings.Repeat("-", 80))

	fmt.Println("\n💡 推荐配置")
	fmt.Println("  • 生产环境：Snappy（平衡性能和压缩率）")
	fmt.Println("  • 高吞吐量：LZ4（最快速度）")
	fmt.Println("  • 存储优先：GZIP 或 Zstd（高压缩率）")
	fmt.Println("  • 低延迟：None（无压缩）")
	fmt.Println("  • 最佳平衡：Zstd（Kafka 2.1+）")
}

