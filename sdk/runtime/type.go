package runtime

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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

	SetCasbin(key string, enforcer *casbin.SyncedEnforcer)
	GetCasbin() map[string]*casbin.SyncedEnforcer
	GetCasbinKey(key string) *casbin.SyncedEnforcer

	// SetEngine 使用的路由
	SetEngine(engine http.Handler)
	GetEngine() http.Handler

	GetRouter() []Router

	// SetLogger 使用zap
	SetLogger(logger *zap.Logger)
	GetLogger() *zap.Logger

	// SetCrontab crontab
	SetCrontab(key string, crontab *cron.Cron)
	GetCrontab() map[string]*cron.Cron
	GetCrontabKey(key string) *cron.Cron

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
}
