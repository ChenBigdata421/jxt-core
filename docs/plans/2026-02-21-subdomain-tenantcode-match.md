# 子域名匹配租户代码（TenantCode）设计方案

## 背景

当前域名识别（host 类型）支持两种方式：
1. **精确域名匹配**：通过 `DomainLookuper.GetTenantIDByDomain()` 查询
2. **子域名提取**：从 Host 中提取子域名字符串返回，外层中间件通过 `strconv.Atoi` 校验是否为数字

**问题**：当子域名不是数字时（如 `acmeCorp.example.com`），`strconv.Atoi` 失败导致返回 400 错误，无法识别租户。

**需求**：如果子域名不是数字，则将子域名字符串与 tenantCode 做比较来识别租户。

## 设计方案

### 1. 新增独立接口 CodeLookuper（middleware/extract_tenant_id.go）

**设计原则**：不修改现有 `DomainLookuper` 接口，避免破坏性变更。使用类型断言实现向后兼容。

```go
// CodeLookuper 定义租户代码查找接口
// Provider 隐式实现此接口（鸭子类型）
// 使用独立接口而非扩展 DomainLookuper，避免破坏性变更
type CodeLookuper interface {
    GetTenantIDByCode(code string) (int, bool)
}
```

**兼容性说明**：
- `DomainLookuper` 接口保持不变，已实现该接口的类型无需修改
- `Provider` 同时实现 `DomainLookuper` 和 `CodeLookuper`
- 通过类型断言检查是否支持 Code 查找

### 2. Provider 添加 GetTenantIDByCode 方法（provider/provider.go）

**位置**：在 `GetTenantIDByDomain()` 方法附近

```go
// GetTenantIDByCode 通过租户代码获取租户ID
// 用于子域名匹配场景：subdomain.example.com -> 查找 code=subdomain 的租户
//
// 注意：DNS 子域名不区分大小写（RFC 4343），因此 code 查找也使用小写匹配
func (p *Provider) GetTenantIDByCode(code string) (int, bool) {
    if code == "" {
        return 0, false
    }

    data := p.data.Load().(*tenantData)

    // 使用 codeIndex 进行 O(1) 查找，与 domainIndex 模式一致
    tenantID, ok := data.codeIndex[strings.ToLower(code)]
    return tenantID, ok
}
```

### 3. 修改 tenantData 结构体（provider/provider.go）

```go
type tenantData struct {
    Metas       map[int]*TenantMeta                       `json:"metas"`
    Databases   map[int]map[string]*ServiceDatabaseConfig `json:"databases"`
    Ftps        map[int][]*FtpConfigDetail                `json:"ftps"`
    Storages    map[int]*StorageConfig                    `json:"storages"`
    Domains     map[int]*DomainConfig                     `json:"domains"`
    domainIndex map[string]int                            // domain -> tenantID（内嵌索引，不序列化）
    codeIndex   map[string]int                            // code(lowercase) -> tenantID（内嵌索引，不序列化）
}
```

### 4. 修改 buildDomainIndex 函数（provider/provider.go）

重命名为 `buildIndexes`，同时构建 domainIndex 和 codeIndex：

```go
// buildIndexes 构建 domainIndex 和 codeIndex
// domainIndex: domain(lowercase) -> tenantID
// codeIndex: code(lowercase) -> tenantID
func buildIndexes(data *tenantData) map[string]int {
    domainIndex := make(map[string]int)
    codeIndex := make(map[string]int)

    for tenantID, domain := range data.Domains {
        if domain.Primary != "" {
            domainIndex[strings.ToLower(domain.Primary)] = tenantID
        }
        for _, alias := range domain.Aliases {
            if alias != "" {
                domainIndex[strings.ToLower(alias)] = tenantID
            }
        }
        if domain.Internal != "" {
            domainIndex[strings.ToLower(domain.Internal)] = tenantID
        }
    }

    // 构建 codeIndex（小写规范化，符合 DNS 不区分大小写特性）
    for tenantID, meta := range data.Metas {
        if meta.Code != "" {
            codeIndex[strings.ToLower(meta.Code)] = tenantID
        }
    }

    data.domainIndex = domainIndex
    data.codeIndex = codeIndex
    return domainIndex // 保持返回值兼容现有调用
}
```

### 5. 修改 extractFromHost 函数（middleware/extract_tenant_id.go）

**位置**：第 240-278 行

