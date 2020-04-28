.PHONY: build clean

BLADE_SRC_ROOT=`pwd`

GO_ENV=CGO_ENABLED=1
GO_MODULE=GO111MODULE=on
GO=env $(GO_ENV) $(GO_MODULE) go

UNAME := $(shell uname)

ifeq ($(BLADE_VERSION), )
	BLADE_VERSION=0.6.0
endif
ifeq ($(BLADE_VENDOR), )
	BLADE_VENDOR=community
endif

BUILD_TARGET=target
BUILD_TARGET_DIR_NAME=chaosblade-$(BLADE_VERSION)
BUILD_TARGET_PKG_DIR=$(BUILD_TARGET)/chaosblade-$(BLADE_VERSION)
BUILD_TARGET_BIN=$(BUILD_TARGET_PKG_DIR)/bin
BUILD_IMAGE_PATH=build/image/blade
BUILD_IMAGE_BIN=build/_output/bin
# cache downloaded file
BUILD_TARGET_CACHE=$(BUILD_TARGET)/cache

OS_YAML_FILE_NAME=chaosblade-k8s-spec-$(BLADE_VERSION).yaml
OS_YAML_FILE_PATH=$(BUILD_TARGET_BIN)/$(OS_YAML_FILE_NAME)

VERSION_PKG=github.com/chaosblade-io/chaosblade-operator/version
GO_X_FLAGS=-X=$(VERSION_PKG).CombinedVersion=$(BLADE_VERSION),$(BLADE_VENDOR)
GO_FLAGS=-ldflags $(GO_X_FLAGS)

ifeq ($(GOOS), linux)
	GO_FLAGS=-ldflags="-linkmode external -extldflags -static $(GO_X_FLAGS)"
endif

build: pre_build build_yaml build_fuse 

build_all: build build_image

build_image: build_webhook
	operator-sdk build --go-build-args="$(GO_FLAGS)" chaosblade-operator:${BLADE_VERSION}

build_linux: build

pre_build:
	rm -rf $(BUILD_TARGET_PKG_DIR) $(BUILD_TARGET_PKG_FILE_PATH)
	mkdir -p $(BUILD_TARGET_BIN) $(BUILD_TARGET_LIB)

build_yaml: build/spec.go
	$(GO) run $< $(OS_YAML_FILE_PATH)

build_webhook:
	$(GO) build $(GO_FLAGS) -o $(BUILD_IMAGE_BIN)/chaosblade-webhook cmd/webhook/main.go

build_fuse:
	$(GO) build $(GO_FLAGS) -o $(BUILD_TARGET_BIN)/chaos_fuse  cmd/hookfs/main.go


# test
test:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...
# clean all build result
clean:
	go clean ./...
	rm -rf $(BUILD_TARGET)
	rm -rf $(BUILD_IMAGE_PATH)/$(BUILD_TARGET_DIR_NAME)
