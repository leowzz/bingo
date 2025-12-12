.PHONY: help build run test test-verbose test-coverage benchmark benchmark-all benchmark-logger benchmark-comparison clean fmt lint vet install deps tidy run-dev check ci release release-notes

# 变量定义
BINARY_NAME=bingo
MAIN_PACKAGE=.
CONFIG_FILE=config.yaml
GO_TEST_FLAGS=-v
GO_BENCH_FLAGS=-benchmem
GO_BENCH_TIME=3s

# 默认目标
.DEFAULT_GOAL := help

## help: 显示帮助信息
help:
	@echo "可用目标:"
	@echo "  make build          - 构建二进制文件"
	@echo "  make run            - 运行应用（使用默认配置）"
	@echo "  make run-dev        - 开发模式运行（显示详细日志）"
	@echo "  make test           - 运行单元测试"
	@echo "  make test-verbose   - 运行单元测试（详细输出）"
	@echo "  make test-coverage  - 运行单元测试并生成覆盖率报告"
	@echo "  make benchmark      - 运行所有 benchmark 测试"
	@echo "  make benchmark-main - 运行主 benchmark 测试（性能指标）"
	@echo "  make benchmark-all  - 运行所有 benchmark（包含对比测试）"
	@echo "  make benchmark-logger - 仅运行 logger 的 benchmark"
	@echo "  make benchmark-comparison - 运行格式对比 benchmark"
	@echo "  make clean          - 清理构建文件"
	@echo "  make check          - 运行所有检查（fmt, vet, test）"
	@echo "  make ci             - CI 环境运行（测试、覆盖率、构建）"
	@echo "  make fmt            - 格式化代码"
	@echo "  make lint           - 运行 golangci-lint（如果已安装）"
	@echo "  make vet            - 运行 go vet"
	@echo "  make deps           - 下载依赖"
	@echo "  make tidy           - 整理 go.mod"
	@echo "  make install        - 安装到 GOPATH/bin"
	@echo "  make release        - 交叉编译生成各平台二进制文件到 dist 目录"
	@echo "  make release-notes  - 生成 Release 描述（输出到控制台）"

## build: 构建二进制文件
build:
	@echo "构建 $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "构建完成: ./$(BINARY_NAME)"

## run: 运行应用
run:
	@echo "运行应用..."
	@go run $(MAIN_PACKAGE) -config $(CONFIG_FILE)

## run-dev: 开发模式运行
run-dev:
	@echo "开发模式运行应用..."
	@LOG_LEVEL=debug go run $(MAIN_PACKAGE) -config $(CONFIG_FILE)

## test: 运行单元测试
test:
	@echo "运行单元测试..."
	@go test $(GO_TEST_FLAGS) ./...

## test-verbose: 运行单元测试（详细输出）
test-verbose:
	@echo "运行单元测试（详细输出）..."
	@go test -v -cover ./...

## test-coverage: 运行单元测试并生成覆盖率报告
test-coverage:
	@echo "运行单元测试并生成覆盖率报告..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"
	@go tool cover -func=coverage.out | tail -1

## benchmark: 运行所有 benchmark 测试
benchmark:
	@echo "运行 benchmark 测试..."
	@go test -bench=. $(GO_BENCH_FLAGS) -benchtime=$(GO_BENCH_TIME) ./...

## benchmark-main: 运行主 benchmark 测试（性能指标）
benchmark-main:
	@echo "运行主 benchmark 测试（性能指标）..."
	@go test -bench=. $(GO_BENCH_FLAGS) -benchtime=$(GO_BENCH_TIME) .

## benchmark-all: 运行所有 benchmark（包含详细对比）
benchmark-all:
	@echo "运行所有 benchmark 测试（详细输出）..."
	@echo "\n=== Logger Benchmark ==="
	@go test -bench=. $(GO_BENCH_FLAGS) -benchtime=$(GO_BENCH_TIME) ./internal/logger
	@echo "\n=== Engine Benchmark ==="
	@go test -bench=. $(GO_BENCH_FLAGS) -benchtime=$(GO_BENCH_TIME) ./engine || echo "No benchmark in engine"
	@echo "\n=== Executor Benchmark ==="
	@go test -bench=. $(GO_BENCH_FLAGS) -benchtime=$(GO_BENCH_TIME) ./executor || echo "No benchmark in executor"
	@echo "\n=== Listener Benchmark ==="
	@go test -bench=. $(GO_BENCH_FLAGS) -benchtime=$(GO_BENCH_TIME) ./listener || echo "No benchmark in listener"

## benchmark-logger: 仅运行 logger 的 benchmark
benchmark-logger:
	@echo "运行 logger benchmark 测试..."
	@go test -bench=. $(GO_BENCH_FLAGS) -benchtime=$(GO_BENCH_TIME) ./internal/logger

## benchmark-comparison: 运行格式对比 benchmark
benchmark-comparison:
	@echo "运行格式对比 benchmark..."
	@go test -bench=BenchmarkFormatComparison $(GO_BENCH_FLAGS) -benchtime=$(GO_BENCH_TIME) ./internal/logger

## clean: 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@rm -rf dist
	@go clean -cache
	@echo "清理完成"

## fmt: 格式化代码
fmt:
	@echo "格式化代码..."
	@go fmt ./...
	@echo "格式化完成"

## lint: 运行 golangci-lint（如果已安装）
lint:
	@echo "运行 golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint 未安装，跳过..." && exit 0)
	@golangci-lint run ./...

## vet: 运行 go vet
vet:
	@echo "运行 go vet..."
	@go vet ./...
	@echo "go vet 检查完成"

## deps: 下载依赖
deps:
	@echo "下载依赖..."
	@go mod download
	@echo "依赖下载完成"

## tidy: 整理 go.mod
tidy:
	@echo "整理 go.mod..."
	@go mod tidy
	@echo "go.mod 整理完成"

## install: 安装到 GOPATH/bin
install:
	@echo "安装到 GOPATH/bin..."
	@go install $(MAIN_PACKAGE)
	@echo "安装完成"

## check: 运行所有检查（fmt, vet, test）
check: fmt vet test
	@echo "所有检查完成"

## ci: CI 环境运行（测试、覆盖率、构建）
ci: deps tidy fmt vet test-coverage build
	@echo "CI 检查完成"


run_py_tool:
	@echo "运行 py_tool..."
	cd py_tool && uv run db_tool.py

## release: 交叉编译生成各平台二进制文件
release:
	@echo "开始交叉编译各平台二进制文件..."
	@mkdir -p dist
	@echo "目标目录: ./dist"
	@echo ""
	@echo "编译 Linux amd64..."
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)
	@echo "编译 Linux arm64..."
	@GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-linux-arm64 $(MAIN_PACKAGE)
	@echo "编译 macOS amd64..."
	@GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-darwin-amd64 $(MAIN_PACKAGE)
	@echo "编译 macOS arm64..."
	@GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-darwin-arm64 $(MAIN_PACKAGE)
	@echo "编译 Windows amd64..."
	@GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PACKAGE)
	@echo "编译 Windows arm64..."
	@GOOS=windows GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)-windows-arm64.exe $(MAIN_PACKAGE)
	@echo ""
	@echo "交叉编译完成！生成的文件："
	@ls -lh dist/
	@echo ""
	@echo "文件已保存到 ./dist 目录"

## release-notes: 生成 Release 描述
release-notes:
	@bash scripts/generate_release.sh