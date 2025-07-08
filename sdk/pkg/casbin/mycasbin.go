package mycasbin

import (
	"sync"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/log"
	"github.com/casbin/casbin/v2/model"
	redisWatcher "github.com/go-admin-team/redis-watcher/v2"
	"github.com/go-redis/redis/v9"
	"gorm.io/gorm"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"
	gormAdapter "github.com/go-admin-team/gorm-adapter/v3"
)

// Initialize the model from a string.
var text = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && (keyMatch2(r.obj, p.obj) || keyMatch(r.obj, p.obj)) && (r.act == p.act || p.act == "*")
`

var (
	enforcer *casbin.SyncedEnforcer //策略执行器实例
	once     sync.Once
)

// 表名前缀为sys
// 策略表名为casbin_rule
// sys_casbin_rule存放访问控制策略
func Setup(db *gorm.DB, _ string) *casbin.SyncedEnforcer {
	once.Do(func() {
		Apter, err := gormAdapter.NewAdapterByDBUseTableName(db, "sys", "casbin_rule")
		if err != nil && err.Error() != "invalid DDL" {
			panic(err)
		}
		//权限模型加载
		m, err := model.NewModelFromString(text)
		if err != nil {
			panic(err)
		}
		//创建支持并发的执行器实例
		enforcer, err = casbin.NewSyncedEnforcer(m, Apter)
		if err != nil {
			panic(err)
		}
		//加载数据库中的策略到内存
		err = enforcer.LoadPolicy()
		if err != nil {
			panic(err)
		}
		// set redis watcher if redis config is not nil
		if config.CacheConfig.Redis != nil {
			w, err := redisWatcher.NewWatcher(config.CacheConfig.Redis.Addr, redisWatcher.WatcherOptions{
				Options: redis.Options{
					Network:  "tcp",
					Password: config.CacheConfig.Redis.Password,
				},
				Channel:    "/casbin",
				IgnoreSelf: false,
			})
			if err != nil {
				panic(err)
			}

			err = w.SetUpdateCallback(updateCallback)
			if err != nil {
				panic(err)
			}
			err = enforcer.SetWatcher(w)
			if err != nil {
				panic(err)
			}
		}

		log.SetLogger(&Logger{})
		enforcer.EnableLog(true)
	})

	return enforcer
}

func updateCallback(msg string) {
	//l := logger.NewHelper(sdk.Runtime.GetLogger())//废弃原有logger
	logger.Infof("casbin updateCallback msg: %v", msg) //使用sdk/pkg/logger
	err := enforcer.LoadPolicy()
	if err != nil {
		logger.Errorf("casbin LoadPolicy err: %v", err) //使用sdk/pkg/logger
	}
}
