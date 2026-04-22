# ComposeBoard Makefile
# 作者：凌封
# 网址：https://fengin.cn

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS = -s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)
BINARY = composeboard
OUTPUT_DIR = bin

.PHONY: all build build-linux build-arm build-all clean version i18n-check

# 默认: 编译当前平台
all: build

# 编译当前平台
build:
	go build -ldflags "$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY) .

# 交叉编译: Linux amd64
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY)-linux-amd64 .
	@echo "✓ 编译完成: $(OUTPUT_DIR)/$(BINARY)-linux-amd64"

# 交叉编译: Linux arm64
build-arm:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY)-linux-arm64 .
	@echo "✓ 编译完成: $(OUTPUT_DIR)/$(BINARY)-linux-arm64"

# 编译所有平台
build-all: i18n-check build-linux build-arm
	@echo "$(VERSION)" > $(OUTPUT_DIR)/VERSION
	@echo "✓ 所有平台编译完成, 版本: $(VERSION)"

# i18n key 一致性检查
i18n-check:
	@node scripts/check-i18n-keys.js

# 清理编译产物
clean:
	rm -rf $(OUTPUT_DIR)

# 查看版本
version:
	@echo $(VERSION)
