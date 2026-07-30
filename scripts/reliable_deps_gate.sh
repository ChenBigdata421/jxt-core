#!/usr/bin/env bash
# PR-2 reliable 包四门禁：J2 依赖卫生 / M14 无 context key / §3.3 TryClaim 无外部事务 / D9 无占位符。
#
# 本轮评审（B4）：本脚本是 **CI 兜底**，Windows 开发机没有 grep/chmod，本地一律跑
# `go test ./sdk/pkg/reliable/ -run TestGate_`（Go 原生实现，跨平台且自带自测）。
# 两边的占位符模式必须保持一致，改一边时另一边同步。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "== J2: kernel root zero deps =="
if go list -deps ./sdk/pkg/reliable | grep -E 'gorm\.io|prometheus|gin-gonic|IBM/sarama|nats-io'; then
  echo "FAIL: kernel root imports banned deps"; exit 1
fi
echo "== J2: gormshared only gorm =="
if go list -deps ./sdk/pkg/reliable/store/gormshared | grep -E 'prometheus|gin-gonic|gorm.io/driver/(mysql|postgres)|sarama'; then
  echo "FAIL: gormshared has driver/prometheus/gin deps"; exit 1
fi
echo "== J2: store/mysql only gorm+mysql =="
if go list -deps ./sdk/pkg/reliable/store/mysql | grep -E 'prometheus|gin-gonic|gorm.io/driver/postgres|sarama'; then
  echo "FAIL: store/mysql has banned deps"; exit 1
fi
echo "== J2: store/postgres only gorm+pg =="
if go list -deps ./sdk/pkg/reliable/store/postgres | grep -E 'prometheus|gin-gonic|gorm.io/driver/mysql|sarama'; then
  echo "FAIL: store/postgres has banned deps"; exit 1
fi
echo "== M14: no context.WithValue =="
# --exclude=gates_test.go 与 Go 版 scanReliable 的跳过逻辑对齐：门禁文件自身承载词表，
# 不排除会让门禁永远自报红（「门禁抓门禁自己」）。
if grep -rn --exclude=gates_test.go "context.WithValue" sdk/pkg/reliable; then echo "FAIL: M14 violation"; exit 1; fi
echo "== §3.3: TryClaim takes no *gorm.DB =="
if grep -rnE --exclude=gates_test.go 'TryClaim\(.*gorm\.DB' sdk/pkg/reliable; then echo "FAIL: §3.3 violation"; exit 1; fi
echo "== D9: no placeholders =="
# 模式与 gates_test.go 的 placeholderRe 逐字对齐（D21：扩到能抓「实施时」与假类型）。
if grep -rnE --exclude=gates_test.go '实施时|TBD|TODO|FIXME|DATEADD\(|fill in|= interface\{\}|stubDB' sdk/pkg/reliable; then
  echo "FAIL: D9 placeholder violation"; exit 1
fi
echo "ALL GATES GREEN"
