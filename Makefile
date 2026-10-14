BIN := go-toralizer
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test lint vuln tidy
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/go-toralizer
test:
	go test -race -cover ./...
lint:
	golangci-lint run
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
tidy:
	go mod tidy
