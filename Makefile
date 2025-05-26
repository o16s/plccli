# Binary name
BINARY_NAME=plccli

# Build directory
BUILD_DIR=build

# Main Go package
MAIN_PACKAGE=.

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Linker flags
LD_FLAGS = -X 'main.buildVersion=$(VERSION)' \
           -X 'main.buildCommit=$(COMMIT)' \
           -X 'main.buildTime=$(BUILD_TIME)'

.PHONY: all build clean build-mac build-linux fix

# Default target: build for current platform
build:
	go build -ldflags="$(LD_FLAGS)" -o $(BINARY_NAME) $(MAIN_PACKAGE)

# Fix common code issues before building
fix:
	@echo "Fixing common code issues..."
	@grep -l "logSuffix :=" main.go | xargs sed -i '' '/logSuffix :=/,+2d' || true
	@echo "Fix complete. Now trying to build..."
	go build -ldflags="$(LD_FLAGS)" -o $(BINARY_NAME) $(MAIN_PACKAGE)

# Build for all platforms
all: clean build-mac build-linux

# Build for macOS (Apple Silicon)
build-mac:
	mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LD_FLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PACKAGE)

# Build for Linux (both amd64 and arm64)
build-linux:
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LD_FLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LD_FLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PACKAGE)

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)

# Run the application
run: build
	./$(BINARY_NAME)