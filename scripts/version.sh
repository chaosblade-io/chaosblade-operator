#!/bin/bash

# 获取版本信息的脚本
# 用于在构建时注入版本信息到二进制文件中

set -e

# 获取Git标签版本
get_git_version() {
    # 优先使用最新的tag
    local git_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
    # 移除v前缀
    echo "${git_tag#v}"
}

# 获取Git提交哈希
get_git_commit() {
    git rev-parse --short HEAD 2>/dev/null || echo "unknown"
}

# 获取Git分支
get_git_branch() {
    git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown"
}

# 获取构建时间
get_build_time() {
    date -u '+%Y-%m-%dT%H:%M:%SZ'
}

# 获取Go版本
get_go_version() {
    go version | awk '{print $3}'
}

# 获取平台信息
get_platform() {
    echo "$(go env GOOS)/$(go env GOARCH)"
}

# 主函数
main() {
    local version=$(get_git_version)
    local commit=$(get_git_commit)
    local branch=$(get_git_branch)
    local build_time=$(get_build_time)
    local go_version=$(get_go_version)
    local platform=$(get_platform)
    
    # 输出版本信息，用于Makefile中的ldflags
    echo "VERSION=$version"
    echo "GIT_COMMIT=$commit"
    echo "GIT_BRANCH=$branch"
    echo "BUILD_TIME=$build_time"
    echo "GO_VERSION=$go_version"
    echo "PLATFORM=$platform"
}

# 如果直接运行此脚本，则输出所有版本信息
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main
fi