```go
// extractFromHost extracts tenant ID from Host header
// Priority:
//  1. If domainLookup is configured, attempt exact domain match first
//  2. Fall back to subdomain extraction:
//     2a. If subdomain is numeric, use as tenant ID directly
//     2b. If subdomain is non-numeric and CodeLookuper available, match by tenant code
//
// Exact domain matching does not support wildcards.
// DNS is case-insensitive (RFC 4343), all lookups use lowercase.
func extractFromHost(c *gin.Context, lookup DomainLookuper) (string, bool) {
    host := c.Request.Host
    if host == "" {
        return "", false
    }

    // Remove port if present
    if idx := strings.Index(host, ":"); idx != -1 {
        host = host[:idx]
    }

    // 1. If domain lookup is configured, try exact domain match first
    if lookup != nil {
        if tenantID, ok := lookup.GetTenantIDByDomain(host); ok {
            return strconv.Itoa(tenantID), true
        }
    }

    // 2. Fall back to subdomain extraction
    parts := strings.Split(host, ".")
    if len(parts) < 2 {
        return "", false
    }

    subdomain := parts[0]
    if subdomain == "" || subdomain == "www" {
        return "", false
    }

    // 2a. If subdomain is numeric, return directly (e.g., "123.example.com" -> "123")
    if _, err := strconv.Atoi(subdomain); err == nil {
        return subdomain, true
    }

    // 2b. Subdomain is non-numeric, try to match by tenant code
    //     e.g., "acmeCorp.example.com" -> lookup by code "acmecorp" (lowercase)
    //     Uses type assertion for backward compatibility - only Provider supports this
    if lookup != nil {
        if codeLookup, ok := lookup.(CodeLookuper); ok {
            // DNS is case-insensitive, normalize to lowercase
            if tenantID, ok := codeLookup.GetTenantIDByCode(strings.ToLower(subdomain)); ok {
                return strconv.Itoa(tenantID), true
            }
        }
    }

    return "", false
}
```

## 识别优先级（修改后）

```
┌─────────────────────────────────────────────────────────────┐
│                    Host: AcmeCorp.example.com                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  优先级 1：精确域名匹配（小写规范化）                          │
│  lookup.GetTenantIDByDomain("acmecorp.example.com")         │
│  → 在 domainIndex 中查找完整域名                              │
└─────────────────────────────────────────────────────────────┘
                              │ 未找到
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  优先级 2a：数字子域名检查                                     │
│  strconv.Atoi("AcmeCorp") → 失败（非数字）                    │
└─────────────────────────────────────────────────────────────┘
                              │ 非数字
                              ▼
┌─────────────────────────────────────────────────────────────┐
│  优先级 2b：租户代码匹配（新增，小写规范化）                    │
│  类型断言：lookup.(CodeLookuper)                             │
│  codeLookup.GetTenantIDByCode("acmecorp")                   │
│  → 在 codeIndex 中查找 code="acmecorp" 的租户                │
│  → 找到则返回 tenantID                                        │
└─────────────────────────────────────────────────────────────┘
                              │ 未找到
                              ▼
                         返回 400 错误
```

## 使用场景对比

| 场景 | Host | 识别方式 | 结果 |
|------|------|----------|------|
| 数字子域名 | `123.example.com` | 2a. 数字直接返回 | tenantID=123 |
| 租户代码匹配 | `AcmeCorp.example.com` | 2b. Code 查找（小写） | 查找 code="acmecorp" |
| 精确域名 | `app.acme.com` | 1. 域名索引 | 域名配置的 tenantID |
| 无匹配 | `unknown.example.com` | - | 400 错误 |

## 修改文件清单

| 文件 | 修改位置 | 修改类型 | 说明 |
|------|----------|----------|------|
| `middleware/extract_tenant_id.go` | `DomainLookuper` 接口后 | 新增 | `CodeLookuper` 接口定义 |
| `middleware/extract_tenant_id.go` | `extractFromHost` 函数 | 修改 | 增加类型断言 + Code 匹配逻辑 |
| `provider/provider.go` | `tenantData` 结构体 | 修改 | 添加 `codeIndex` 字段 |
| `provider/provider.go` | `GetTenantIDByDomain` 附近 | 新增 | `GetTenantIDByCode()` 方法实现 |
| `provider/provider.go` | `buildDomainIndex` 函数 | 修改 | 重命名为 `buildIndexes`，同时构建 codeIndex |
| `provider/provider.go` | `LoadAll` 函数 | 修改 | 调用 `buildIndexes` |
| `provider/provider.go` | `handleWatchEvent` 函数 | 修改 | 调用 `buildIndexes`（重建索引） |
| `provider/provider.go` | `copyData` 函数 | 修改 | 注释说明 codeIndex 由调用方重建 |
| `middleware/extract_tenant_id_test.go` | `mockDomainLookuper` | 修改 | 添加 `GetTenantIDByCode` 方法 |
| `middleware/extract_tenant_id_test.go` | 现有测试用例 | 修改 | 更新 "Domain lookup fails and subdomain is non-numeric" 测试 |

