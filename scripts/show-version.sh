#!/bin/bash
# Copyright 2025 The ChaosBlade Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# 版本信息展示脚本
# 用于快速查看当前项目的版本信息

set -e

echo "=== ChaosBlade Operator 版本信息 ==="

# 获取Git标签版本
GIT_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "无标签")
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
GO_VERSION=$(go version | awk '{print $3}')

echo "Git标签: $GIT_TAG"
echo "Git提交: $GIT_COMMIT"
echo "Git分支: $GIT_BRANCH"
echo "构建时间: $BUILD_TIME"
echo "Go版本: $GO_VERSION"
echo "平台: $(go env GOOS)/$(go env GOARCH)"

# 如果存在构建产物，显示其版本信息
if [ -f "target/chaosblade-*/bin/chaosblade-operator" ]; then
    echo ""
    echo "=== 构建产物版本信息 ==="
    target/chaosblade-*/bin/chaosblade-operator --version 2>/dev/null || echo "无法获取构建产物版本信息"
fi

echo ""
echo "=== 构建命令示例 ==="
echo "显示版本信息: make show-version"
echo "构建二进制: make build_binary"
echo "构建Docker镜像: make docker-build"
echo "完整构建: make build_all"
echo "======================"
