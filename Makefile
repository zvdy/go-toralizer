# go-toralizer development tasks.
#
# `make help` lists every target.

BINARY      := go-toralizer
PKG         := github.com/zvdy/go-toralizer
CMD         := ./cmd/$(BINARY)
BIN_DIR     := bin
GOBIN       := $(shell go env GOPATH)/bin

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
DATE        ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKG)/internal/buildinfo.version=$(VERSION) \
	-X $(PKG)/internal/buildinfo.commit=$(COMMIT) \
	-X $(PKG)/internal/buildinfo.date=$(DATE)

GOLANGCI_LINT_VERSION := v2.13.2

.DEFAULT_GOAL := help
.PHONY: help build install run test test-race cover lint fmt vet tidy vulncheck tools clean ci

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Targets:\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build the binary into ./bin
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) $(CMD)

install: ## Install the binary into $(GOBIN)
	go install -trimpath -ldflags '$(LDFLAGS)' $(CMD)

run: build ## Build and show the binary's help output
	@$(BIN_DIR)/$(BINARY) --help

test: ## Run the test suite
	go test -shuffle=on -timeout 5m ./...

test-race: ## Run the test suite under the race detector
	go test -race -shuffle=on -timeout 5m ./...

cover: ## Run tests and report coverage per function
	go test -race -timeout 5m -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

lint: tools ## Run golangci-lint
	$(GOBIN)/golangci-lint run ./...

fmt: ## Format the source tree
	gofmt -w .

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy go.mod and go.sum
	go mod tidy

vulncheck: ## Scan dependencies for known vulnerabilities
	GOTOOLCHAIN=local go run golang.org/x/vuln/cmd/govulncheck@latest ./...

tools: ## Install the development tools this Makefile needs
	@test -x $(GOBIN)/golangci-lint || \
	  GOTOOLCHAIN=local go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

ci: lint vet test-race ## Run everything CI runs

clean: ## Remove build output
	rm -rf $(BIN_DIR) coverage.out dist
