package eventbus

import "testing"

// The seam is no-op by default; the service wires prometheus into it. Core MUST
// NOT import prometheus (§8.4③ / J2). These tests verify the seam forwards
// (topic, partition, seconds) to the injected reporter and no-ops safely when
// unset or when topic is empty (empty-topic guard, review 2026-07-26).
func TestPartitionStall_SeamForwardsToReporter(t *testing.T) {
	var gotTopic string
	var gotPart int32
	var gotSecs float64
	StallReporter = func(topic string, partition int32, seconds float64) {
		gotTopic, gotPart, gotSecs = topic, partition, seconds
	}
	defer func() { StallReporter = nil }()
	ReportPartitionStall("orders", 3, 42.5)
	if gotTopic != "orders" || gotPart != 3 || gotSecs != 42.5 {
		t.Fatalf("seam forwarded %s/%d/%v, want orders/3/42.5", gotTopic, gotPart, gotSecs)
	}
}

func TestPartitionStall_EmptyTopicNoOps(t *testing.T) {
	called := false
	StallReporter = func(string, int32, float64) { called = true }
	defer func() { StallReporter = nil }()
	ReportPartitionStall("", 3, 42.5) // partial-revert / pre-claim state
	if called {
		t.Fatal("empty topic must not call the reporter (cardinality guard)")
	}
}

func TestPartitionStall_NilReportersNoOp(t *testing.T) {
	StallReporter = nil
	StallEnterReporter = nil
	ReportPartitionStall("orders", 3, 42.5)   // must not panic
	ReportPartitionStallEnter("orders", 3)
	ClearPartitionStall("orders", 3)
}

// TestPartitionStall_ClearForwardsAndGuards 验证 clear 路径与 report 路径对称：
// 转发 (topic, partition)、空 topic no-op（cardinality 守卫）、未注入 reporter no-op（不 panic）。
// 补齐 seam 的测试对等——report 路径已有 EmptyTopic/Nil 守卫测试，clear 路径此前仅由
// NilReportersNoOp 顺带覆盖一个分支，删守卫的回归不会被捕获。
func TestPartitionStall_ClearForwardsAndGuards(t *testing.T) {
	var cleared []int32
	SetStallClearReporter(func(_ string, part int32) { cleared = append(cleared, part) })
	defer SetStallClearReporter(nil)

	ClearPartitionStall("orders", 7)
	ClearPartitionStall("", 99) // 空 topic 守卫：不得调用
	if len(cleared) != 1 || cleared[0] != 7 {
		t.Fatalf("clear forwarded %v, want [7]", cleared)
	}

	SetStallClearReporter(nil)
	ClearPartitionStall("orders", 7) // 未注入 reporter：不得 panic
}
