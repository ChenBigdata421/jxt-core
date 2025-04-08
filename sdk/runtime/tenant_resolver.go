package runtime

import (
	"strings"
	"sync"
)

// TenantResolver 使用 sync.Map 实现高并发租户解析
type TenantResolver struct {
	hostToTenantMap sync.Map
}

// NewTenantResolver 创建一个新的租户解析器
func NewTenantResolver() *TenantResolver {
	return &TenantResolver{}
}

// SetTenantMapping 添加或更新域名到租户ID的映射
func (tr *TenantResolver) SetTenantMapping(host, tenantID string) {
	// 存储时去除可能的端口号
	if idx := strings.IndexByte(host, ':'); idx != -1 {
		host = host[:idx]
	}
	tr.hostToTenantMap.Store(host, tenantID)
}

// GetTenantID 通过主机名获取租户ID
func (tr *TenantResolver) GetTenantID(host string) (string, bool) {
	// 处理可能的端口号
	if idx := strings.IndexByte(host, ':'); idx != -1 {
		host = host[:idx]
	}

	// 从sync.Map中检索
	val, exists := tr.hostToTenantMap.Load(host)
	if !exists {
		return "", false
	}

	tenantID, ok := val.(string)
	return tenantID, ok
}

// LoadFromMap 批量加载映射数据
func (tr *TenantResolver) LoadFromMap(mappings map[string]string) {
	for host, tenantID := range mappings {
		tr.SetTenantMapping(host, tenantID)
	}
}

// TenantMiddleware 创建HTTP中间件，从请求Host中提取租户ID
/*
func TenantMiddleware(resolver *TenantResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID, exists := resolver.GetTenantID(r.Host)
			if !exists {
				http.Error(w, "Tenant not found", http.StatusNotFound)
				return
			}

			// 将TenantID添加到请求上下文
			ctx := context.WithValue(r.Context(), "tenantID", tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
*/

/*
// 使用示例
func main() {
	resolver := NewTenantResolver()

	// 设置初始映射关系
	initialMappings := map[string]string{
		"tenant1.example.com": "t-001",
		"tenant2.example.com": "t-002",
		"app.tenant3.com":     "t-003",
	}
	resolver.LoadFromMap(initialMappings)

	// 也可以单独添加映射
	resolver.SetMapping("customer.saas-app.com", "t-004")

	// 创建HTTP服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tenantID := r.Context().Value("tenantID").(string)
		fmt.Fprintf(w, "Hello, Tenant: %s\n", tenantID)
	})

	// 包装HTTP处理器，添加租户解析中间件
	handler := TenantMiddleware(resolver)(mux)

	fmt.Println("Server running on :8080")
	http.ListenAndServe(":8080", handler)
}
*/
