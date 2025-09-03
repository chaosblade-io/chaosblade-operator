# ChaosBlade Operator 版本管理指南

本文档详细说明了 ChaosBlade Operator 的版本管理流程，包括 Git Tag 自动化构建、版本信息注入和发布流程。

## 概述

ChaosBlade Operator 现在支持完整的 Git Tag → 自动化构建 → 编译产物包含版本流程，确保每个构建产物都包含完整的版本信息。

## 版本信息组成

每个构建产物都包含以下版本信息：

- **版本号**: 基于 Git 标签的语义化版本
- **供应商**: community 或 aliyun
- **构建时间**: ISO 8601 格式的 UTC 时间
- **Git 提交**: 短哈希值
- **Git 分支**: 构建时的分支名
- **Go 版本**: 构建时使用的 Go 版本
- **平台**: 目标平台 (OS/Arch)

## 快速开始

### 1. 查看当前版本信息

```bash
# 使用脚本查看
./scripts/show-version.sh

# 使用 Makefile 查看
make show-version

# 查看构建产物版本
make build_binary
./target/chaosblade-*/bin/chaosblade-operator --version
```

### 2. 构建带版本信息的产物

```bash
# 构建基本组件
make build

# 构建二进制文件
make build_binary

# 构建 Docker 镜像
make docker-build
make docker-build-arm64

# 完整构建流程
make build_all
```

## 版本发布流程

### 1. 自动发布 (推荐)

当推送 Git 标签时，GitHub Actions 会自动触发构建：

```bash
# 创建新版本标签
git tag v1.8.0

# 推送标签 (触发自动化构建)
git push origin v1.8.0
```

### 2. 手动发布

使用发布脚本进行手动版本发布：

```bash
# 预览发布计划
./scripts/release.sh -d 1.8.0

# 创建版本标签
./scripts/release.sh 1.8.0

# 创建版本标签并自动推送
./scripts/release.sh -p 1.8.0

# 强制覆盖已存在的标签
./scripts/release.sh -f -p 1.8.0
```

### 3. 手动构建

```bash
# 指定版本和供应商
BLADE_VERSION=1.8.0 BLADE_VENDOR=community make build_all

# 构建特定平台
GOOS=linux GOARCH=amd64 make build_binary
```

## 环境变量配置

### 构建时环境变量

- `BLADE_VERSION`: 指定版本号 (默认: Git 标签)
- `BLADE_VENDOR`: 指定供应商 (默认: community)
- `GOOS`: 目标操作系统
- `GOARCH`: 目标架构

### 示例

```bash
# 构建社区版本 1.8.0
BLADE_VERSION=1.8.0 BLADE_VENDOR=community make build_all

# 构建阿里云版本 1.8.0
BLADE_VERSION=1.8.0 BLADE_VENDOR=aliyun make build_all

# 交叉编译 Linux AMD64
GOOS=linux GOARCH=amd64 make build_binary
```

## GitHub Actions 工作流

### 自动触发条件

- 推送 Git 标签 (格式: `v*`)
- 手动触发 (可指定版本和供应商)

### 构建产物

- Docker 镜像 (AMD64/ARM64)
- 二进制文件
- YAML 配置文件
- Helm Chart

### 自动发布

- 创建 GitHub Release
- 上传构建产物
- 更新 Docker 镜像标签

## 版本信息验证

### 1. 二进制文件版本信息

```bash
# 构建后查看版本信息
make build_binary
./target/chaosblade-*/bin/chaosblade-operator --version
```

### 2. Docker 镜像版本信息

```bash
# 查看镜像标签
docker images | grep chaosblade-operator

# 运行容器查看版本
docker run --rm ghcr.io/chaosblade-io/chaosblade-operator:1.8.0 --version
```

### 3. 版本信息 API

在代码中可以通过以下方式获取版本信息：

```go
import "github.com/chaosblade-io/chaosblade-operator/version"

// 获取完整版本信息
versionInfo := version.GetVersionInfo()

// 获取格式化的版本字符串
versionString := version.GetVersionString()

// 获取简短版本
shortVersion := version.GetShortVersion()
```

## 故障排除

### 常见问题

1. **版本信息显示为 "unknown"**
   - 检查是否正确设置了 `BLADE_VERSION` 环境变量
   - 确认 Git 仓库中有标签

2. **构建失败**
   - 检查 Go 版本兼容性
   - 确认所有依赖已安装

3. **Docker 构建失败**
   - 检查 Docker 服务状态
   - 确认有足够的磁盘空间

### 调试命令

```bash
# 显示详细构建信息
make show-version

# 清理构建产物
make clean

# 查看帮助信息
make help
```

## 最佳实践

### 1. 版本号规范

- 使用语义化版本 (SemVer)
- 格式: `MAJOR.MINOR.PATCH`
- 预发布版本: `1.8.0-beta.1`
- 构建元数据: `1.8.0+build.123`

### 2. 发布流程

- 在 `main` 或 `master` 分支上发布
- 确保所有更改已提交
- 使用发布脚本自动化流程
- 及时更新 CHANGELOG.md

### 3. 标签管理

- 使用 `v` 前缀 (如 `v1.8.0`)
- 避免删除已发布的标签
- 使用强制覆盖时要谨慎

## 相关文件

- `version/version.go`: 版本信息定义
- `Makefile`: 构建配置
- `.github/workflows/release-build.yml`: 自动化构建工作流
- `scripts/release.sh`: 版本发布脚本
- `scripts/show-version.sh`: 版本信息展示脚本
- `CHANGELOG.md`: 变更日志

## 支持

如果遇到问题，请：

1. 查看本文档的故障排除部分
2. 检查 GitHub Actions 构建日志
3. 提交 Issue 描述问题
4. 联系维护团队

---

*最后更新: 2024年*
