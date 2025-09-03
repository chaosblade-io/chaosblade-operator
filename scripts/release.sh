#!/bin/bash

# 自动化版本发布脚本
# 用于创建新版本标签并触发自动化构建

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 显示帮助信息
show_help() {
    echo "用法: $0 [选项] <版本号>"
    echo ""
    echo "选项:"
    echo "  -h, --help     显示此帮助信息"
    echo "  -d, --dry-run  仅显示将要执行的命令，不实际执行"
    echo "  -f, --force    强制覆盖已存在的标签"
    echo "  -p, --push     自动推送到远程仓库"
    echo ""
    echo "版本号格式: 例如 1.8.0, 2.0.0-beta.1"
    echo ""
    echo "示例:"
    echo "  $0 1.8.0              # 创建版本 1.8.0"
    echo "  $0 -p 1.8.0           # 创建版本并推送"
    echo "  $0 -d 1.8.0           # 预览将要执行的命令"
}

# 检查Git状态
check_git_status() {
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        echo -e "${RED}错误: 当前目录不是Git仓库${NC}"
        exit 1
    fi
    
    # 检查是否有未提交的更改
    if [ -n "$(git status --porcelain)" ]; then
        echo -e "${YELLOW}警告: 有未提交的更改${NC}"
        git status --short
        echo ""
        read -p "是否继续? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
    
    # 检查是否在正确的分支上
    CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
    if [ "$CURRENT_BRANCH" != "main" ] && [ "$CURRENT_BRANCH" != "master" ]; then
        echo -e "${YELLOW}警告: 当前分支是 $CURRENT_BRANCH，建议在 main/master 分支上发布${NC}"
        read -p "是否继续? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
}

# 验证版本号格式
validate_version() {
    local version=$1
    
    # 检查版本号格式 (支持语义化版本)
    if [[ ! $version =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$ ]]; then
        echo -e "${RED}错误: 无效的版本号格式: $version${NC}"
        echo "版本号应该遵循语义化版本规范，例如: 1.8.0, 2.0.0-beta.1"
        exit 1
    fi
    
    # 检查版本号是否已经存在
    if git tag -l | grep -q "^v$version$"; then
        if [ "$FORCE" = "true" ]; then
            echo -e "${YELLOW}警告: 版本 v$version 已存在，将强制覆盖${NC}"
        else
            echo -e "${RED}错误: 版本 v$version 已存在${NC}"
            echo "使用 -f 选项强制覆盖，或选择其他版本号"
            exit 1
        fi
    fi
}

# 更新CHANGELOG
update_changelog() {
    local version=$1
    local current_date=$(date +%Y-%m-%d)
    
    echo -e "${BLUE}更新 CHANGELOG.md...${NC}"
    
    # 备份原文件
    cp CHANGELOG.md CHANGELOG.md.bak
    
    # 替换未发布部分
    sed -i.bak "s/## \[未发布\]/## \[$version\] - $current_date/" CHANGELOG.md
    
    # 添加新的未发布部分
    sed -i.bak "/## \[$version\] - $current_date/a\\
\\
## [未发布]\\
\\
### 新增\\
- \\
\\
### 变更\\
- \\
\\
### 修复\\
- \\
" CHANGELOG.md
    
    # 清理临时文件
    rm -f CHANGELOG.md.bak
    
    echo -e "${GREEN}CHANGELOG.md 已更新${NC}"
}

# 创建Git标签
create_git_tag() {
    local version=$1
    
    echo -e "${BLUE}创建Git标签 v$version...${NC}"
    
    if [ "$FORCE" = "true" ]; then
        git tag -f "v$version"
    else
        git tag "v$version"
    fi
    
    echo -e "${GREEN}Git标签 v$version 已创建${NC}"
}

# 推送标签
push_tag() {
    local version=$1
    
    echo -e "${BLUE}推送标签到远程仓库...${NC}"
    
    if [ "$PUSH" = "true" ]; then
        git push origin "v$version"
        echo -e "${GREEN}标签已推送到远程仓库${NC}"
    else
        echo -e "${YELLOW}标签未推送，请手动执行: git push origin v$version${NC}"
    fi
}

# 显示发布信息
show_release_info() {
    local version=$1
    
    echo ""
    echo -e "${GREEN}=== 版本发布完成 ===${NC}"
    echo "版本号: v$version"
    echo "标签: v$version"
    echo ""
    
    if [ "$PUSH" = "true" ]; then
        echo -e "${GREEN}下一步:${NC}"
        echo "1. GitHub Actions 将自动触发构建"
        echo "2. 构建完成后将自动创建 Release"
        echo "3. 构建产物将自动上传到 Release 页面"
        echo ""
        echo "查看构建状态: https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^/]*\).*/\1/')/actions"
    else
        echo -e "${YELLOW}下一步:${NC}"
        echo "1. 推送标签: git push origin v$version"
        echo "2. 等待自动化构建完成"
    fi
    
    echo ""
    echo -e "${BLUE}版本信息:${NC}"
    echo "CHANGELOG.md 已更新"
    echo "Git标签已创建"
}

# 主函数
main() {
    local version=""
    local DRY_RUN=false
    local FORCE=false
    local PUSH=false
    
    # 解析命令行参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -d|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -f|--force)
                FORCE=true
                shift
                ;;
            -p|--push)
                PUSH=true
                shift
                ;;
            -*)
                echo -e "${RED}错误: 未知选项 $1${NC}"
                show_help
                exit 1
                ;;
            *)
                if [ -z "$version" ]; then
                    version=$1
                else
                    echo -e "${RED}错误: 只能指定一个版本号${NC}"
                    show_help
                    exit 1
                fi
                shift
                ;;
        esac
    done
    
    # 检查版本号参数
    if [ -z "$version" ]; then
        echo -e "${RED}错误: 请指定版本号${NC}"
        show_help
        exit 1
    fi
    
    # 检查Git状态
    check_git_status
    
    # 验证版本号
    validate_version "$version"
    
    # 显示将要执行的操作
    echo -e "${BLUE}=== 版本发布计划 ===${NC}"
    echo "版本号: v$version"
    echo "强制覆盖: $FORCE"
    echo "自动推送: $PUSH"
    echo "仅预览: $DRY_RUN"
    echo ""
    
    if [ "$DRY_RUN" = "true" ]; then
        echo -e "${YELLOW}预览模式 - 不会实际执行任何操作${NC}"
        echo "将要执行的命令:"
        echo "  git tag v$version"
        echo "  sed -i 's/## \[未发布\]/## \[$version\] - $(date +%Y-%m-%d)/' CHANGELOG.md"
        if [ "$PUSH" = "true" ]; then
            echo "  git push origin v$version"
        fi
        exit 0
    fi
    
    # 确认操作
    echo -e "${YELLOW}确认发布版本 v$version? (y/N): ${NC}"
    read -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "操作已取消"
        exit 1
    fi
    
    # 执行发布流程
    update_changelog "$version"
    create_git_tag "$version"
    push_tag "$version"
    show_release_info "$version"
}

# 运行主函数
main "$@"
