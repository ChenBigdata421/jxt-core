#!/usr/bin/env bash
# 单模块门禁：强制 jxt-core 永远只作为单一 Go 模块发布，禁止 sdk/ 等子目录独立发布。
#
# 背景：本仓库自 go-admin-core fork 继承过一条错误 tag 线（v0.1、v1.0.2、v1.2.0–v1.6.5
# + sdk/* + plugins/logger/zap/*），其 go.mod 声明的是 matchstalk / go-admin-team 的路径，
# 既非本模块的合法发布，版本号又高于正规线，导致未 pin 的 `go mod tidy` 误选坏 tag（如
# sdk@v1.5.2）而硬失败（详见 README 版本历史 v1.7.0）。v1.7.0 已清理这批 tag；本门禁负责
# 把它们挡在门外、不让其回来。
#
# 三道检查（任一失败即红）：
#   1. 全树有且仅有一个 go.mod（根模块）——任何嵌套 go.mod 都会让子目录变成独立可发布模块
#      （即使不打 <dir>/v* tag，也能以 pseudo-version @latest 被解析）。
#   2. 不存在嵌套模块 tag——根 tag 一律是裸 `v*`，任何含 `/` 的 tag（如 sdk/v1.x）均为
#      独立子模块发布，禁止。
#   3. 每个现存 tag 的根 go.mod 必须声明 github.com/ChenBigdata421/jxt-core——
#      直接拦截「打了一个路径不符的 tag」这一类回归（正是本次事故的根因）。
#
# CI 兜底：见 .github/workflows/module-hygiene.yml（checkout 必须 fetch-depth:0 以取全量 tag）。
# 本地：git bash / Linux 直接 `bash scripts/single_module_gate.sh`（纯 git/find/grep/awk，无需 Go）。
#
# 注意：本门禁在「坏 tag 仍存在」时必然变红——v1.7.0 清理之前勿单独提交本门禁，
# 应与 tag 清理 + v1.7.0 同批落地，否则 master CI 红。
# 远见：若将来做 v2 主版本（module path 改 …/jxt-core/v2），需同步放宽检查 3 的期望路径。
set -euo pipefail
cd "$(dirname "$0")/.."

EXPECTED_MODULE='github.com/ChenBigdata421/jxt-core'
failed=0

echo "== 检查 1：全树仅一个 go.mod（根模块）=="
nested=$(find . -name go.mod -not -path './.git/*' | grep -vx './go.mod' || true)
if [ -n "$nested" ]; then
  echo "FAIL: 发现嵌套 go.mod（会让子目录变成独立可发布模块）："
  echo "$nested"
  failed=1
else
  echo "ok"
fi

echo "== 检查 2：无嵌套模块 tag（根 tag 必须是裸 v*，不得含 /）=="
slash_tags=$(git tag -l '*/*' || true)
if [ -n "$slash_tags" ]; then
  echo "FAIL: 发现含 / 的 tag（独立子模块发布）："
  echo "$slash_tags"
  failed=1
else
  echo "ok"
fi

echo "== 检查 3：每个 tag 的根 go.mod 必须声明 $EXPECTED_MODULE =="
bad_paths=""
for tag in $(git tag -l); do
  mod=$(git show "$tag:go.mod" 2>/dev/null | awk '/^module /{print $2; exit}' || true)
  if [ "$mod" != "$EXPECTED_MODULE" ]; then
    bad_paths="$bad_paths
  $tag -> ${mod:-<无 go.mod>}"
  fi
done
if [ -n "$bad_paths" ]; then
  echo "FAIL: 存在 module path 不符的 tag（正是本次事故根因）："
  echo "$bad_paths"
  failed=1
else
  echo "ok"
fi

if [ "$failed" -ne 0 ]; then
  echo "GATE RED：jxt-core 单模块不变量被破坏"
  exit 1
fi
echo "ALL GATES GREEN：jxt-core 为单一模块，无独立子模块发布"
