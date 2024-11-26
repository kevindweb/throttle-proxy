.PHONY: lint fmt lintfix test checkpath check cover unit unit-norace deps
lint:
	goimports -l -w -local $(shell head -n 1 go.mod | cut -d ' ' -f 2) .
	@golangci-lint run

fmt:
	go fmt ./...

lintfix:
	@golangci-lint run --fix

test: unit

checkpath:
ifeq ("","$(shell which go)")
	$(error go binary not in PATH)
endif

check: checkpath
ifeq ("","$(shell which golangci-lint)")
	$(error golangci-lint binary not in PATH)
endif

cover: check
ifndef CI
	go tool cover -html .cover/cover.out
else
	go tool cover -html .cover/cover.out -o .cover/all.html
endif

unit:
	@echo 'Running unit tests...'
	@mkdir -p .cover
	@GOFLAGS=$(GOFLAGS) go test -v -race -count=10 ./... \
		-coverprofile .cover/cover.out

unit-norace:
	@echo 'Running unit tests without race detection...'
	@mkdir -p .cover
	@GOFLAGS=$(GOFLAGS) go test -v ./... -coverprofile .cover/cover.out

deps:
	go get -u ./...
	go mod tidy
