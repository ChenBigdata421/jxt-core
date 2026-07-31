package eventbus

import (
	"testing"
)

// TestIsActiveTopic_AtomicTransitions 验证 IsActiveTopic 访问器在真实 activeTopicHandlers
// sync.Map 状态上反映 false → true → false 的生命周期。
// Activate → Store；Deactivate → Delete：访问器只读 map，故直接驱动 map 等价于走真实
// activateTopicHandler/deactivateTopicHandler 路径，且不依赖 broker / logger 接线。
func TestIsActiveTopic_AtomicTransitions(t *testing.T) {
	bus := &kafkaEventBus{} // activeTopicHandlers 为零值 sync.Map，可直接使用
	const topic = "domain.evidence.created"

	// 预状态：尚未激活 → 必须返回 false。
	if bus.IsActiveTopic(topic) {
		t.Fatalf("pre-activate: IsActiveTopic(%q) = true, want false", topic)
	}

	// 激活：等价于 activateTopicHandler 的 map 副作用。
	bus.activeTopicHandlers.Store(topic, &handlerWrapper{})

	// 激活后：必须返回 true。
	if !bus.IsActiveTopic(topic) {
		t.Fatalf("post-activate: IsActiveTopic(%q) = false, want true", topic)
	}

	// 停用：等价于 deactivateTopicHandler 的 map 副作用。
	bus.activeTopicHandlers.Delete(topic)

	// 停用后：必须返回 false。
	if bus.IsActiveTopic(topic) {
		t.Fatalf("post-deactivate: IsActiveTopic(%q) = true, want false", topic)
	}
}

// TestIsActiveTopic_UnactivatedTopicReturnedFalse 覆盖从未激活过的 topic 名，
// 确保 Load 对缺失 key 的零值行为（ok=false）被正确翻译为 false。
func TestIsActiveTopic_UnactivatedTopicReturnedFalse(t *testing.T) {
	bus := &kafkaEventBus{}
	const topic = "never.activated"

	if bus.IsActiveTopic(topic) {
		t.Fatalf("IsActiveTopic(%q) = true for never-activated topic, want false", topic)
	}
}
