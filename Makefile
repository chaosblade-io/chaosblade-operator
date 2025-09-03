.PHONY: build clean

GO_ENV=CGO_ENABLED=1
GO_MODULE=GO111MODULE=on
GO=env $(GO_ENV) $(GO_MODULE) go

# 获取当前平台信息
CURRENT_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH := $(shell uname -m)
ifeq ($(CURRENT_ARCH),x86_64)
CURRENT_ARCH := amd64
else ifeq ($(CURRENT_ARCH),aarch64)
CURRENT_ARCH := arm64
endif

UNAME := $(shell uname)

# 版本信息获取
ifeq ($(BLADE_VERSION), )
	BLADE_VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0")
endif
ifeq ($(BLADE_VENDOR), )
	BLADE_VENDOR=community
endif

# 动态获取Git信息
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GO_VERSION := $(shell go version | awk '{print $$3}')
PLATFORM := $(shell echo "$(GOOS)/$(GOARCH)")

BUILD_TARGET=target
BUILD_TARGET_DIR_NAME=chaosblade-$(BLADE_VERSION)
BUILD_TARGET_PKG_DIR=$(BUILD_TARGET)/chaosblade-$(BLADE_VERSION)
BUILD_TARGET_BIN=$(BUILD_TARGET_PKG_DIR)/bin
BUILD_TARGET_YAML=$(BUILD_TARGET_PKG_DIR)/yaml
BUILD_IMAGE_PATH=build/image/blade

OS_YAML_FILE_NAME=chaosblade-k8s-spec-$(BLADE_VERSION).yaml
OS_YAML_FILE_PATH=$(BUILD_TARGET_YAML)/$(OS_YAML_FILE_NAME)

VERSION_PKG=github.com/chaosblade-io/chaosblade-operator/version

# 完整的版本信息ldflags
VERSION_LDFLAGS=-X=$(VERSION_PKG).Version=$(BLADE_VERSION) \
	-X=$(VERSION_PKG).Product=$(BLADE_VENDOR) \
	-X=$(VERSION_PKG).BuildTime=$(BUILD_TIME) \
	-X=$(VERSION_PKG).GitCommit=$(GIT_COMMIT) \
	-X=$(VERSION_PKG).GitBranch=$(GIT_BRANCH) \
	-X=$(VERSION_PKG).GoVersion=$(GO_VERSION) \
	-X=$(VERSION_PKG).Platform=$(PLATFORM) \
	-X=$(VERSION_PKG).CombinedVersion=$(BLADE_VERSION),$(BLADE_VENDOR)

GO_FLAGS=-ldflags "$(VERSION_LDFLAGS)"

# Linux静态链接支持
ifeq ($(GOOS), linux)
	GO_FLAGS=-ldflags="-linkmode external -extldflags -static $(VERSION_LDFLAGS)"
endif

# 显示版本信息
show-version:
	@echo "=== 构建版本信息 ==="
	@echo "版本: $(BLADE_VERSION)"
	@echo "供应商: $(BLADE_VENDOR)"
	@echo "Git提交: $(GIT_COMMIT)"
	@echo "Git分支: $(GIT_BRANCH)"
	@echo "构建时间: $(BUILD_TIME)"
	@echo "Go版本: $(GO_VERSION)"
	@echo "平台: $(PLATFORM)"
	@echo "=================="

build: show-version build_yaml build_fuse

build_all: pre_build build docker-build
build_all_arm64: pre_build build docker-build-arm64

docker-build:
	GOOS="linux" GOARCH="amd64" go build $(GO_FLAGS) -o build/_output/bin/chaosblade-operator cmd/manager/main.go
	docker buildx build -f build/image/amd/Dockerfile --platform=linux/amd64 -t ghcr.io/chaosblade-io/chaosblade-operator:${BLADE_VERSION} .

docker-build-arm64:
	GOOS="linux" GOARCH="arm64" go build $(GO_FLAGS) -o build/_output/bin/chaosblade-operator cmd/manager/main.go
	docker buildx build -f build/image/arm/Dockerfile  --platform=linux/arm64  -t ghcr.io/chaosblade-io/chaosblade-operator-arm64:${BLADE_VERSION} .

