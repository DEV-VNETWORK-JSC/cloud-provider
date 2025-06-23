# Version management
VERSION_FILE := .version
VERSION := $(shell if [ -f $(VERSION_FILE) ]; then cat $(VERSION_FILE); else echo "v0.1.0"; fi)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date -u '+%Y-%m-%d')

# Binary name
BINARY := cloud-provider-manager

# Build directory
BUILD_DIR := bin

# Build flags
LDFLAGS := -X k8s.io/cloud-provider/version.Version=$(VERSION) \
           -X k8s.io/cloud-provider/version.BuildTime=$(BUILD_TIME) \
           -X k8s.io/cloud-provider/version.GitCommit=$(GIT_COMMIT)

# Build variables
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Setting SHELL to bash allows bash commands to be executed by recipes
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build

.PHONY: build
build: ## Build binary for current platform
	@echo "Building $(BINARY) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./sample

.PHONY: build-linux
build-linux: ## Build binary for Linux AMD64
	@echo "Building $(BINARY) for linux/amd64..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./sample

.PHONY: build-linux-arm64
build-linux-arm64: ## Build binary for Linux ARM64
	@echo "Building $(BINARY) for linux/arm64..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-linux-arm64 ./sample

.PHONY: build-darwin
build-darwin: ## Build binary for macOS AMD64
	@echo "Building $(BINARY) for darwin/amd64..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-darwin-amd64 ./sample

.PHONY: build-darwin-arm64
build-darwin-arm64: ## Build binary for macOS ARM64
	@echo "Building $(BINARY) for darwin/arm64..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./sample

.PHONY: build-all
build-all: build-linux build-linux-arm64 build-darwin build-darwin-arm64 ## Build for all platforms
	@echo "Built binaries for all platforms:"
	@ls -la $(BUILD_DIR)/

.PHONY: install
install: build ## Install binary to GOPATH/bin
	@echo "Installing $(BINARY) to $(GOPATH)/bin..."
	@cp $(BUILD_DIR)/$(BINARY) $(GOPATH)/bin/
	@echo "$(BINARY) installed successfully"

.PHONY: install-local
install-local: build ## Install binary to /usr/local/bin (requires sudo)
	@echo "Installing $(BINARY) to /usr/local/bin..."
	@sudo cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/
	@echo "$(BINARY) installed to /usr/local/bin/"

##@ Development

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	@echo "Running go vet..."
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests
	@echo "Running tests..."
	go test -v -coverprofile cover.out ./...

.PHONY: lint
lint: ## Run golangci-lint (install: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin)
	@echo "Running golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin"; \
	fi

.PHONY: tidy
tidy: ## Run go mod tidy
	@echo "Tidying go modules..."
	go mod tidy

.PHONY: verify
verify: fmt vet tidy test lint ## Run all verification checks

##@ Version Management

.PHONY: version-patch
version-patch: ## Increment patch version (e.g., v1.0.5 -> v1.0.6)
	@echo "Current version: $(VERSION)"
	@if [ ! -f $(VERSION_FILE) ]; then echo "v0.1.0" > $(VERSION_FILE); fi
	@awk -F'[v.]' '{ printf("v%d.%d.%d", $$2, $$3, $$4+1) }' $(VERSION_FILE) > $(VERSION_FILE).tmp
	@mv $(VERSION_FILE).tmp $(VERSION_FILE)
	@echo "New version: $$(cat $(VERSION_FILE))"

.PHONY: version-minor
version-minor: ## Increment minor version (e.g., v1.0.5 -> v1.1.0)
	@echo "Current version: $(VERSION)"
	@if [ ! -f $(VERSION_FILE) ]; then echo "v0.1.0" > $(VERSION_FILE); fi
	@awk -F'[v.]' '{ printf("v%d.%d.%d", $$2, $$3+1, 0) }' $(VERSION_FILE) > $(VERSION_FILE).tmp
	@mv $(VERSION_FILE).tmp $(VERSION_FILE)
	@echo "New version: $$(cat $(VERSION_FILE))"

.PHONY: version-major
version-major: ## Increment major version (e.g., v1.0.5 -> v2.0.0)
	@echo "Current version: $(VERSION)"
	@if [ ! -f $(VERSION_FILE) ]; then echo "v0.1.0" > $(VERSION_FILE); fi
	@awk -F'[v.]' '{ printf("v%d.%d.%d", $$2+1, 0, 0) }' $(VERSION_FILE) > $(VERSION_FILE).tmp
	@mv $(VERSION_FILE).tmp $(VERSION_FILE)
	@echo "New version: $$(cat $(VERSION_FILE))"

.PHONY: release
release: version-patch build-all ## Create new release (increments patch version and builds all platforms)
	@VERSION=$$(cat $(VERSION_FILE))
	@echo "Released version $$VERSION"
	@git add $(VERSION_FILE)
	@git commit -m "Release version $$VERSION"
	@git tag -a "$$VERSION" -m "Release version $$VERSION" || true
	@echo "Created release $$VERSION with binaries for all platforms"

##@ Docker

# Docker image settings
REGISTRY ?= k8s.io.infra.vnetwork.dev
IMAGE_NAME ?= cloud-provider
IMAGE_TAG ?= $(VERSION)
IMG ?= $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

.PHONY: docker-build
docker-build: ## Build docker image
	@echo "Building Docker image $(IMG)..."
	docker build -t $(IMG) .

.PHONY: docker-build-fast
docker-build-fast: build-linux ## Build docker image using pre-built binary
	@echo "Building Docker image $(IMG) with pre-built binary..."
	docker build -f Dockerfile.prebuilt -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push docker image
	@echo "Pushing Docker image $(IMG)..."
	docker push $(IMG)

.PHONY: docker-run
docker-run: docker-build-fast ## Run docker container locally
	@echo "Running Docker container..."
	docker run --rm -it \
		-v ~/.kube:/root/.kube:ro \
		$(IMG) --help

##@ Utilities

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f cover.out
	@echo "Clean complete"

.PHONY: show-version
show-version: ## Show current version info
	@echo "Version: $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

.PHONY: compress
compress: build-all ## Compress all binaries with UPX (requires upx installed)
	@echo "Compressing binaries with UPX..."
	@if command -v upx >/dev/null 2>&1; then \
		upx -9 $(BUILD_DIR)/*; \
	else \
		echo "UPX not installed. Install with: brew install upx (macOS) or apt-get install upx (Linux)"; \
	fi

.PHONY: package
package: build-all ## Create tar.gz packages for each platform
	@echo "Creating release packages..."
	@mkdir -p $(BUILD_DIR)/release
	@for binary in $(BUILD_DIR)/$(BINARY)-*; do \
		platform=$$(basename $$binary | sed 's/$(BINARY)-//'); \
		tar -czf $(BUILD_DIR)/release/$(BINARY)-$(VERSION)-$$platform.tar.gz -C $(BUILD_DIR) $$(basename $$binary); \
		echo "Created $(BUILD_DIR)/release/$(BINARY)-$(VERSION)-$$platform.tar.gz"; \
	done
	@ls -la $(BUILD_DIR)/release/

.PHONY: generate
generate: ## Run go generate for code generation
	@echo "Running go generate..."
	go generate ./...

.PHONY: codegen
codegen: ## Run Kubernetes code generation
	@echo "Running Kubernetes code generation..."
	@if [ -d "hack" ]; then \
		bash hack/update-codegen.sh; \
	else \
		echo "No hack directory found, skipping codegen"; \
	fi