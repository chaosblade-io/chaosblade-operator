.PHONY: show-version linux_amd64 linux_arm64 pre_build operator chaos_fuse yaml build_binary build_linux_amd64_image build_linux_arm64_image build_linux_amd64_helm build_linux_arm64_helm build_linux_amd64_release build_linux_arm64_release push_image test clean help

# Default target - show help when no target is specified
.DEFAULT_GOAL := help

# Container runtime configuration - compatible with Docker and Podman
# Auto-detect available container runtime
ifeq ($(CONTAINER_RUNTIME),)
    ifeq ($(shell command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1 && echo "podman"),podman)
        CONTAINER_RUNTIME := podman
    else ifeq ($(shell command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1 && echo "docker"),docker)
        CONTAINER_RUNTIME := docker
    else
        CONTAINER_RUNTIME := docker
    endif
endif

# Get current platform information
CURRENT_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH := $(shell uname -m)
ifeq ($(CURRENT_ARCH),x86_64)
CURRENT_ARCH := amd64
else ifeq ($(CURRENT_ARCH),aarch64)
CURRENT_ARCH := arm64
endif

GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

UNAME := $(shell uname)

# Version information retrieval
ifeq ($(BLADE_VERSION), )
	BLADE_VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0")
endif
ifeq ($(BLADE_VENDOR), )
	BLADE_VENDOR=community
endif

# Dynamically get Git information
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

# Complete version information ldflags
VERSION_LDFLAGS=-X=$(VERSION_PKG).Version=$(BLADE_VERSION) \
	-X=$(VERSION_PKG).Product=$(BLADE_VENDOR) \
	-X=$(VERSION_PKG).BuildTime=$(BUILD_TIME) \
	-X=$(VERSION_PKG).GitCommit=$(GIT_COMMIT) \
	-X=$(VERSION_PKG).GitBranch=$(GIT_BRANCH) \
	-X=$(VERSION_PKG).GoVersion=$(GO_VERSION) \
	-X=$(VERSION_PKG).Platform=$(PLATFORM) \
	-X=$(VERSION_PKG).CombinedVersion=$(BLADE_VERSION),$(BLADE_VENDOR)

GO_FLAGS=-ldflags "$(VERSION_LDFLAGS)"
GO_FLAGS_WITH_STATIC=-ldflags="-linkmode external -extldflags -static $(VERSION_LDFLAGS)"

# Cross-compilation CC detection for chaos_fuse
define detect_cc
$(strip $(if $(and $(filter amd64,$(GOARCH)),$(shell command -v musl-gcc 2>/dev/null)),musl-gcc,\
$(if $(and $(filter amd64,$(GOARCH)),$(wildcard /usr/local/musl/bin/musl-gcc)),/usr/local/musl/bin/musl-gcc,\
$(if $(and $(filter amd64,$(GOARCH)),$(shell command -v x86_64-linux-musl-gcc 2>/dev/null)),x86_64-linux-musl-gcc,\
$(if $(and $(filter arm64,$(GOARCH)),$(shell command -v aarch64-linux-musl-gcc 2>/dev/null)),aarch64-linux-musl-gcc,\
$(if $(and $(filter amd64,$(GOARCH)),$(shell command -v gcc 2>/dev/null)),gcc,\
$(if $(and $(filter arm64,$(GOARCH)),$(shell command -v aarch64-linux-gnu-gcc 2>/dev/null)),aarch64-linux-gnu-gcc,\
$(if $(and $(filter arm64,$(GOARCH)),$(shell command -v gcc 2>/dev/null)),gcc,\
container))))))))
endef

CC_FOR_CHAOS_FUSE := $(call detect_cc)

# Display version information
show-version:
	@echo "=== Build Version Information ==="
	@echo "Version: $(BLADE_VERSION)"
	@echo "Vendor: $(BLADE_VENDOR)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Git Branch: $(GIT_BRANCH)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Go Version: $(GO_VERSION)"
	@echo "Platform: $(PLATFORM)"
	@echo "=================="

# Linux AMD64 platform build
linux_amd64: show-version pre_build
	@echo "Building Linux AMD64 platform components..."
	$(MAKE) operator GOOS=linux GOARCH=amd64
	@echo "chaosblade-operator build completed"
	$(MAKE) chaos_fuse GOOS=linux GOARCH=amd64
	@echo "chaos_fuse build completed"
	$(MAKE) yaml GOOS=linux GOARCH=arm64
	@echo "YAML specification file generation completed"
	@echo "Linux AMD64 platform build completed"

# Linux ARM64 platform build
linux_arm64: show-version pre_build
	@echo "Building Linux ARM64 platform components..."
	$(MAKE) operator GOOS=linux GOARCH=arm64
	@echo "chaosblade-operator build completed"
	$(MAKE) chaos_fuse GOOS=linux GOARCH=arm64
	@echo "chaos_fuse build completed"
	$(MAKE) yaml GOOS=linux GOARCH=arm64
	@echo "YAML specification file generation completed"
	@echo "Linux ARM64 platform build completed"

