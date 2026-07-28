// Package mysql 是 reliable Store 的 MySQL 薄方言包（D17）。
// 仅提供：migration SQL（精确 schema）、driver classifier（errors.As *mysql.MySQLError）、
// NewStore(db) 注入 classifier 到 gormshared。GORM 逻辑全部在 store/gormshared。
package mysql
