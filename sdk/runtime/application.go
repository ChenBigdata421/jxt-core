package runtime

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	"github.com/ChenBigdata421/jxt-core/storage"
	"github.com/ChenBigdata421/jxt-core/storage/queue"
	"github.com/casbin/casbin/v2"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type Application struct {
	tenantDBs        sync.Map                                                        //租户数据库连接，非CQRS
	tenantServiceDBs sync.Map                                                        //服务级数据库连接（key: "tenantID:serviceCode"）
	casbins          sync.Map                                                        //casbin
	engine           http.Handler                                                    //路由引擎
	crontabs         sync.Map                                                        //crontab
	mux              sync.RWMutex                                                    //互斥锁
	middlewares      map[string]interface{}                                          //中间件
	cache            storage.AdapterCache                                            //缓存
	queue            storage.AdapterQueue                                            //队列
	locker           storage.AdapterLocker                                           //分布式锁
	memoryQueue      storage.AdapterQueue                                            //内存队列
	eventBus         eventbus.EventBus                                               //事件总线
	handler          map[string][]func(r *gin.RouterGroup, hand ...*gin.HandlerFunc) //handler
	routers          []Router                                                        //路由
	configs          map[string]interface{}                                          // 系统参数
	appRouters       []func()                                                        // app路由
}

type Router struct {
	HttpMethod, RelativePath, Handler string
}

type Routers struct {
	List []Router
}

// SetTenantDB 非CQRS时，设置租户数据库连接
func (e *Application) SetTenantDB(tenantID int, db *gorm.DB) {
	e.tenantDBs.Store(tenantID, db)
}

// GetTenantDB 向后兼容：优先返回 security-management 服务，否则返回默认 tenantDBs
func (e *Application) GetTenantDB(tenantID int) *gorm.DB {
	// 优先尝试服务级配置的 security-management
	if db := e.GetTenantServiceDB(tenantID, "security-management"); db != nil {
		return db
	}
	// 回退到旧的 tenantDBs
	if db, ok := e.tenantDBs.Load(tenantID); ok {
		return db.(*gorm.DB)
	}
	return nil
}

// GetTenantDBs 遍历所有租户数据库连接
// 使用举例，统计活跃的数据库连接数
// count := 0
//
//	app.GetTenantDBs(func(tenantID int, db *gorm.DB) bool {
//	    if db != nil {
//	        count++
//	    }
//	    return true
//	})
//
// fmt.Printf("活跃租户数量: %d\n", count)
func (e *Application) GetTenantDBs(fn func(tenantID int, db *gorm.DB) bool) {
	e.tenantDBs.Range(func(key, value interface{}) bool {
		return fn(key.(int), value.(*gorm.DB))
	})
}

// === 服务级数据库管理方法 ===

// SetTenantServiceDB 设置租户指定服务的数据库连接
func (e *Application) SetTenantServiceDB(tenantID int, serviceCode string, db *gorm.DB) {
	key := e.buildTenantServiceKey(tenantID, serviceCode)
	e.tenantServiceDBs.Store(key, db)
}

// GetTenantServiceDB 获取租户指定服务的数据库连接
func (e *Application) GetTenantServiceDB(tenantID int, serviceCode string) *gorm.DB {
	key := e.buildTenantServiceKey(tenantID, serviceCode)
	if db, ok := e.tenantServiceDBs.Load(key); ok {
		return db.(*gorm.DB)
	}
	return nil
}

// GetTenantServiceDBs 遍历所有租户服务数据库连接
func (e *Application) GetTenantServiceDBs(fn func(tenantID int, serviceCode string, db *gorm.DB) bool) {
	e.tenantServiceDBs.Range(func(key, value interface{}) bool {
		// key 格式: "tenantID:serviceCode"
		keyStr := key.(string)
		tenantID, serviceCode := e.parseTenantServiceKey(keyStr)
		return fn(tenantID, serviceCode, value.(*gorm.DB))
	})
}

// buildTenantServiceKey 构建租户服务缓存键
func (e *Application) buildTenantServiceKey(tenantID int, serviceCode string) string {
	return fmt.Sprintf("%d:%s", tenantID, serviceCode)
}

// parseTenantServiceKey 解析租户服务缓存键
func (e *Application) parseTenantServiceKey(key string) (int, string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return 0, ""
	}
	tenantID, _ := strconv.Atoi(parts[0])
	return tenantID, parts[1]
}

// SetTenantCasbin 设置对应租户的casbin
func (e *Application) SetTenantCasbin(tenantID int, enforcer *casbin.SyncedEnforcer) {
	e.casbins.Store(tenantID, enforcer)
}

// GetTenantCasbin 根据租户id获取casbin
func (e *Application) GetTenantCasbin(tenantID int) *casbin.SyncedEnforcer {
	if value, ok := e.casbins.Load(tenantID); ok {
		return value.(*casbin.SyncedEnforcer)
	}
	return nil
}

// GetCasbins 遍历所有租户casbin
func (e *Application) GetCasbins(fn func(tenantID int, enforcer *casbin.SyncedEnforcer) bool) {
	e.casbins.Range(func(key, value interface{}) bool {
		return fn(key.(int), value.(*casbin.SyncedEnforcer))
	})
}

// SetEngine 设置路由引擎
func (e *Application) SetEngine(engine http.Handler) {
	e.engine = engine
}

// GetEngine 获取路由引擎
func (e *Application) GetEngine() http.Handler {
	return e.engine
}

// GetRouter 获取路由表
func (e *Application) GetRouter() []Router {
	return e.setRouter()
}

