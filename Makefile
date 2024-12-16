GO = go
GOIMPORTS = goimports
SOURCES := $(shell find . -name '*.go')
VERSION := $(shell git describe --tags --always --dirty)

.PHONY: all
all: build

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all        : Build the project"
	@echo "  build      : Build the binary"
	@echo "  clean      : Remove build artifacts"
	@echo "  lint       : Run linters"
	@echo "  test       : Run tests with race detection"
	@echo "  test-norace: Run tests without race detection"
	@echo "  deps       : Update dependencies"
	@echo "  version    : Show current version"

.PHONY: version
version:
	@echo "Current version: $(VERSION)"

define check_binary
	@command -v $(1) >/dev/null 2>&1 || { echo "Error: $(1) binary not in PATH"; exit 1; }
endef

.PHONY: checkpath
checkpath:
	$(call check_binary,go)

.PHONY: check
check: checkpath
	$(call check_binary,golangci-lint)

.PHONY: docker-clean
docker-clean:
	docker compose down --volumes
	docker compose down --rmi all

.PHONY: clean
clean: docker-clean
	rm -f throttle-proxy

.PHONY: build
build: throttle-proxy

.PHONY: throttle-proxy
throttle-proxy: $(SOURCES)
	@echo ">> building binaries..."
	@$(GO) build -o $@ github.com/kevindweb/throttle-proxy

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: lint
lint: fmt
	@$(GOIMPORTS) -l -w -local $(shell head -n 1 go.mod | cut -d ' ' -f 2) .
	@golangci-lint run

.PHONY: lintfix
lintfix: fmt
	@golangci-lint run --fix

.PHONY: ruff
ruff:
	ruff check .

TEST_FLAGS := -v -coverprofile .cover/cover.out
TEST_PATH := ./...

.PHONY: test
test:
	@echo 'Running unit tests...'
	@mkdir -p .cover
	@GOFLAGS=$(GOFLAGS) go test $(TEST_FLAGS) -race -count=10 $(TEST_PATH)

.PHONY: test-norace
test-norace:
	@echo 'Running unit tests without race detection...'
	@mkdir -p .cover
	@GOFLAGS=$(GOFLAGS) go test $(TEST_FLAGS) $(TEST_PATH)

.PHONY: cover
cover: check
ifndef CI
	go tool cover -html .cover/cover.out
else
	go tool cover -html .cover/cover.out -o .cover/all.html
endif

.PHONY: deps
deps:
	go get -u ./...
	go mod tidy
