.PHONY: build install clean

BUILD_DIR=./bin
BINARY_NAME=gma
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X github.com/samzong/gma/cmd.Version=$(VERSION)"

build:
	@echo "Build Command..."
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Build Command Done"

install: build
	@echo "Install Command..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "Install Command Done"

clean:
	@echo "Clean Command..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean Command Done"

test:
	@echo "Run Test Command..."
	@go test -v ./...
	@echo "Test Command Done" 