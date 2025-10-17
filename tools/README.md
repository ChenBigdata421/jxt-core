# go-admin-core 迁移工具

## v1.8.0 迁移脚本

### 功能说明

`migrate-v1.8.sh` 脚本用于自动将项目代码从 go-admin-core v1.7.x 迁移到 v1.8.0。

主要变更:
- `sdk/pkg/captcha` → `captcha`
- `sdk/pkg/jwtauth` → `jwtauth`
- `sdk/pkg/response` → `response`
- `sdk/pkg/casbin` → `casbin`
- `observability/audit` → `observe/audit`
- `tools/gorm/logger` → `tools/gorm/gormlog`

### 使用方法

#### 1. 在您的项目根目录运行

```bash
# 下载脚本
curl -O https://raw.githubusercontent.com/go-admin-team/go-admin-core/dev/tools/migrate-v1.8.sh

# 或者如果已经在 go-admin-core 项目中
cp /path/to/go-admin-core/tools/migrate-v1.8.sh .

# 添加执行权限
chmod +x migrate-v1.8.sh

# 运行迁移
./migrate-v1.8.sh
```

#### 2. 对于 go-admin-pro 项目

```bash
cd /path/to/go-admin-pro
bash /path/to/go-admin-core/tools/migrate-v1.8.sh
```

### 脚本功能

1. **环境检查**
   - 验证是否在 Go 项目根目录
   - 检查是否依赖 go-admin-core

2. **自动备份**
   - 在父目录创建带时间戳的备份
   - 排除 vendor、.git、node_modules

3. **智能扫描**
   - 查找所有使用旧导入路径的 .go 文件
   - 显示将要修改的文件列表

4. **安全替换**
   - 自动替换所有导入路径
   - 保留代码格式和注释

5. **验证测试**
   - 运行 `go mod tidy`
   - 尝试编译项目

6. **生成报告**
   - 创建详细的迁移报告
   - 记录所有变更

### 手动迁移

如果您不想使用自动脚本，可以手动执行以下步骤:

#### 1. 更新 go.mod

```bash
go get github.com/go-admin-team/go-admin-core@v1.8.0-beta
go mod tidy
```

#### 2. 批量替换导入路径

使用 IDE 的"查找并替换"功能（确保启用正则表达式）:

```
查找: github\.com/go-admin-team/go-admin-core/sdk/pkg/captcha
替换: github.com/go-admin-team/go-admin-core/captcha

查找: github\.com/go-admin-team/go-admin-core/sdk/pkg/jwtauth
替换: github.com/go-admin-team/go-admin-core/jwtauth

查找: github\.com/go-admin-team/go-admin-core/sdk/pkg/response
替换: github.com/go-admin-team/go-admin-core/response

查找: github\.com/go-admin-team/go-admin-core/sdk/pkg/casbin
替换: github.com/go-admin-team/go-admin-core/casbin
```

#### 3. 验证更改

```bash
# 格式化代码
go fmt ./...

# 整理依赖
go mod tidy

# 编译测试
go build ./...

# 运行测试
go test ./...
```

### 恢复备份

如果迁移后出现问题，可以恢复备份:

```bash
# 查看备份目录
ls -la ../backup_before_v1.8_*

# 恢复备份（替换 TIMESTAMP 为实际时间戳）
rm -rf ./*
cp -r ../backup_before_v1.8_TIMESTAMP/* ./
```

### 兼容性说明

- ✅ **v1.8.0 - v1.9.x**: 新旧路径都可用，旧路径会有 deprecated 警告
- ⚠️ **v2.0.0**: 旧路径将被移除，必须使用新路径

### 常见问题

#### Q: 脚本会修改哪些文件？

A: 只修改导入了 `sdk/pkg/{captcha,jwtauth,response,casbin}` 的 `.go` 文件，不会修改其他文件。

#### Q: 是否安全？

A: 脚本会在修改前创建完整备份，所有替换都使用精确的路径匹配，不会误改其他代码。

#### Q: 如果编译失败怎么办？

A: 
1. 检查 `git diff` 看具体变更
2. 恢复备份目录
3. 手动迁移问题文件
4. 提交 issue 到 GitHub

#### Q: 需要更新依赖版本吗？

A: 是的，建议更新到 v1.8.0-beta 或更高版本:
```bash
go get github.com/go-admin-team/go-admin-core@v1.8.0-beta
```

### 获取帮助

- 📖 [完整迁移文档](../../docs/migration/v1.8.0-plan.md)
- 🐛 [提交 Issue](https://github.com/go-admin-team/go-admin-core/issues)
- 💬 [讨论区](https://github.com/go-admin-team/go-admin-core/discussions)

### 示例输出

```
================================================
  go-admin-core v1.8.0 自动迁移工具
================================================

[INFO] 检测到 Go 项目: github.com/go-admin-team/go-admin-pro
[SUCCESS] 检测到 go-admin-core 依赖
[INFO] 创建备份: backup_before_v1.8_20251017_150000
[SUCCESS] 备份完成: ../backup_before_v1.8_20251017_150000
[INFO] 扫描需要更新的文件...
[INFO] 找到以下文件需要更新:
  - app/admin/apis/captcha.go
  - common/middleware/init.go
  - common/storage/initialize.go
  ...
[INFO] 开始更新导入路径...
[INFO] 处理文件: app/admin/apis/captcha.go
[SUCCESS] 已更新 15 个文件
[SUCCESS] go mod tidy 成功
[SUCCESS] 项目编译成功
[SUCCESS] 报告已生成: migration_report_20251017_150000.txt

================================================
[SUCCESS] 迁移完成！
================================================

[INFO] 下一步:
  1. 检查变更: git diff
  2. 运行测试: go test ./...
  3. 提交变更: git commit -am 'chore: migrate to go-admin-core v1.8.0'
```