pre_build:
	@mkdir -p $(BUILD_TARGET_BIN) $(BUILD_TARGET_YAML) build/_output/bin

operator:
	@echo "Building chaosblade-operator for $(GOOS)/$(GOARCH)..."
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(GO_FLAGS) -o build/_output/bin/chaosblade-operator cmd/manager/main.go

chaos_fuse: ## Build chaos_fuse for Linux (supports cross-compilation from macOS)
ifeq ($(GOOS),linux)
	@echo "Detected CC for chaos_fuse: $(CC_FOR_CHAOS_FUSE)"
	@if [ "$(CC_FOR_CHAOS_FUSE)" != "container" ]; then \
		echo "Building chaos_fuse for Linux $(GOARCH) using $(CC_FOR_CHAOS_FUSE)..."; \
		CC=$(CC_FOR_CHAOS_FUSE) CGO_ENABLED=1 go build $(GO_FLAGS) -o $(BUILD_TARGET_BIN)/chaos_fuse cmd/hookfs/main.go; \
	elif command -v $(CONTAINER_RUNTIME) >/dev/null 2>&1 && $(CONTAINER_RUNTIME) info >/dev/null 2>&1; then \
		echo "Building chaos_fuse for Linux $(GOARCH) using $(CONTAINER_RUNTIME)..."; \
		if [ "$(GOARCH)" = "amd64" ]; then \
			$(CONTAINER_RUNTIME) run --rm -v $(PWD):/src:Z -w /src --platform linux/amd64 golang:1.21-alpine sh -c "apk add --no-cache musl-dev gcc && cd /src && CGO_ENABLED=1 go build $(GO_FLAGS) -o /src/$(BUILD_TARGET_BIN)/chaos_fuse cmd/hookfs/main.go" >/dev/null 2>&1; \
		elif [ "$(GOARCH)" = "arm64" ]; then \
			$(CONTAINER_RUNTIME) run --rm -v $(PWD):/src:Z -w /src golang:1.21-alpine sh -c "apk add --no-cache musl-dev gcc && cd /src && CGO_ENABLED=1 GOARCH=arm64 GOOS=linux go build $(GO_FLAGS) -o /src/$(BUILD_TARGET_BIN)/chaos_fuse cmd/hookfs/main.go" >/dev/null 2>&1; \
		else \
			echo "Unsupported architecture $(GOARCH) for chaos_fuse"; \
		fi; \
	else \
		echo "Warning: No suitable cross-compilation toolchain found for chaos_fuse"; \
		echo "Available options:"; \
		echo "  1. Install musl-tools: apt-get install musl-tools (Ubuntu/Debian)"; \
		echo "  2. Install musl-gcc: brew install FiloSottile/musl-cross/musl-cross (macOS)"; \
		echo "  3. Install specific cross-compilers for ARM64: apt-get install gcc-aarch64-linux-gnu g++-aarch64-linux-gnu"; \
		echo "  4. Use Docker/Podman with proper platform emulation"; \
	fi
else
	@echo "Skipping chaos_fuse build on $(GOOS) for target - Linux only"
endif


yaml: build/spec.go
	@echo "Building spec generator..."
	@GOOS=$(CURRENT_OS) GOARCH=$(CURRENT_ARCH) go build $(GO_FLAGS) -o build/_output/bin/spec $<
	@echo "Generating YAML specifications..."
	@GOOS=$(CURRENT_OS) GOARCH=$(CURRENT_ARCH) build/_output/bin/spec $(OS_YAML_FILE_PATH) $(if $(JVM_SPEC_PATH),$(JVM_SPEC_PATH),)

only_yaml: pre_build yaml

# Build binary files and display version information
build_binary: show-version
	CGO_ENABLED=0 go build $(GO_FLAGS) -o $(BUILD_TARGET_BIN)/chaosblade-operator cmd/manager/main.go
	@echo "Binary file build completed: $(BUILD_TARGET_BIN)/chaosblade-operator"
	@echo "Version information:"
	@$(BUILD_TARGET_BIN)/chaosblade-operator version 2>/dev/null || echo "Unable to get version information"


##----------------------------------------------------------------------------
# build image

build_linux_amd64_image:
	CGO_ENABLED=0 GOOS="linux" GOARCH="amd64" go build $(GO_FLAGS) -o build/_output/bin/chaosblade-operator cmd/manager/main.go
	$(CONTAINER_RUNTIME) buildx build -f build/image/amd/Dockerfile --platform=linux/amd64 -t ghcr.io/chaosblade-io/chaosblade-operator:${BLADE_VERSION} .

build_linux_arm64_image:
	CGO_ENABLED=0 GOOS="linux" GOARCH="arm64" go build $(GO_FLAGS) -o build/_output/bin/chaosblade-operator cmd/manager/main.go
	$(CONTAINER_RUNTIME) buildx build -f build/image/arm/Dockerfile  --platform=linux/arm64  -t ghcr.io/chaosblade-io/chaosblade-operator-arm64:${BLADE_VERSION} .

