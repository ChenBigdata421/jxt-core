package repotest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

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
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		db := mustOpen(t, gormmysql.Open, dsn+sep+"multiStatements=true")
		applyMigration(t, db, mysql.Migration())
		return db, func() {}
	}
	if testing.Short() || os.Getenv("RELIABLE_SKIP_CONTAINERS") != "" {
		t.Skip("mysql conformance needs RELIABLE_MYSQL_DSN or Docker")
	}
	ctx := context.Background()
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
	ctx := context.Background()
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
	db, err := gorm.Open(dialector(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
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
