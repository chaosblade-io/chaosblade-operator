# ChaosBlade Operator Build Guide

This document describes how to build the ChaosBlade Operator project. The project supports multi-platform builds, including Linux AMD64 and ARM64 architectures.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Environment Variables](#environment-variables)
- [Build Targets](#build-targets)
- [Container Image Building](#container-image-building)
- [Testing](#testing)
- [Cleanup](#cleanup)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### Basic Requirements

- **Go 1.21+**: For compiling Go code
- **Git**: For retrieving version information
- **Make**: For executing build scripts

### Container Runtime (Optional)

The project automatically detects available container runtimes:
- **Docker**: Recommended
- **Podman**: As an alternative to Docker

### Cross-compilation Toolchain (Optional)

For Linux builds of the `chaos_fuse` component, one of the following tools is required:

#### Linux Systems
```bash
# Ubuntu/Debian - Basic build tools
sudo apt-get install musl-tools build-essential

# For ARM64 cross-compilation (when building on x86_64 for ARM64)
sudo apt-get install gcc-aarch64-linux-gnu g++-aarch64-linux-gnu

# For native ARM64 compilation (when building on ARM64 for ARM64)
# The standard gcc from build-essential is sufficient
```

#### macOS Systems
```bash
# Install musl cross-compiler using Homebrew
brew install FiloSottile/musl-cross/musl-cross
```

### Platform Compatibility

- **Build Platforms**: macOS, Linux
- **Target Platforms**: Linux AMD64, Linux ARM64
- **Container Platforms**: Multi-architecture container builds supported

## Environment Variables

### Version-related

| Variable | Description | Default | Example |
|----------|-------------|---------|----------|
| `BLADE_VERSION` | Version number | Git tag (e.g., `1.7.4`) | `1.8.0` |
| `BLADE_VENDOR` | Vendor identifier | `community` | `enterprise` |

### Build-related

| Variable | Description | Default | Example |
|----------|-------------|---------|----------|
| `CONTAINER_RUNTIME` | Container runtime | Auto-detected | `docker`, `podman` |
| `JVM_SPEC_PATH` | JVM specification file path | None | `/path/to/jvm/spec` |

### Example Configuration
```bash
export BLADE_VERSION=1.8.0
export BLADE_VENDOR=community
export CONTAINER_RUNTIME=docker
```

## Build Targets

### View Help Information

```bash
make help
```

Displays all available build targets and environment variable descriptions.

### View Version Information

```bash
make show-version
```

Displays current build version information, including:
- Version number
- Vendor
- Git commit hash
- Git branch
- Build time
- Go version
- Target platform

### Complete Platform Builds

#### Linux AMD64 Platform
```bash
make linux_amd64
```

Builds:
- `chaosblade-operator` (Linux AMD64)
- `chaos_fuse` (Linux AMD64)
- YAML specification files

#### Linux ARM64 Platform
```bash
make linux_arm64
```

Builds:
- `chaosblade-operator` (Linux ARM64)
- `chaos_fuse` (Linux ARM64)
- YAML specification files

### Individual Component Builds

#### Build Operator
```bash
make operator GOOS=linux GOARCH=amd64
```

#### Build chaos_fuse
```bash
make chaos_fuse GOOS=linux GOARCH=amd64
```

#### Generate YAML Specifications
```bash
make yaml
# Or generate YAML only
make only_yaml
```

#### Build Local Binary
```bash
make build_binary
```

## Container Image Building

### Linux AMD64 Image
```bash
make build_linux_amd64_image
```

Builds and tags as: `ghcr.io/chaosblade-io/chaosblade-operator:${BLADE_VERSION}`

### Linux ARM64 Image
```bash
make build_linux_arm64_image
```

Builds and tags as: `ghcr.io/chaosblade-io/chaosblade-operator-arm64:${BLADE_VERSION}`

### Push Images
```bash
make push_image
```

Pushes the built images to GitHub Container Registry.

## Testing

Run the project test suite:

```bash
make test
```

This command will:
- Run all test cases
- Enable race detection
- Generate code coverage report (`coverage.txt`)

## Cleanup

Clean all build artifacts:

```bash
make clean
```

Cleans:
- Go build cache
- `target/` directory
- Build image directories

## Build Output

### Directory Structure

```
target/chaosblade-${BLADE_VERSION}/
├── bin/
│   └── chaos_fuse             # File system hook tool
└── yaml/
    └── chaosblade-k8s-spec-${BLADE_VERSION}.yaml  # Kubernetes specification file
```

### Temporary Build Files

```
build/_output/bin/
├── chaosblade-operator        # Temporarily built operator file
└── spec                       # Specification generator tool
```

### File Description
* `chaos_fuse` and `chaosblade-k8s-spec-${BLADE_VERSION}.yaml` need to be packaged into chaosblade for use (can be compiled and packaged directly in the chaosblade project);
* `chaosblade-operator` needs to be packaged into the chaosblade-operator image for use (can be compiled directly using build_linux_xxx_image tasks);

## Troubleshooting

### chaos_fuse Build Failure

**Issue**: Missing cross-compilation toolchain

**Solutions**:
1. **Preferred approach**: Install appropriate cross-compiler
   ```bash
   # macOS
   brew install FiloSottile/musl-cross/musl-cross
   
   # Linux
   sudo apt-get install musl-tools
   ```

2. **Alternative approach**: Use container build
   ```bash
   # Ensure Docker/Podman is running
   docker info  # or podman info
   ```

3. **Manually specify container runtime**:
   ```bash
   CONTAINER_RUNTIME=podman make chaos_fuse GOOS=linux GOARCH=amd64
   ```

### Container Build Failure

**Issue**: Container runtime unavailable

**Solutions**:
1. Check Docker/Podman status
   ```bash
   docker info
   # or
   podman info
   ```

2. Start Docker service
   ```bash
   # macOS/Linux
   sudo systemctl start docker
   # or use Docker Desktop
   ```

### Version Information Retrieval Failure

**Issue**: Git repository information unavailable

**Solutions**:
1. Ensure running build within Git repository
2. Manually specify version:
   ```bash
   BLADE_VERSION=1.8.0 make linux_amd64
   ```

### Permission Issues

**Issue**: File permission errors

**Solutions**:
1. Check directory permissions
2. Run build with appropriate user permissions
3. For container builds, ensure SELinux compatibility (`:Z` flag)

## Advanced Usage

### Custom Build Flags

```bash
# Add custom ldflags
GO_FLAGS="-ldflags '-X main.customFlag=value'" make operator
```

### Parallel Builds

```bash
# Use parallel builds for acceleration
make -j4 linux_amd64
```

### Debug Builds

```bash
# Enable verbose output
make V=1 linux_amd64
```

## Contributing Guidelines

When building new features or fixes:

1. Ensure all build targets work properly
2. Run complete test suite: `make test`
3. Verify cross-platform builds: `make linux_amd64 linux_arm64`
4. Check code coverage reports

## Related Documentation

- [Contributing Guide](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)
