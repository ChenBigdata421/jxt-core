package mycasbin

import (
	"errors"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/stretchr/testify/assert"
)

// ErroringAdapter 是一个总是返回错误的 Adapter，用于测试 LoadPolicy 错误处理
type ErroringAdapter struct{}

func (a *ErroringAdapter) LoadPolicy(m model.Model) error {
	return errors.New("simulated load policy error")
}

func (a *ErroringAdapter) SavePolicy(m model.Model) error {
	return nil
}

func (a *ErroringAdapter) AddPolicy(sec string, ptype string, rule []string) error {
	return nil
}

func (a *ErroringAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	return nil
}

func (a *ErroringAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	return nil
}

// SuccessAdapter 是一个成功的 Adapter，用于初始化
type SuccessAdapter struct{}

func (a *SuccessAdapter) LoadPolicy(m model.Model) error {
	// 不做任何事，返回成功
	return nil
}

func (a *SuccessAdapter) SavePolicy(m model.Model) error {
	return nil
}

func (a *SuccessAdapter) AddPolicy(sec string, ptype string, rule []string) error {
	return nil
}

func (a *SuccessAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	return nil
}

func (a *SuccessAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	return nil
}

// TestLoadPolicy_ErrorBehavior 验证 SyncedEnforcer.LoadPolicy() 在 Adapter 返回错误时的行为
// 关键问题：当 adapter.LoadPolicy() 返回错误时，enforcer 是否保留原策略（不清空）？
//
// 这个测试模拟 Redis Watcher 回调时 gRPC 调用失败的场景：
// 1. 微服务启动时成功加载策略到内存
// 2. security-management 修改权限并发布 Redis 通知
// 3. 微服务收到通知，调用 LoadPolicy() 重新加载
// 4. 但此时 gRPC 调用失败（网络问题、服务不可用等）
// 5. 关键问题：微服务内存中的原有策略是否被保留？
func TestLoadPolicy_ErrorBehavior(t *testing.T) {
	// 1. 创建 model（使用 jxt-core 实际的模型定义）
	m, err := model.NewModelFromString(text)
	assert.NoError(t, err, "模型创建应成功")

	// 2. 创建 enforcer（使用 SuccessAdapter，模拟正常启动）
	e, err := casbin.NewSyncedEnforcer(m, &SuccessAdapter{})
	assert.NoError(t, err, "Enforcer 创建应成功")
	assert.NotNil(t, e, "Enforcer 不应为 nil")

	// 3. 手动添加策略到 model（模拟初始成功加载的策略）
	added, err := e.AddPolicy("admin", "/api/test", "GET")
	assert.NoError(t, err, "添加策略应成功")
	assert.True(t, added, "策略应被添加")

	added, err = e.AddPolicy("user", "/api/read", "GET")
	assert.NoError(t, err, "添加策略应成功")
	assert.True(t, added, "策略应被添加")

	// 验证策略已存在
	policiesBefore := e.GetPolicy()
	policyCountBefore := len(policiesBefore)

	assert.Equal(t, 2, policyCountBefore, "初始应有 2 条策略")

	t.Logf("LoadPolicy 失败前 - 策略数: %d", policyCountBefore)
	for i, p := range policiesBefore {
		t.Logf("  策略 %d: %v", i+1, p)
	}

	// 4. 替换为 ErroringAdapter，然后调用 LoadPolicy
	// 这是模拟 Redis Watcher 回调时 gRPC 调用失败的场景
	e.SetAdapter(&ErroringAdapter{})

	err = e.LoadPolicy()

	// 5. 验证行为
	assert.Error(t, err, "LoadPolicy 应返回错误")

	policiesAfter := e.GetPolicy()
	policyCountAfter := len(policiesAfter)

	t.Logf("LoadPolicy 失败后 - 策略数: %d", policyCountAfter)
	for i, p := range policiesAfter {
		t.Logf("  策略 %d: %v", i+1, p)
	}

	// 关键验证：策略是否被保留？
	// 如果 Casbin 保留原策略，policyCountAfter 应等于 policyCountBefore
	// 如果 Casbin 清空了策略，policyCountAfter 应等于 0
	if policyCountAfter == policyCountBefore {
		t.Log("✅ 验证通过：Casbin v2.54.0 在 Adapter 返回错误时保留所有原策略")
	} else if policyCountAfter == 0 {
		t.Error("❌ 验证失败：Casbin v2.54.0 在 Adapter 返回错误时清空了所有策略（需要添加保护逻辑）")
	} else {
		t.Logf("⚠️  部分清空：策略数从 %d 变为 %d", policyCountBefore, policyCountAfter)
	}
}

