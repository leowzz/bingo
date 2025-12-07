# 测试文档

本文档说明 BinGo 项目的测试结构、运行方法和最佳实践。

## 测试结构

BinGo 项目使用 Go 标准测试框架，测试文件遵循 `*_test.go` 命名规范。

### 测试文件分布

```
bingo/
├── main_test.go              # 主程序测试（EventHandler、Batch、Ordering）
├── benchmark_test.go          # 性能基准测试
├── config/
│   └── config_test.go         # 配置加载测试
├── engine/
│   ├── matcher_test.go        # 规则匹配器测试
│   └── cel_test.go            # CEL 表达式引擎测试
├── executor/
│   ├── executor_test.go        # 执行器管理器测试
│   ├── redis_test.go           # Redis 执行器测试
│   └── log_test.go             # 日志执行器测试
├── listener/
│   └── event_test.go          # 事件处理测试
└── utils/
    └── template_test.go        # 模板渲染测试
```

## 运行测试

### 运行所有测试

```bash
# 运行所有测试
go test ./...

# 运行所有测试（详细输出）
go test -v ./...

# 运行所有测试（带覆盖率）
go test -v -cover ./...
```

### 运行特定包的测试

```bash
# 运行 executor 包的测试
go test ./executor

# 运行 engine 包的测试
go test ./engine
```

### 运行特定测试函数

```bash
# 运行 TestEventHandler_Batch 测试
go test -v -run TestEventHandler_Batch

# 运行所有 TestEventHandler 开头的测试
go test -v -run TestEventHandler
```

### 使用 Makefile

项目提供了 Makefile 来简化测试命令：

```bash
# 运行单元测试
make test

# 运行单元测试（详细输出）
make test-verbose

# 运行单元测试并生成覆盖率报告
make test-coverage
```

## 覆盖率报告

### 生成覆盖率报告

```bash
# 生成覆盖率文件
go test -coverprofile=coverage.out ./...

# 生成 HTML 报告
go tool cover -html=coverage.out -o coverage.html

# 查看覆盖率统计
go tool cover -func=coverage.out
```

### 使用 Makefile

```bash
# 生成覆盖率报告（自动打开 HTML）
make test-coverage
```

生成的 `coverage.html` 文件可以在浏览器中打开，查看详细的代码覆盖率。

## 性能基准测试

### 运行基准测试

```bash
# 运行所有基准测试
go test -bench=. -benchmem

# 运行基准测试（指定时间）
go test -bench=. -benchmem -benchtime=3s

# 运行特定基准测试
go test -bench=BenchmarkMatcher -benchmem
```

### 使用 Makefile

```bash
# 运行所有 benchmark 测试
make benchmark

# 运行主 benchmark 测试
make benchmark-main
```

### 基准测试说明

基准测试文件 `benchmark_test.go` 包含以下测试：

- `BenchmarkMatcher`: 规则匹配性能
- `BenchmarkCEL`: CEL 表达式评估性能
- `BenchmarkTemplate`: 模板渲染性能

## 测试最佳实践

### 1. 单元测试

- **单一职责**：每个测试函数只测试一个功能点
- **独立性**：测试之间不应该有依赖关系
- **可重复性**：测试应该可以重复运行，结果一致
- **快速执行**：单元测试应该快速完成

### 2. 测试命名

遵循 Go 测试命名规范：

```go
// 测试函数命名：Test + 被测试的函数/类型 + 测试场景
func TestEventHandler_Batch(t *testing.T) { }
func TestRedisExecutor_Execute_DEL(t *testing.T) { }
```

### 3. 表驱动测试

对于多个测试场景，使用表驱动测试：

```go
func TestEvent_GetField(t *testing.T) {
    tests := []struct {
        name     string
        event    *Event
        field    string
        expected interface{}
    }{
        {
            name: "从 NewRow 获取字段",
            event: &Event{...},
            field: "id",
            expected: 123,
        },
        // ... 更多测试用例
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := tt.event.GetField(tt.field)
            if result != tt.expected {
                t.Errorf("GetField() = %v, want %v", result, tt.expected)
            }
        })
    }
}
```

### 4. Mock 对象

对于外部依赖，使用 Mock 对象：

```go
// mockExecutor 模拟执行器，用于测试
type mockExecutor struct {
    executedActions []executedAction
    mu              sync.Mutex
}
```

### 5. 测试辅助函数

提取公共的测试逻辑到辅助函数：

```go
// createTestEvent 创建测试事件
func createTestEvent(table string, action listener.Action, id interface{}) *listener.Event {
    // ...
}

// createTestMatcher 创建测试匹配器
func createTestMatcher(rules []engine.Rule) *engine.Matcher {
    // ...
}
```

### 6. 清理资源

测试完成后清理资源：

```go
func TestSomething(t *testing.T) {
    handler := NewEventHandler(...)
    handler.Start()
    defer handler.Stop()  // 确保资源被清理
    // ... 测试代码
}
```

## 测试覆盖的功能

### 已覆盖的功能

- ✅ 规则匹配器（`engine/matcher_test.go`）
- ✅ CEL 表达式引擎（`engine/cel_test.go`）
- ✅ Redis 执行器（`executor/redis_test.go`）
- ✅ 日志执行器（`executor/log_test.go`）
- ✅ 事件处理（`listener/event_test.go`）
- ✅ 模板渲染（`utils/template_test.go`）
- ✅ 配置加载（`config/config_test.go`）
- ✅ EventHandler Batch 功能（`main_test.go`）
- ✅ EventHandler Ordering 功能（`main_test.go`）

### 需要补充的测试

- ⚠️ Webhook 执行器（需要 Mock HTTP 服务器）
- ⚠️ Kafka 执行器（需要 Mock Kafka 服务器）
- ⚠️ RabbitMQ 执行器（需要 Mock RabbitMQ 服务器）
- ⚠️ 规则文件热重载（需要文件系统操作）

## 持续集成

### CI 配置示例

项目可以在 CI 环境中运行测试：

```yaml
# .github/workflows/test.yml
name: Test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: '1.25'
      - run: go test -v -coverprofile=coverage.out ./...
      - run: go tool cover -func=coverage.out
```

## 故障排查

### 测试失败

如果测试失败，检查：

1. **日志输出**：使用 `-v` 参数查看详细输出
2. **测试环境**：确保测试环境配置正确
3. **依赖服务**：某些测试可能需要外部服务（Redis、MySQL 等）

### 测试超时

如果测试超时：

1. 检查是否有死锁或阻塞
2. 增加超时时间：`go test -timeout 30s`
3. 检查异步操作的等待时间

## 相关资源

- [Go 测试文档](https://golang.org/pkg/testing/)
- [Go 基准测试](https://golang.org/pkg/testing/#hdr-Benchmarks)
- [Makefile](../Makefile) - 项目 Makefile 包含测试相关命令

