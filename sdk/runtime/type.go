package runtime

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ChenBigdata421/jxt-core/logger"
	"github.com/ChenBigdata421/jxt-core/storage"
	"github.com/casbin/casbin/v2"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type Runtime interface {
	// SetDb 非CQRS时，多db设置，⚠️SetDbs不允许并发,可以根据自己的业务，例如app分库、host分库
	SetDb(key string, db *gorm.DB)
	GetDb() map[string]*gorm.DB
	GetDbByKey(key string) *gorm.DB

	// SetCommandDb CQRS时，设置对应key的db
	SetCommandDb(key string, db *gorm.DB)
	GetCommandDb() map[string]*gorm.DB
	GetCommandDbByKey(key string) *gorm.DB

	// SetQueryDb CQRS时，设置对应key的db
	SetQueryDb(key string, db *gorm.DB)
	GetQueryDb() map[string]*gorm.DB
	GetQueryDbByKey(key string) *gorm.DB

	SetCasbin(key string, enforcer *casbin.SyncedEnforcer)
	GetCasbin() map[string]*casbin.SyncedEnforcer
	GetCasbinKey(key string) *casbin.SyncedEnforcer

	// SetEngine 使用的路由
	SetEngine(engine http.Handler)
	GetEngine() http.Handler

	GetRouter() []Router

	// SetLogger 使用go-admin定义的logger，参考来源go-micro
	SetLogger(logger logger.Logger)
	GetLogger() logger.Logger

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