push_image:
	$(CONTAINER_RUNTIME) push ghcr.io/chaosblade-io/chaosblade-operator:${BLADE_VERSION}
	$(CONTAINER_RUNTIME) push ghcr.io/chaosblade-io/chaosblade-operator-arm64:${BLADE_VERSION}

# Build Helm packages with version updates
build_linux_amd64_helm: show-version pre_build
	@echo "Building Linux AMD64 Helm package..."
	@# Update Chart.yaml versions
	@sed -i.bak 's/^appVersion: ".*"/appVersion: "$(BLADE_VERSION)"/' deploy/helm/chaosblade-operator/Chart.yaml
	@sed -i.bak 's/^version: .*/version: $(BLADE_VERSION)/' deploy/helm/chaosblade-operator/Chart.yaml
	@# Update values.yaml versions
	@sed -i.bak 's/^  version: .*/  version: $(BLADE_VERSION)/' deploy/helm/chaosblade-operator/values.yaml
	@sed -i.bak 's/^  version: .*/  version: $(BLADE_VERSION)/' deploy/helm/chaosblade-operator/values.yaml
	@# Clean up backup files
	@rm -f deploy/helm/chaosblade-operator/Chart.yaml.bak deploy/helm/chaosblade-operator/values.yaml.bak
	@# Package Helm chart
	@helm package deploy/helm/chaosblade-operator --destination $(BUILD_TARGET) --version $(BLADE_VERSION) --app-version $(BLADE_VERSION)
	@# Rename the package to include architecture
	@mv $(BUILD_TARGET)/chaosblade-operator-$(BLADE_VERSION).tgz $(BUILD_TARGET)/chaosblade-operator-amd64-$(BLADE_VERSION).tgz
	@echo "Linux AMD64 Helm package created: $(BUILD_TARGET)/chaosblade-operator-amd64-$(BLADE_VERSION).tgz"

build_linux_arm64_helm: show-version pre_build
	@echo "Building Linux ARM64 Helm package..."
	@# Update Chart.yaml versions
	@sed -i.bak 's/^appVersion: ".*"/appVersion: "$(BLADE_VERSION)"/' deploy/helm/chaosblade-operator-arm64/Chart.yaml
	@sed -i.bak 's/^version: .*/version: $(BLADE_VERSION)/' deploy/helm/chaosblade-operator-arm64/Chart.yaml
	@# Update values.yaml versions
	@sed -i.bak 's/^  version: .*/  version: $(BLADE_VERSION)/' deploy/helm/chaosblade-operator-arm64/values.yaml
	@sed -i.bak 's/^  version: .*/  version: $(BLADE_VERSION)/' deploy/helm/chaosblade-operator-arm64/values.yaml
	@# Clean up backup files
	@rm -f deploy/helm/chaosblade-operator-arm64/Chart.yaml.bak deploy/helm/chaosblade-operator-arm64/values.yaml.bak
	@# Package Helm chart
	@helm package deploy/helm/chaosblade-operator-arm64 --destination $(BUILD_TARGET) --version $(BLADE_VERSION) --app-version $(BLADE_VERSION)
	@echo "Linux ARM64 Helm package created: $(BUILD_TARGET)/chaosblade-operator-arm64-$(BLADE_VERSION).tgz"

##----------------------------------------------------------------------------


build_linux_amd64_release: build_linux_amd64_image build_linux_amd64_helm
build_linux_arm64_release: build_linux_arm64_image build_linux_arm64_helm

# test
test:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...

# clean all build result
clean:
	go clean ./...
	rm -rf $(BUILD_TARGET)
	rm -rf $(BUILD_IMAGE_PATH)/$(BUILD_TARGET_DIR_NAME)

# Help information
help:
	@echo "Available build targets:"
	@echo "  linux_amd64    - Build Linux AMD64 platform components (operator + chaos_fuse + yaml)"
	@echo "  linux_arm64    - Build Linux ARM64 platform components (operator + chaos_fuse + yaml)"
	@echo "  build_linux_amd64_image - Build Linux AMD64 Docker image"
	@echo "  build_linux_arm64_image - Build Linux ARM64 Docker image"
	@echo "  build_linux_amd64_helm - Build and package Linux AMD64 Helm chart"
	@echo "  build_linux_arm64_helm - Build and package Linux ARM64 Helm chart"
	@echo "  build_linux_amd64_release - Build image and Helm package for AMD64"
	@echo "  build_linux_arm64_release - Build image and Helm package for ARM64"
	@echo "  push_image     - Push images to image registry"
	@echo "  show-version   - Display current version information"
	@echo "  clean          - Clean build artifacts"
	@echo ""
	@echo "Version-related environment variables:"
	@echo "  BLADE_VERSION  - Specify version number (default: Git tag)"
	@echo "  BLADE_VENDOR  - Specify vendor (default: community)"
	@echo ""
	@echo "Build-related environment variables:"
	@echo "  JVM_SPEC_PATH - Specify JVM specification file path (for container.JvmSpecFileForYaml)"
	@echo "  CONTAINER_RUNTIME - Specify container runtime (docker or podman, auto-detected by default)"
	@echo ""
