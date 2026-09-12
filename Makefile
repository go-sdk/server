MAKEFLAGS       += --no-print-directory

VERSION         ?= v1.0.0
BIN_DIR         ?= bin

GO_LDFLAGS_PART := -s -w
GO_LDFLAGS_PART += -X "github.com/go-sdk/core/osx.iVersion=$(VERSION)"
GO_LDFLAGS      := -ldflags '$(GO_LDFLAGS_PART)'

PROTOC_GEN_GO_VERSION           ?= v1.36.12 # https://github.com/protocolbuffers/protobuf-go
PROTOC_GEN_GO_GRPC_VERSION      ?= v1.6.2   # https://github.com/grpc/grpc-go
PROTOC_GEN_GRPC_GATEWAY_VERSION ?= v2.30.0  # https://github.com/grpc-ecosystem/grpc-gateway

.PHONY: tidy
tidy:					##@ Tidy go.mod and go.sum.
	@go mod tidy

.PHONY: prepare
prepare:				##@ Install buf local plugins.
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	@go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@$(PROTOC_GEN_GRPC_GATEWAY_VERSION)

.PHONY: generate
generate:				##@ Lint & Generate proto files.
	@if command -v buf >/dev/null 2>&1; then \
		rm -rf tests/pb && rm -f options/*.pb.go && \
		buf lint && buf generate && echo "done."; \
	else \
		echo "buf is not installed. Please install it from https://github.com/bufbuild/buf"; \
		exit 1; \
	fi

.PHONY: lint
lint: tidy				##@ Lint all packages.
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout 5m && \
		echo "done."; \
	else \
		echo "golangci-lint is not installed. Please install it from https://github.com/golangci/golangci-lint"; \
		exit 1; \
	fi

.PHONY: test
test: tidy				##@ Test all packages.
	@if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format testname --format-icons text -- -race -count 1 -failfast -v ./...; \
	else \
		go test -race -count 1 -failfast -v ./...; \
	fi

.PHONY: build
build:					##@ Build tests/server into bin.
	@mkdir -p $(BIN_DIR)
	@go build $(GO_LDFLAGS) -o $(BIN_DIR)/server ./tests/server

.PHONY: run
run: build				##@ Run tests/server example.
	@$(BIN_DIR)/server


.PHONY: help
help:					##@ (Default) Show help.
	@printf "\nUsage: make <command>\n"
	@grep -F -h "##@" $(MAKEFILE_LIST) | grep -F -v grep -F | sed -e 's/\\$$//' | awk 'BEGIN {FS = ":*[[:space:]]*##@[[:space:]]*"}; \
	{ \
		if($$2 == "") \
			pass; \
		else if($$0 ~ /^#/) \
			printf "\n%s\n", $$2; \
		else if($$1 == "") \
			printf "     %-20s%s\n", "", $$2; \
		else \
			printf "\n    \033[34m%-20s\033[0m %s\n", $$1, $$2; \
	}'
	@printf "\n"

.DEFAULT_GOAL := help
