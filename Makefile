.PHONY: build test clean install fmt lint vet deps coverage help

BINARY_NAME := contextspectre
BUILD_DIR   := ./bin
MAIN_PATH   := ./cmd/contextspectre
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION_NUM  = $(patsubst v%,%,$(VERSION))
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w -X main.version=$(VERSION_NUM) -X main.commit=$(COMMIT) -X main.date=$(DATE)

## help: Display this help message
help:
	@echo "Available targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'

## build: Build the binary
build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

## test: Run tests with race detection
test:
	go test -race ./...

## vet: Run go vet
vet:
	go vet ./...

## lint: Run golangci-lint
lint:
	golangci-lint run --timeout=5m

## fmt: Format code
fmt:
	gofmt -w .
	@command -v goimports >/dev/null 2>&1 && goimports -w . || true

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR) dist/

## install: Install the binary to GOPATH
install:
	go install -ldflags="$(LDFLAGS)" $(MAIN_PATH)

## deps: Download and tidy dependencies
deps:
	go mod download
	go mod tidy

## coverage: Run tests with coverage report
coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

.DEFAULT_GOAL := help
