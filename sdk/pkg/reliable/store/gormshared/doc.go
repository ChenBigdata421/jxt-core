// Package gormshared 是 reliable Store 的共享 GORM 实现（D17）。
//
// 只依赖 gorm.io/gorm（J2：不引任何数据库驱动）。portable GORM tag（type:bytes/datetime/json）
// 由 GORM 按方言翻译；精确 schema（DATETIME(3)/BYTEA/JSONB/partial index/CHECK）由
// store/mysql/migration.go 与 store/postgres/migration.go 的方言 SQL 负责——model 只管 ORM 映射。
//
// §3.3 独立提交保证（D16 + 本轮评审 F1/A3）：NewStore(db, classifier) 派生 claimDB = db.Session(&gorm.Session{NewDB:true}），
// TryClaim 用 claimDB 独立提交。**关键 GORM 语义（已在 gorm v1.24.2 核实）**：Session{NewDB:true} 只置 clone
// 标志、不改 ConnPool——claimDB 复用 db 的底层 ConnPool。db 为 pooled 时 Create 走池连接逐语句自动提交；db 为事务
// 句柄时 claimDB 继承 tx ConnPool、TryClaim 静默并入调用方事务，§3.3 三大失效（租约回收扫不到 / 行锁阻塞并发 /
// claim_id 校验形同虚设）全部复活。故 NewStore 在构造期用 ConnPool 类型断言拒绝 tx 句柄（panic）——构造期保证，
// 不再仅靠文档；store/repotest 的 confForbiddenCaseTxJoin 钉住该 guard 真的触发。
package gormshared
