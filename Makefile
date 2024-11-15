.PHONY: lint
lint:
	goimports -l -w -local $(shell head -n 1 go.mod | cut -d ' ' -f 2) .
	@golangci-lint run

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: lintfix
lintfix:
	@golangci-lint run --fix

.PHONY: test
test: unit

.PHONY: checkpath
checkpath:
ifeq ("","$(shell which go)")
	$(error go binary not in PATH)
endif

.PHONY: check
check: checkpath
ifeq ("","$(shell which golangci-lint)")
	$(error golangci-lint binary not in PATH)
endif

.PHONY: cover
cover: check
ifndef CI
	go tool cover -html .cover/cover.out
else
	go tool cover -html .cover/cover.out -o .cover/all.html
endif

.PHONY: unit
unit:
	@echo 'Running unit tests...'
	@mkdir -p .cover
	@GOFLAGS=$(GOFLAGS) go test -v -race -count=10 ./... \
		-coverprofile .cover/cover.out

.PHONY: unit-norace
unit-norace:
	@echo 'Running unit tests without race detection...'
	@mkdir -p .cover
	@GOFLAGS=$(GOFLAGS) go test -v ./... -coverprofile .cover/cover.out

.PHONY: deps
deps:
	go get -u ./...
	go mod tidy

	(cd examples && go get -u ./... && go mod tidy)