// TestLoadPolicy_ErrorBehaviorWithMultiplePolicies 测试多条策略的保护
func TestLoadPolicy_ErrorBehaviorWithMultiplePolicies(t *testing.T) {
	m, err := model.NewModelFromString(text)
	assert.NoError(t, err, "模型创建应成功")

	e, err := casbin.NewSyncedEnforcer(m, &SuccessAdapter{})
	assert.NoError(t, err, "Enforcer 创建应成功")
	assert.NotNil(t, e, "Enforcer 不应为 nil")

	// 添加多条策略（模拟真实的权限配置）
	policiesToAdd := [][]string{
		{"admin", "/api/v1/*", "*"},
		{"admin", "/api/v2/*", "GET"},
		{"user", "/api/v1/read", "GET"},
		{"user", "/api/v1/write", "POST"},
		{"guest", "/api/public/*", "GET"},
	}

	for _, p := range policiesToAdd {
		_, _ = e.AddPolicy(p[0], p[1], p[2])
	}

	policiesBefore := e.GetPolicy()
	policyCountBefore := len(policiesBefore)

	t.Logf("LoadPolicy 失败前 - 策略数: %d", policyCountBefore)

	// 替换为 ErroringAdapter，然后调用 LoadPolicy
	e.SetAdapter(&ErroringAdapter{})
	err = e.LoadPolicy()

	assert.Error(t, err, "LoadPolicy 应返回错误")

	policiesAfter := e.GetPolicy()
	policyCountAfter := len(policiesAfter)

	t.Logf("LoadPolicy 失败后 - 策略数: %d", policyCountAfter)

	// 验证策略是否被保留
	if policyCountAfter == policyCountBefore {
		t.Log("✅ 验证通过：Casbin v2.54.0 在 Adapter 返回错误时保留所有原策略（多条策略场景）")
	} else {
		t.Errorf("❌ 验证失败：策略数从 %d 变为 %d", policyCountBefore, policyCountAfter)
	}
}

// TestLoadPolicy_RBMModelWithRoleDefinition 测试带有角色定义的 RBAC 模型
// （虽然 jxt-core 当前不使用角色定义，但为了完整性测试）
func TestLoadPolicy_RBMModelWithRoleDefinition(t *testing.T) {
	// 使用带角色定义的 RBAC 模型
	rbacModelText := `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act
`

	m, err := model.NewModelFromString(rbacModelText)
	assert.NoError(t, err, "模型创建应成功")

	e, err := casbin.NewSyncedEnforcer(m, &SuccessAdapter{})
	assert.NoError(t, err, "Enforcer 创建应成功")

	// 添加策略和角色继承
	_, _ = e.AddPolicy("admin", "/api/admin", "GET")
	_, _ = e.AddPolicy("user", "/api/user", "GET")
	_, _ = e.AddGroupingPolicy("alice", "admin")
	_, _ = e.AddGroupingPolicy("bob", "user")

	policiesBefore := e.GetPolicy()
	groupingPoliciesBefore := e.GetGroupingPolicy()

	t.Logf("RBAC 模型 - LoadPolicy 失败前 - 策略数: %d, 角色继承数: %d",
		len(policiesBefore), len(groupingPoliciesBefore))

	// 替换为 ErroringAdapter
	e.SetAdapter(&ErroringAdapter{})
	err = e.LoadPolicy()

	assert.Error(t, err, "LoadPolicy 应返回错误")

	policiesAfter := e.GetPolicy()
	groupingPoliciesAfter := e.GetGroupingPolicy()

	t.Logf("RBAC 模型 - LoadPolicy 失败后 - 策略数: %d, 角色继承数: %d",
		len(policiesAfter), len(groupingPoliciesAfter))

	// 验证所有策略都被保留
	policiesPreserved := len(policiesAfter) == len(policiesBefore)
	groupingPoliciesPreserved := len(groupingPoliciesAfter) == len(groupingPoliciesBefore)

	if policiesPreserved && groupingPoliciesPreserved {
		t.Log("✅ 验证通过：Casbin v2.54.0 在 Adapter 返回错误时保留所有原策略（RBAC 模型）")
	} else {
		t.Errorf("❌ 验证失败（RBAC 模型）：策略保留: %v, 角色继承保留: %v",
			policiesPreserved, groupingPoliciesPreserved)
	}
}
