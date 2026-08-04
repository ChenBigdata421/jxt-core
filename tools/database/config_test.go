package database

import (
	"fmt"
	"sync/atomic"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

var sqliteOpenCounter atomic.Int64

// sqliteOpen returns a CGO-free in-memory sqlite dialector for Init()'s `open` parameter.
// The label is folded into a unique in-memory DSN so the main DB, each source, and each
// replica get isolated pools (cache=shared scopes a DB to its DSN). This lets the test
// exercise Init()'s resolver/pool orchestration WITHOUT a running MySQL server or a valid
// DSN — dsn0/dsn1 are now opaque labels, no longer parsed by the mysql driver.
func sqliteOpen(label string) gorm.Dialector {
	id := sqliteOpenCounter.Add(1)
	return sqlite.Dialector{DriverName: "sqlite", DSN: fmt.Sprintf("file:memdb_%s_%d?mode=memory&cache=shared", label, id)}
}

var dsn0 = "dsn0"
var dsn1 = "dsn1"
var tables = []interface{}{"sys_user", "sys_role"}

func TestDBConfig_Init(t *testing.T) {
	type fields struct {
		dsn             string
		connMaxIdleTime int
		connMaxLifetime int
		maxIdleConns    int
		maxOpenConns    int
		registers       []ResolverConfigure
	}
	type args struct {
		config *gorm.Config
		open   func(string) gorm.Dialector
	}
	registers := make([]ResolverConfigure, 0)
	registers = append(registers, &DBResolverConfig{
		sources:  []string{dsn0},
		replicas: []string{dsn1},
		policy:   "random",
		tables:   tables,
	})
	registers = append(registers, &DBResolverConfig{
		sources:  []string{dsn0},
		replicas: []string{dsn1},
		policy:   "random",
		tables:   tables,
	})
	registers = append(registers, &DBResolverConfig{
		sources:  []string{dsn0},
		replicas: []string{dsn1},
		policy:   "random",
		//tables:   tables,
	})
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			"test0",
			fields{
				dsn: dsn0,
			},
			args{
				config: &gorm.Config{},
				open:   sqliteOpen,
			},
			false,
		},
		{
			"test1",
			fields{
				dsn:             dsn0,
				connMaxIdleTime: 600,
				connMaxLifetime: 60,
				maxIdleConns:    200,
				maxOpenConns:    100,
				registers:       registers,
			},
			args{
				config: &gorm.Config{},
				open:   sqliteOpen,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &DBConfig{
				dsn:             tt.fields.dsn,
				connMaxIdleTime: tt.fields.connMaxIdleTime,
				connMaxLifetime: tt.fields.connMaxLifetime,
				maxIdleConns:    tt.fields.maxIdleConns,
				maxOpenConns:    tt.fields.maxOpenConns,
				registers:       tt.fields.registers,
			}
			_, err := e.Init(tt.args.config, tt.args.open)
			if (err != nil) != tt.wantErr {
				t.Errorf("Init() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
