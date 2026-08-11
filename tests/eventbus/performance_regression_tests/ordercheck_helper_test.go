package performance_tests

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
)

// OrderChecker 高性能顺序检查器（使用分片锁减少竞争）
//
// 抽取自 kafka_nats_envelope_comparison_test.go（该文件已加 //go:build integration），
// 以便 hermetic 的 order_checker_test.go 在默认构建里也能编译。
// broker 测试（integration 构建）与本文件同包，仍可直接使用。
type OrderChecker struct {
	shards     [256]*orderShard // 256 个分片，减少锁竞争
	violations int64            // 顺序违反计数（原子操作）
}

// orderShard 单个分片
type orderShard struct {
	mu        sync.Mutex
	sequences map[string]int64
}

// NewOrderChecker 创建顺序检查器
func NewOrderChecker() *OrderChecker {
	oc := &OrderChecker{}
	for i := 0; i < 256; i++ {
		oc.shards[i] = &orderShard{
			sequences: make(map[string]int64),
		}
	}
	return oc
}

// Check 检查顺序（线程安全，使用分片锁）
func (oc *OrderChecker) Check(aggregateID string, version int64) bool {
	// 使用 FNV-1a hash 选择分片（与 Keyed-Worker Pool 相同的算法）
	h := fnv.New32a()
	h.Write([]byte(aggregateID))
	shardIndex := h.Sum32() % 256

	shard := oc.shards[shardIndex]
	shard.mu.Lock()
	defer shard.mu.Unlock()

	lastSeq, exists := shard.sequences[aggregateID]
	if exists && version <= lastSeq {
		atomic.AddInt64(&oc.violations, 1)
		return false // 顺序违反
	}

	shard.sequences[aggregateID] = version
	return true // 顺序正确
}

// GetViolations 获取顺序违反次数
func (oc *OrderChecker) GetViolations() int64 {
	return atomic.LoadInt64(&oc.violations)
}
