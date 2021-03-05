.PHONY: build clean

BLADE_SRC_ROOT=`pwd`

GO_ENV=CGO_ENABLED=1
GO_MODULE=GO111MODULE=on
GO=env $(GO_ENV) $(GO_MODULE) go

UNAME := $(shell uname)

ifeq ($(BLADE_VERSION), )
	BLADE_VERSION=0.10.0
endif
ifeq ($(BLADE_VENDOR), )
	BLADE_VENDOR=community
endif

BUILD_TARGET=target
BUILD_TARGET_DIR_NAME=chaosblade-$(BLADE_VERSION)
BUILD_TARGET_PKG_DIR=$(BUILD_TARGET)/chaosblade-$(BLADE_VERSION)
BUILD_TARGET_BIN=$(BUILD_TARGET_PKG_DIR)/bin
BUILD_TARGET_YAML=$(BUILD_TARGET_PKG_DIR)/yaml
BUILD_IMAGE_PATH=build/image/blade

OS_YAML_FILE_NAME=chaosblade-k8s-spec-$(BLADE_VERSION).yaml
OS_YAML_FILE_PATH=$(BUILD_TARGET_YAML)/$(OS_YAML_FILE_NAME)

VERSION_PKG=github.com/chaosblade-io/chaosblade-operator/version
GO_X_FLAGS=-X=$(VERSION_PKG).CombinedVersion=$(BLADE_VERSION),$(BLADE_VENDOR)
GO_FLAGS=-ldflags $(GO_X_FLAGS)

# cache downloaded file
CACHE_PATH=build/cache
DOWNLOAD_URL=https://chaosblade.oss-cn-hangzhou.aliyuncs.com/agent/github/${BLADE_VERSION}
CHAOSBLADE_FILE=chaosblade-${BLADE_VERSION}-linux-amd64.tar.gz
CHAOSBLADE_UNZIP_DIR=$(CACHE_PATH)/chaosblade-${BLADE_VERSION}
CHAOSBLADE_PATH=$(CACHE_PATH)/chaosblade

ifeq ($(GOOS), linux)
	GO_FLAGS=-ldflags="-linkmode external -extldflags -static $(GO_X_FLAGS)"
endif

build: pre_build build_yaml build_fuse

build_all: build build_image

build_image:
	operator-sdk build --go-build-args="$(GO_FLAGS)" chaosblade-operator:${BLADE_VERSION}

# only build_fuse and yaml
build_linux:
	docker build -f build/musl/Dockerfile -t chaosblade-operator-build-musl:latest build/musl
	docker run --rm \
		-v $(shell echo -n ${GOPATH}):/go \
		-w /go/src/github.com/chaosblade-io/chaosblade-operator \
		chaosblade-operator-build-musl:latest

pre_chaosblade:
ifneq ($(CHAOSBLADE_PATH), $(wildcard $(CHAOSBLADE_PATH)))
	wget "$(DOWNLOAD_URL)/$(CHAOSBLADE_FILE)" -O $(CACHE_PATH)/$(CHAOSBLADE_FILE)
	tar zxvf $(CACHE_PATH)/$(CHAOSBLADE_FILE) -C $(CACHE_PATH)
	mv $(CHAOSBLADE_UNZIP_DIR) $(CHAOSBLADE_PATH)
	rm -rf $(CACHE_PATH)/$(CHAOSBLADE_FILE)
endif

pre_build: pre_mkdir pre_chaosblade

pre_mkdir:
	rm -rf $(BUILD_TARGET_PKG_DIR) $(BUILD_TARGET_PKG_FILE_PATH)
	mkdir -p $(BUILD_TARGET_BIN) $(BUILD_TARGET_YAML) $(CACHE_PATH)

build_yaml: build/spec.go
	$(GO) run $< $(OS_YAML_FILE_PATH) $(CHAOSBLADE_PATH)/yaml/chaosblade-jvm-spec-$(BLADE_VERSION).yaml

build_fuse:
	$(GO) build $(GO_FLAGS) -o $(BUILD_TARGET_BIN)/chaos_fuse  cmd/hookfs/main.go


# test
test:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...
# clean all build result
clean:
	go clean ./...
	rm -rf $(BUILD_TARGET) $(CACHE_PATH)
	rm -rf $(BUILD_IMAGE_PATH)/$(BUILD_TARGET_DIR_NAME)
