package repotest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store/mysql"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/reliable/store/postgres"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	gormmysql "gorm.io/driver/mysql"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Dialect string

const (
	DialectMySQL    Dialect = "mysql"
	DialectPostgres Dialect = "postgres"
)

// Setup 为指定方言准备干净库 + 建表，返回 *gorm.DB 与 cleanup。
// 优先 env DSN（CI 预拉容器），回退 testcontainers，再无则 t.Skip。
func Setup(t *testing.T, dialect Dialect) (*gorm.DB, func()) {
	t.Helper()
	switch dialect {
	case DialectMySQL:
		return setupMySQL(t)
	case DialectPostgres:
		return setupPostgres(t)
	default:
		t.Fatalf("unknown dialect %s", dialect)
		return nil, nil
	}
}

func setupMySQL(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	if dsn := os.Getenv("RELIABLE_MYSQL_DSN"); dsn != "" {
		// multiStatements 是必需的（CreateTableSQL 是多语句）。
		// parseTime + loc=UTC 是必需的：go-sql-driver/mysql 默认把 DATETIME 返回为
		// []uint8，而 kernel 会把 created_at/updated_at 等列 Scan 进 *time.Time，
		// 缺 parseTime 会触发 "unsupported Scan, storing driver.Value type []uint8
		// into *time.Time"。仅在用户未显式设置时追加，尊重显式覆盖。
		dsn = ensureMySQLParam(dsn, "multiStatements=true")
		dsn = ensureMySQLParam(dsn, "parseTime=true")
		dsn = ensureMySQLParam(dsn, "loc=UTC")
		db := mustOpen(t, gormmysql.Open, dsn)
		applyMigration(t, db, mysql.Migration())
		return db, func() {}
	}
	if testing.Short() || os.Getenv("RELIABLE_SKIP_CONTAINERS") != "" {
		t.Skip("mysql conformance needs RELIABLE_MYSQL_DSN or Docker")
	}
	// 90s 上限：缓存镜像启动 ~30-60s 够用；Docker daemon 不可达时跳过而非挂死 ~15min。
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, err := tcmysql.Run(ctx, "mysql:8.0",
		tcmysql.WithDatabase("reliable_test"),
		tcmysql.WithUsername("test"),
		tcmysql.WithPassword("test"))
	if err != nil {
		t.Skipf("mysql container unavailable: %v (set RELIABLE_MYSQL_DSN)", err)
	}
	dsn, _ := c.ConnectionString(ctx, "charset=utf8mb4", "parseTime=True", "loc=UTC", "multiStatements=true")
	db := mustOpen(t, gormmysql.Open, dsn)
	applyMigration(t, db, mysql.Migration())
	return db, func() { _ = c.Terminate(context.Background()) }
}

func setupPostgres(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	if dsn := os.Getenv("RELIABLE_PG_DSN"); dsn != "" {
		db := mustOpen(t, gormpg.Open, dsn)
		applyMigration(t, db, postgres.Migration())
		return db, func() {}
	}
	if testing.Short() || os.Getenv("RELIABLE_SKIP_CONTAINERS") != "" {
		t.Skip("pg conformance needs RELIABLE_PG_DSN or Docker")
	}
	// 90s 上限：缓存镜像启动 ~30-60s 够用；Docker daemon 不可达时跳过而非挂死 ~15min。
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("reliable_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"))
	if err != nil {
		t.Skipf("pg container unavailable: %v (set RELIABLE_PG_DSN)", err)
	}
	dsn, _ := c.ConnectionString(ctx, "sslmode=disable", "timezone=UTC")
	db := mustOpen(t, gormpg.Open, dsn)
	applyMigration(t, db, postgres.Migration())
	return db, func() { _ = c.Terminate(context.Background()) }
}

func mustOpen(t *testing.T, dialector func(string) gorm.Dialector, dsn string) *gorm.DB {
	t.Helper()
	// testcontainers 的就绪信号（尤其 postgres:16-alpine——init 期间会重启并提前吐一次
	// "ready to accept connections" 日志）可能在 DB 真正接受连接前就 fire，首个 gorm.Open
	// 偶发 "unexpected EOF"。重试到连上为止（上限 30s，延续 77b4105 的有界启动哲学）。
	var (
		db  *gorm.DB
		err error
	)
	for attempt := 0; attempt < 60; attempt++ {
		db, err = gorm.Open(dialector(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("open gorm after retries: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(20)
	return db
}

// applyMigration 先跑 down（方言自带的 DropTableSQL）再跑 up。
// C4：原稿用裸 `DROP TABLE ... CASCADE` 而不是方言的 DropTableSQL，于是 DropTableSQL
// 写了却从未被执行过。现在每次 Setup 都跑一遍真实的 down 路径，down 不再是死代码；
// 并且重复调用 mig 验证 up 的幂等性（CREATE INDEX 必须带 IF NOT EXISTS）。
func applyMigration(t *testing.T, db *gorm.DB, mig func(*gorm.DB) error) {
	t.Helper()
	_ = db.Exec(dropSQLFor(db)).Error
	if err := mig(db); err != nil {
		t.Fatalf("apply migration (up): %v", err)
	}
	// up 幂等性：重复执行不得报错（CREATE TABLE/INDEX 均须带 IF NOT EXISTS）。
	if err := mig(db); err != nil {
		t.Fatalf("apply migration (up, second run — migration is not idempotent): %v", err)
	}
}

// dropSQLFor 按方言返回该方言自己的 DropTableSQL（migration down）。
func dropSQLFor(db *gorm.DB) string {
	if db.Dialector.Name() == "postgres" {
		return postgres.DropTableSQL
	}
	return mysql.DropTableSQL
}

// NewStoreFor 返回该方言的 store.Store + QuarantineStore（薄包 NewStore 注入 classifier）。
func NewStoreFor(dialect Dialect, db *gorm.DB) (store.Store, store.QuarantineStore) {
	switch dialect {
	case DialectMySQL:
		return mysql.NewStore(db), mysql.NewQuarantineStore(db)
	case DialectPostgres:
		return postgres.NewStore(db), postgres.NewQuarantineStore(db)
	}
	panic(fmt.Sprintf("unknown dialect %s", dialect))
}

// ensureMySQLParam 在 MySQL DSN 上确保 key=value 存在（用户未显式写则追加）。
// 既要保留 ? vs & 的分隔符选择（DSN 已有 ? 时用 &），又要避免重复追加同名 key。
// 仅做朴素的子串匹配：用户写法可能多种（parseTime / parseTime=True / parseTime=1），
// 这里按 key= 前缀判断，若存在任何形如 "<key>=" 的子串即视为用户已显式设置，不再追加。
func ensureMySQLParam(dsn, param string) string {
	key := strings.SplitN(param, "=", 2)[0]
	if strings.Contains(dsn, key+"=") {
		return dsn // 用户已显式设置该参数，尊重覆盖。
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + param
}