push_image:
	docker push ghcr.io/chaosblade-io/chaosblade-operator:${BLADE_VERSION}
	docker push ghcr.io/chaosblade-io/chaosblade-operator-arm64:${BLADE_VERSION}

#operator-sdk 0.19.0 build
build_all_operator: pre_build build build_image
build_image:
	operator-sdk build --go-build-args="$(GO_FLAGS)" ghcr.io/chaosblade-io/chaosblade-operator:${BLADE_VERSION}

build_image_arm64:
	GOOS="linux" GOARCH="arm64" operator-sdk build --go-build-args="$(GO_FLAGS)" ghcr.io/chaosblade-io/chaosblade-operator-arm64:${BLADE_VERSION}

# only build_fuse and yaml
build_linux:
	docker build -f build/musl/Dockerfile -t chaosblade-operator-build-musl:latest build/musl
	docker run --rm \
		-v $(shell echo -n ${GOPATH}):/go \
		-v $(shell pwd):/go/src/github.com/chaosblade-io/chaosblade-operator \
		-w /go/src/github.com/chaosblade-io/chaosblade-operator \
		chaosblade-operator-build-musl:latest

build_arm64:
	docker run --rm --privileged multiarch/qemu-user-static:register --reset
	docker run --rm \
		-v $(shell echo -n ${GOPATH}):/go \
		-v $(shell pwd):/go/src/github.com/chaosblade-io/chaosblade-operator \
		-w /go/src/github.com/chaosblade-io/chaosblade-operator \
		chaosblade-io/chaosblade-build-arm:latest

pre_build:
	mkdir -p $(BUILD_TARGET_BIN) $(BUILD_TARGET_YAML) build/_output/bin

build_spec_yaml: build/spec.go
	GOOS=$(CURRENT_OS) GOARCH=$(CURRENT_ARCH) $(GO) build $(GO_FLAGS) -o build/_output/bin/spec $<
	GOOS=$(CURRENT_OS) GOARCH=$(CURRENT_ARCH) build/_output/bin/spec $(OS_YAML_FILE_PATH) $(if $(JVM_SPEC_PATH),$(JVM_SPEC_PATH),)

build_yaml: pre_build build_spec_yaml

build_fuse:
	$(GO) build $(GO_FLAGS) -o $(BUILD_TARGET_BIN)/chaos_fuse cmd/hookfs/main.go

# 构建二进制文件并显示版本信息
build_binary: show-version
	$(GO) build $(GO_FLAGS) -o $(BUILD_TARGET_BIN)/chaosblade-operator cmd/manager/main.go
	@echo "二进制文件构建完成: $(BUILD_TARGET_BIN)/chaosblade-operator"
	@echo "版本信息:"
	@$(BUILD_TARGET_BIN)/chaosblade-operator version 2>/dev/null || echo "无法获取版本信息"

# test
test:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...

# clean all build result
clean:
	go clean ./...
	rm -rf $(BUILD_TARGET)
	rm -rf $(BUILD_IMAGE_PATH)/$(BUILD_TARGET_DIR_NAME)

# 帮助信息
help:
	@echo "可用的构建目标:"
	@echo "  build          - 构建基本组件 (显示版本信息)"
	@echo "  build_binary   - 构建二进制文件并显示版本信息"
	@echo "  show-version   - 显示当前版本信息"
	@echo "  docker-build   - 构建Docker镜像 (AMD64)"
	@echo "  docker-build-arm64 - 构建Docker镜像 (ARM64)"
	@echo "  build_all      - 完整构建流程"
	@echo "  clean          - 清理构建产物"
	@echo ""
	@echo "版本相关环境变量:"
	@echo "  BLADE_VERSION  - 指定版本号 (默认: Git标签)"
	@echo "  BLADE_VENDOR  - 指定供应商 (默认: community)"
	@echo ""
	@echo "构建相关环境变量:"
	@echo "  JVM_SPEC_PATH - 指定JVM规范文件路径 (用于container.JvmSpecFileForYaml)"
