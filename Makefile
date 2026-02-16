BINARY_NAME=utils
BUILD_DIR=bin
MAIN_PATH=cmd/utils/main.go

.PHONY: all build build-all clean install help submodules-init submodules-update

all: build

build:
	@echo "Building $(BINARY_NAME)..."
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

build-all:
	@echo "Building binaries for all platforms..."
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)

install:
	@echo "Installing $(BINARY_NAME)..."
	go install $(MAIN_PATH)

clean:
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR)

submodules-init:
	@echo "Initializing submodules..."
	git submodule update --init --recursive

submodules-update:
	@echo "Updating submodules to latest..."
	git submodule update --remote --merge

help:
	@echo "Available commands:"
	@echo "  build             - Build binary for current OS"
	@echo "  build-all         - Build binaries for Linux, Mac (Intel/M1), Windows"
	@echo "  install           - Install via go install"
	@echo "  clean             - Remove bin directory"
	@echo "  submodules-init   - Initialize and clone all submodules"
	@echo "  submodules-update - Update submodules to latest versions"
