.PHONY: lint fmt lintfix test checkpath check cover test-norace deps
checkpath:
ifeq ("","$(shell which go)")
	$(error go binary not in PATH)
endif

check: checkpath
ifeq ("","$(shell which golangci-lint)")
	$(error golangci-lint binary not in PATH)
endif

lint: fmt
	goimports -l -w -local $(shell head -n 1 go.mod | cut -d ' ' -f 2) .
	@golangci-lint run

fmt:
	go fmt ./...

lintfix:
	@golangci-lint run --fix

cover: check
ifndef CI
	go tool cover -html .cover/cover.out
else
	go tool cover -html .cover/cover.out -o .cover/all.html
endif

test:
	@echo 'Running unit tests...'
	@mkdir -p .cover
	@GOFLAGS=$(GOFLAGS) go test -v -race -count=10 ./... \
		-coverprofile .cover/cover.out

test-norace:
	@echo 'Running unit tests without race detection...'
	@mkdir -p .cover
	@GOFLAGS=$(GOFLAGS) go test -v ./... -coverprofile .cover/cover.out

deps:
	go get -u ./...
	go mod tidy
