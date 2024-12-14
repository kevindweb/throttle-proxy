.PHONY: all build lint fmt lintfix test checkpath check cover test-norace deps help version docker-clean

GO = go
GOIMPORTS = goimports
SOURCES := $(shell find . -name '*.go')
VERSION := $(shell git describe --tags --always --dirty)

all: build

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

version:
	@echo "Current version: $(VERSION)"

define check_binary
	@command -v $(1) >/dev/null 2>&1 || { echo "Error: $(1) binary not in PATH"; exit 1; }
endef

checkpath:
	$(call check_binary,go)

check: checkpath
	$(call check_binary,golangci-lint)

docker-clean:
	docker compose down --volumes
	docker compose down --rmi all

clean: docker-clean
	rm -f throttle-proxy

build: throttle-proxy

throttle-proxy: $(SOURCES)
	@echo ">> building binaries..."
	@$(GO) build -o $@ github.com/kevindweb/throttle-proxy

fmt:
	go fmt ./...

lint: fmt
	@$(GOIMPORTS) -l -w -local $(shell head -n 1 go.mod | cut -d ' ' -f 2) .
	@golangci-lint run

lintfix: fmt
	@golangci-lint run --fix

TEST_FLAGS := -v -coverprofile .cover/cover.out
TEST_PATH := ./...

test:
	@echo 'Running unit tests...'
	@mkdir -p .cover
	@GOFLAGS=$(GOFLAGS) go test $(TEST_FLAGS) -race -count=10 $(TEST_PATH)

test-norace:
	@echo 'Running unit tests without race detection...'
	@mkdir -p .cover
	@GOFLAGS=$(GOFLAGS) go test $(TEST_FLAGS) $(TEST_PATH)

cover: check
ifndef CI
	go tool cover -html .cover/cover.out
else
	go tool cover -html .cover/cover.out -o .cover/all.html
endif

deps:
	go get -u ./...
	go mod tidy
