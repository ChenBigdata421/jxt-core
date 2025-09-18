package runtime

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/eventbus"
	"github.com/ChenBigdata421/jxt-core/storage"
	"github.com/casbin/casbin/v2"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type Runtime interface {
	// GetTenantID 获取租户id
	SetTenantMapping(host, tenantID string)
	GetTenantID(host string) string

	// SetDb 非CQRS时，多db设置，⚠️SetDbs不允许并发,可以根据自己的业务，例如app分库（用分表分库，或不同微服务实现）、host分库（多租户）
	SetTenantDB(tenantID string, db *gorm.DB)
	GetTenantDB(tenantID string) *gorm.DB
	GetTenantDBs(fn func(tenantID string, db *gorm.DB) bool)

	// SetTenantCommandDB CQRS时，设置对应租户的Command db
	SetTenantCommandDB(tenantID string, db *gorm.DB)
	GetTenantCommandDB(tenantID string) *gorm.DB
	GetTenantCommandDBs(fn func(tenantID string, db *gorm.DB) bool)

	// SetTenantQueryDB CQRS时，设置对应租户的Query db
	SetTenantQueryDB(tenantID string, db *gorm.DB)
	GetTenantQueryDB(tenantID string) *gorm.DB
	GetTenantQueryDBs(fn func(tenantID string, db *gorm.DB) bool)

	// SetTenantCasbin 设置对应租户的casbin
	SetTenantCasbin(tenantID string, enforcer *casbin.SyncedEnforcer)
	// GetTenantCasbin 根据租户id获取casbin
	GetTenantCasbin(tenantID string) *casbin.SyncedEnforcer
	// GetCasbins 获取所有casbin
	GetCasbins(fn func(tenantID string, enforcer *casbin.SyncedEnforcer) bool)

	// SetEngine 使用的路由
	SetEngine(engine http.Handler)
	GetEngine() http.Handler

	GetRouter() []Router

	// SetLogger 使用zap
	SetLogger(logger *zap.Logger)
	GetLogger() *zap.Logger

	// SetCrontab crontab
	SetTenantCrontab(tenantID string, crontab *cron.Cron)
	GetTenantCrontab(tenantID string) *cron.Cron
	GetCrontabs(fn func(tenantID string, crontab *cron.Cron) bool)

	// SetMiddleware middleware
	SetMiddleware(string, interface{})
	GetMiddleware() map[string]interface{}
	GetMiddlewareKey(key string) interface{}

	// SetCacheAdapter cache
	SetCacheAdapter(storage.AdapterCache)
	GetCacheAdapter() storage.AdapterCache
	GetCachePrefix(string) storage.AdapterCache

	GetMemoryQueue(string) storage.AdapterQueue
	SetQueueAdapter(storage.AdapterQueue)
	GetQueueAdapter() storage.AdapterQueue
	GetQueuePrefix(string) storage.AdapterQueue

	SetLockerAdapter(storage.AdapterLocker)
	GetLockerAdapter() storage.AdapterLocker
	GetLockerPrefix(string) storage.AdapterLocker

	SetHandler(key string, routerGroup func(r *gin.RouterGroup, hand ...*gin.HandlerFunc))
	GetHandler() map[string][]func(r *gin.RouterGroup, hand ...*gin.HandlerFunc)
	GetHandlerPrefix(key string) []func(r *gin.RouterGroup, hand ...*gin.HandlerFunc)

	GetStreamMessage(id, stream string, value map[string]interface{}) (storage.Messager, error)

	GetConfig(key string) interface{}
	SetConfig(key string, value interface{})

	// SetAppRouters set AppRouter
	SetAppRouters(appRouters func())
	GetAppRouters() []func()

	// SetEventBus 设置事件总线
	SetEventBus(eventbus.EventBus)
	// GetEventBus 获取事件总线
	GetEventBus() eventbus.EventBus
}
