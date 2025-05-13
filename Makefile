.PHONY: build install clean

BUILD_DIR=./bin
BINARY_NAME=gma
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X github.com/samzong/gma/cmd.Version=$(VERSION)"

build:
	@echo "构建 GMA..."
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "构建完成: $(BUILD_DIR)/$(BINARY_NAME)"

install: build
	@echo "安装 GMA..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "安装完成: $(GOPATH)/bin/$(BINARY_NAME)"

clean:
	@echo "清理..."
	@rm -rf $(BUILD_DIR)
	@echo "清理完成"

test:
	@echo "运行测试..."
	@go test -v ./...
	@echo "测试完成" 