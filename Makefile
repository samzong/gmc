.PHONY: build install clean test fmt all help

BUILD_DIR=./bin
BINARY_NAME=gma
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILDTIME=$(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
LDFLAGS=-ldflags "-X github.com/samzong/gma/cmd.Version=$(VERSION) -X 'github.com/samzong/gma/cmd.BuildTime=$(BUILDTIME)'"

build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build Command Done"

install: build
	@echo "Installing $(BINARY_NAME) $(VERSION)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "Install Command Done"

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean Command Done"

test:
	@echo "Running tests..."
	@go test -v ./...
	@echo "Test Command Done"

fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@go mod tidy
	@echo "Format Command Done"

all: clean fmt build test

help:
	@echo "Available targets:"
	@echo "  build    - Build the binary"
	@echo "  install  - Build and install the binary to GOPATH"
	@echo "  clean    - Remove build artifacts"
	@echo "  test     - Run tests"
	@echo "  fmt      - Format code and tidy modules"
	@echo "  all      - Clean, format, build, and test"
	@echo "  help     - Show this help message"

.DEFAULT_GOAL := help 