// setRouter 设置路由表
func (e *Application) setRouter() []Router {
	switch e.engine.(type) {
	case *gin.Engine:
		routers := e.engine.(*gin.Engine).Routes()
		for _, router := range routers {
			e.routers = append(e.routers, Router{RelativePath: router.Path, Handler: router.Handler, HttpMethod: router.Method})
		}
	}
	return e.routers
}

// SetLogger 设置日志组件
func (e *Application) SetLogger(l *zap.Logger) {
	logger.Logger = l
}

// GetLogger 获取日志组件
func (e *Application) GetLogger() *zap.Logger {
	return logger.Logger
}

// NewConfig 默认值
func NewConfig() *Application {
	return &Application{
		tenantDBs:        sync.Map{},
		tenantServiceDBs: sync.Map{}, // 服务级数据库存储
		casbins:          sync.Map{},
		crontabs:         sync.Map{},
		middlewares:      make(map[string]interface{}),
		memoryQueue:      queue.NewMemory(10000),
		handler:          make(map[string][]func(r *gin.RouterGroup, hand ...*gin.HandlerFunc)),
		routers:          make([]Router, 0),
		configs:          make(map[string]interface{}),
	}
}

// SetCrontab 设置对应key的crontab
func (e *Application) SetTenantCrontab(tenantID int, crontab *cron.Cron) {
	e.crontabs.Store(tenantID, crontab)
}

// GetCrontabKey 根据key获取crontab
func (e *Application) GetTenantCrontab(tenantID int) *cron.Cron {
	if value, ok := e.crontabs.Load(tenantID); ok {
		return value.(*cron.Cron)
	}
	return nil
}

// GetCrontab 获取所有map里的crontab数据
func (e *Application) GetCrontabs(fn func(tenantID int, crontab *cron.Cron) bool) {
	e.crontabs.Range(func(key, value interface{}) bool {
		return fn(key.(int), value.(*cron.Cron))
	})
}

// SetMiddleware 设置中间件
func (e *Application) SetMiddleware(key string, middleware interface{}) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.middlewares[key] = middleware
}

// GetMiddleware 获取所有中间件
func (e *Application) GetMiddleware() map[string]interface{} {
	return e.middlewares
}

// GetMiddlewareKey 获取对应key的中间件
func (e *Application) GetMiddlewareKey(key string) interface{} {
	e.mux.Lock()
	defer e.mux.Unlock()
	return e.middlewares[key]
}

// SetCacheAdapter 设置缓存
func (e *Application) SetCacheAdapter(c storage.AdapterCache) {
	e.cache = c
}

// GetCacheAdapter 获取缓存
func (e *Application) GetCacheAdapter() storage.AdapterCache {
	return NewCache("", e.cache, "")
}

// GetCachePrefix 获取带租户标记的cache
func (e *Application) GetCachePrefix(key string) storage.AdapterCache {
	return NewCache(key, e.cache, "")
}

// SetQueueAdapter 设置队列适配器
func (e *Application) SetQueueAdapter(c storage.AdapterQueue) {
	e.queue = c
}

// GetQueueAdapter 获取队列适配器
func (e *Application) GetQueueAdapter() storage.AdapterQueue {
	return NewQueue("", e.queue)
}

// GetQueuePrefix 获取带租户标记的queue
func (e *Application) GetQueuePrefix(key string) storage.AdapterQueue {
	return NewQueue(key, e.queue)
}

// SetLockerAdapter 设置分布式锁
func (e *Application) SetLockerAdapter(c storage.AdapterLocker) {
	e.locker = c
}

// GetLockerAdapter 获取分布式锁
func (e *Application) GetLockerAdapter() storage.AdapterLocker {
	return NewLocker("", e.locker)
}

func (e *Application) GetLockerPrefix(key string) storage.AdapterLocker {
	return NewLocker(key, e.locker)
}

func (e *Application) SetHandler(key string, routerGroup func(r *gin.RouterGroup, hand ...*gin.HandlerFunc)) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.handler[key] = append(e.handler[key], routerGroup)
}

func (e *Application) GetHandler() map[string][]func(r *gin.RouterGroup, hand ...*gin.HandlerFunc) {
	e.mux.Lock()
	defer e.mux.Unlock()
	return e.handler
}

func (e *Application) GetHandlerPrefix(key string) []func(r *gin.RouterGroup, hand ...*gin.HandlerFunc) {
	e.mux.Lock()
	defer e.mux.Unlock()
	return e.handler[key]
}

// GetStreamMessage 获取队列需要用的message
func (e *Application) GetStreamMessage(id, stream string, value map[string]interface{}) (storage.Messager, error) {
	message := &queue.Message{}
	message.SetID(id)
	message.SetStream(stream)
	message.SetValues(value)
	return message, nil
}

func (e *Application) GetMemoryQueue(prefix string) storage.AdapterQueue {
	return NewQueue(prefix, e.memoryQueue)
}

// SetConfig 设置对应key的config
func (e *Application) SetConfig(key string, value interface{}) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.configs[key] = value
}

// GetConfig 获取对应key的config
func (e *Application) GetConfig(key string) interface{} {
	e.mux.Lock()
	defer e.mux.Unlock()
	return e.configs[key]
}

// SetAppRouters 设置app的路由
func (e *Application) SetAppRouters(appRouters func()) {
	e.appRouters = append(e.appRouters, appRouters)
}

// GetAppRouters 获取app的路由
func (e *Application) GetAppRouters() []func() {
	return e.appRouters
}

// SetEventBus 设置事件总线
func (e *Application) SetEventBus(eb eventbus.EventBus) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.eventBus = eb
}

// GetEventBus 获取事件总线
func (e *Application) GetEventBus() eventbus.EventBus {
	e.mux.RLock()
	defer e.mux.RUnlock()
	return e.eventBus
}
