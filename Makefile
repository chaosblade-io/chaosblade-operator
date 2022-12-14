.PHONY: build clean

GO_ENV=CGO_ENABLED=1
GO_MODULE=GO111MODULE=on
GO=env $(GO_ENV) $(GO_MODULE) go

UNAME := $(shell uname)

ifeq ($(BLADE_VERSION), )
	BLADE_VERSION=1.7.1
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

ifeq ($(GOOS), linux)
	GO_FLAGS=-ldflags="-linkmode external -extldflags -static $(GO_X_FLAGS)"
endif

build: build_yaml build_fuse

build_all: pre_build build docker-build

docker-build:
	GOOS="linux" GOARCH="amd64" go build $(GO_FLAGS) -o build/_output/bin/chaosblade-operator cmd/manager/main.go
	docker build -f build/image/amd/Dockerfile -t chaosbladeio/chaosblade-operator:${BLADE_VERSION} .

docker-build-arm64:
	GOOS="linux" GOARCH="arm64" go build $(GO_FLAGS) -o build/_output/bin/chaosblade-operator cmd/manager/main.go
	docker build -f build/image/arm/Dockerfile -t chaosbladeio/chaosblade-operator-arm64:${BLADE_VERSION} .

push_image:
	docker push chaosbladeio/chaosblade-operator:${BLADE_VERSION}
	docker push chaosbladeio/chaosblade-operator-arm64:${BLADE_VERSION}

#operator-sdk 0.19.0 build
build_all_operator: pre_build build build_image
build_image:
	operator-sdk build --go-build-args="$(GO_FLAGS)" chaosbladeio/chaosblade-operator:${BLADE_VERSION}

build_image_arm64:
	GOOS="linux" GOARCH="arm64" operator-sdk build --go-build-args="$(GO_FLAGS)" chaosbladeio/chaosblade-operator-arm64:${BLADE_VERSION}

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
		chaosbladeio/chaosblade-build-arm:latest

pre_build:
	mkdir -p $(BUILD_TARGET_BIN) $(BUILD_TARGET_YAML)

build_spec_yaml: build/spec.go
	$(GO) run $< $(OS_YAML_FILE_PATH)

build_yaml: pre_build build_spec_yaml

build_fuse:
	$(GO) build $(GO_FLAGS) -o $(BUILD_TARGET_BIN)/chaos_fuse cmd/hookfs/main.go

# test
test:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...
# clean all build result
clean:
	go clean ./...
	rm -rf $(BUILD_TARGET)
	rm -rf $(BUILD_IMAGE_PATH)/$(BUILD_TARGET_DIR_NAME)