## 测试方案

### 需要更新的现有测试

```go
// mockDomainLookuper 需要实现 CodeLookuper 接口
type mockDomainLookuper struct {
    domains map[string]int
    codes   map[string]int  // 新增
}

func (m *mockDomainLookuper) GetTenantIDByDomain(domain string) (int, bool) {
    tenantID, ok := m.domains[domain]
    return tenantID, ok
}

func (m *mockDomainLookuper) GetTenantIDByCode(code string) (int, bool) {
    tenantID, ok := m.codes[strings.ToLower(code)]
    return tenantID, ok
}
```

```go
// 更新现有测试：Domain lookup fails and subdomain is non-numeric
// 原期望：400 错误
// 新期望：如果 mock 配置了 code，则成功；否则 400
t.Run("Domain lookup fails but subdomain matches tenant code", func(t *testing.T) {
    mockLookup := &mockDomainLookuper{
        domains: map[string]int{"other.example.com": 1},
        codes:   map[string]int{"unknown": 2},  // 子域名 "unknown" 匹配 code
    }
    router.Use(ExtractTenantID(
        WithResolverType("host"),
        WithDomainLookup(mockLookup),
    ))
    req.Host = "unknown.example.com"
    // 期望：tenantID=2（通过 code 匹配）
})

t.Run("Domain lookup and code lookup both fail", func(t *testing.T) {
    mockLookup := &mockDomainLookuper{
        domains: map[string]int{},
        codes:   map[string]int{},  // 无匹配
    }
    router.Use(ExtractTenantID(
        WithResolverType("host"),
        WithDomainLookup(mockLookup),
    ))
    req.Host = "unknown.example.com"
    // 期望：400 错误
})
```

### 新增测试

```go
// TestGetTenantIDByCode - Provider 方法测试
// TestCodeLookuperTypeAssertion - 类型断言测试
// TestCodeLookupCaseInsensitive - 大小写不敏感测试
```

### 测试用例

```go
// 场景1：数字子域名
Host: "123.example.com"
期望: tenantID=123

// 场景2：租户代码匹配（新功能）
Metas: {1: {Code: "acmeCorp", ...}}
codeIndex: {"acmecorp": 1}
Host: "AcmeCorp.example.com"  // 大写
期望: tenantID=1  // 小写匹配成功

// 场景3：租户代码不匹配
codeIndex: {"othercorp": 1}
Host: "acmeCorp.example.com"
期望: 400 错误

// 场景4：无 lookup 时的数字子域名
Host: "456.example.com"
lookup: nil
期望: tenantID=456

// 场景5：无 lookup 时的非数字子域名
Host: "acmeCorp.example.com"
lookup: nil
期望: 400 错误（无法查找 Code）

// 场景6：lookup 不实现 CodeLookuper
Host: "acmeCorp.example.com"
lookup: mockDomainLookuper（无 GetTenantIDByCode 方法）
期望: 400 错误（类型断言失败）
```

## 兼容性说明

- **向后兼容**：
  - `DomainLookuper` 接口不变，已实现该接口的类型无需修改
  - 数字子域名的行为不变
  - Provider 自动获得 Code 查找能力
- **接口设计**：使用独立接口 + 类型断言，非破坏性扩展
- **性能**：O(1) codeIndex 查找，与 domainIndex 模式一致

## 设计决策记录

| 决策 | 选择 | 原因 |
|------|------|------|
| 接口扩展方式 | 独立接口 + 类型断言 | 避免破坏性变更 |
| codeIndex 默认启用 | 是 | 与 domainIndex 保持一致，避免 O(n) 查找 |
| 大小写处理 | 统一小写 | DNS 不区分大小写（RFC 4343） |
| nil 检查 | 不添加 | 与 GetTenantIDByDomain 保持一致 |

## 注意事项

1. **大小写规范化**：DNS 子域名不区分大小写，所有 code 查找统一转小写
2. **索引重建**：`handleWatchEvent` 中需要重建 codeIndex（与 domainIndex 同位置）
3. **类型断言失败**：非 Provider 的 lookup 实现不支持 Code 查找，会跳过 2b 步骤
