# Action 扩展支持文档

本文档介绍如何通过 BinGo 开放的接口扩展自定义动作执行器。

## 概述

BinGo 提供了灵活的扩展机制，允许开发者实现自定义的动作执行器。通过实现 `ActionExecutor` 接口，你可以添加任何类型的动作处理逻辑，例如：

- 发送消息到自定义消息队列
- 调用第三方 API
- 执行自定义业务逻辑
- 集成其他系统

## 核心接口

### ActionExecutor 接口

所有动作执行器必须实现 `ActionExecutor` 接口：

```go
package executor

import (
    "context"
    "bingo/engine"
    "bingo/listener"
)

// ActionExecutor 动作执行器接口
type ActionExecutor interface {
    // Execute 执行动作
    // ctx: 上下文，用于控制超时和取消
    // action: 动作配置，包含从规则文件中读取的配置
    // event: 数据库变更事件
    Execute(ctx context.Context, action engine.Action, event *listener.Event) error
    
    // Type 返回执行器类型
    // 这个类型必须与规则文件中 action.type 字段匹配
    Type() string
}
```

### 接口说明

#### Execute 方法

- **参数**：
  - `ctx context.Context`：用于控制执行超时和取消
  - `action engine.Action`：动作配置，包含从 YAML 规则文件中读取的所有字段
  - `event *listener.Event`：数据库变更事件，包含表名、操作类型、新旧数据等

- **返回值**：
  - `error`：如果执行失败，返回错误；成功则返回 `nil`

#### Type 方法

- **返回值**：执行器类型字符串，必须与规则文件中 `action.type` 字段匹配
- **示例**：如果返回 `"custom"`，则规则文件中应使用 `type: "custom"`

## 实现步骤

### 步骤 1：定义执行器结构体

创建一个结构体来实现 `ActionExecutor` 接口：

```go
package executor

import (
    "context"
    "fmt"
    
    "bingo/engine"
    "bingo/internal/logger"
    "bingo/listener"
    "bingo/utils"
)

// CustomExecutor 自定义执行器示例
type CustomExecutor struct {
    // 可以在这里添加执行器需要的配置或客户端
    // 例如：HTTP 客户端、数据库连接、消息队列客户端等
}

// Type 返回执行器类型
func (c *CustomExecutor) Type() string {
    return "custom"
}
```

### 步骤 2：实现 Execute 方法

实现 `Execute` 方法来处理动作：

```go
// Execute 执行自定义动作
func (c *CustomExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
    // 1. 验证必需参数
    if action.URL == "" {
        return fmt.Errorf("custom executor: URL 不能为空")
    }
    
    // 2. 使用模板渲染（如果需要）
    // 模板可以使用 event 中的所有字段，例如：{{ .Table }}, {{ .ID }}, {{ .NewRow.Email }}
    url, err := utils.RenderTemplate(action.URL, event)
    if err != nil {
        return fmt.Errorf("渲染 URL 模板失败: %w", err)
    }
    
    // 3. 执行自定义逻辑
    // 例如：发送 HTTP 请求、写入数据库、发送消息等
    logger.Infow("执行自定义动作",
        "type", c.Type(),
        "url", url,
        "table", event.Table,
        "action", event.Action,
    )
    
    // 4. 返回结果
    return nil
}
```

### 步骤 3：在 Action 结构体中添加字段（可选）

如果需要在规则文件中配置自定义参数，可以在 `engine.Action` 结构体中添加字段：

```go
// 在 engine/rule.go 的 Action 结构体中添加
type Action struct {
    // ... 现有字段 ...
    
    // 自定义字段
    CustomField1 string `yaml:"custom_field1,omitempty"`
    CustomField2 int    `yaml:"custom_field2,omitempty"`
}
```

**注意**：如果只是临时使用，也可以直接使用 `Action` 结构体中已有的通用字段（如 `URL`、`Body`、`Headers` 等），无需修改 `Action` 结构体。

### 步骤 4：注册执行器

在应用启动时注册执行器。有两种方式：

#### 方式 1：修改 main.go（推荐用于内置执行器）

在 `main.go` 的 `NewApp` 函数中，找到创建执行器的地方：

```go
// 创建执行器管理器
exec := executor.NewExecutor()

// 注册自定义执行器
exec.Register(&executor.CustomExecutor{})
```

#### 方式 2：通过插件机制（推荐用于外部扩展）

如果执行器在单独的包中，可以在应用初始化时注册：

```go
import (
    "bingo/executor"
    "your-package/custom-executor"
)

func init() {
    // 在应用启动时注册
    // 注意：这需要在 NewExecutor() 之后调用
}
```

**注意**：由于 `NewExecutor()` 在 `main.go` 中创建，外部包无法直接访问。建议通过以下方式之一：

1. 修改 `main.go` 添加注册代码
2. 创建一个初始化函数，在 `main.go` 中调用
3. 使用依赖注入模式

### 步骤 5：在规则文件中使用

在 `rules.yaml` 中配置使用自定义执行器：

```yaml
rules:
  - id: "custom_action_example"
    name: "自定义动作示例"
    database: "test_db"
    table: "users"
    events: ["INSERT", "UPDATE"]
    filter: "NewRow['status'] == 'active'"
    actions:
      - type: "custom"                    # 必须与 Type() 返回值匹配
        url: "https://api.example.com/users/{{ .ID }}"
        custom_field1: "value1"
        custom_field2: 100
```

## 完整示例

### 示例 1：简单的通知执行器

```go
package executor

import (
    "context"
    "fmt"
    "net/http"
    "strings"
    "time"
    
    "bingo/engine"
    "bingo/internal/logger"
    "bingo/listener"
    "bingo/utils"
)

// NotificationExecutor 通知执行器
// 发送通知到外部系统
type NotificationExecutor struct {
    client *http.Client
}

// NewNotificationExecutor 创建通知执行器
func NewNotificationExecutor() *NotificationExecutor {
    return &NotificationExecutor{
        client: &http.Client{
            Timeout: 10 * time.Second,
        },
    }
}

// Type 返回执行器类型
func (n *NotificationExecutor) Type() string {
    return "notification"
}

// Execute 执行通知动作
func (n *NotificationExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
    // 验证必需参数
    if action.URL == "" {
        return fmt.Errorf("notification executor: URL 不能为空")
    }
    
    // 渲染 URL 模板
    url, err := utils.RenderTemplate(action.URL, event)
    if err != nil {
        return fmt.Errorf("渲染 URL 模板失败: %w", err)
    }
    
    // 准备请求体（使用模板渲染）
    body := action.Body
    if body == "" {
        // 如果没有指定 body，使用默认 JSON 格式
        body = `{"table": "{{ .Table }}", "action": "{{ .Action }}", "data": {{ .NewRow | toJson }}}`
    }
    
    renderedBody, err := utils.RenderTemplate(body, event)
    if err != nil {
        return fmt.Errorf("渲染 Body 模板失败: %w", err)
    }
    
    // 创建 HTTP 请求
    req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(renderedBody))
    if err != nil {
        return fmt.Errorf("创建请求失败: %w", err)
    }
    
    // 设置请求头
    req.Header.Set("Content-Type", "application/json")
    if action.Headers != nil {
        for k, v := range action.Headers {
            // 渲染请求头值（支持模板）
            renderedValue, err := utils.RenderTemplate(v, event)
            if err != nil {
                logger.Warnw("渲染请求头失败", "header", k, "error", err)
                renderedValue = v
            }
            req.Header.Set(k, renderedValue)
        }
    }
    
    // 发送请求
    resp, err := n.client.Do(req)
    if err != nil {
        return fmt.Errorf("发送通知失败: %w", err)
    }
    defer resp.Body.Close()
    
    // 检查响应状态
    if resp.StatusCode >= 400 {
        return fmt.Errorf("通知失败，状态码: %d", resp.StatusCode)
    }
    
    logger.Infow("通知发送成功",
        "url", url,
        "table", event.Table,
        "action", event.Action,
    )
    
    return nil
}
```

### 示例 2：数据库写入执行器

```go
package executor

import (
    "context"
    "database/sql"
    "fmt"
    
    "bingo/engine"
    "bingo/internal/logger"
    "bingo/listener"
    "bingo/utils"
    
    _ "github.com/go-sql-driver/mysql"
)

// DatabaseExecutor 数据库写入执行器
// 将事件数据写入到另一个数据库
type DatabaseExecutor struct {
    db *sql.DB
}

// NewDatabaseExecutor 创建数据库执行器
func NewDatabaseExecutor(dsn string) (*DatabaseExecutor, error) {
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        return nil, fmt.Errorf("打开数据库连接失败: %w", err)
    }
    
    // 测试连接
    if err := db.Ping(); err != nil {
        return nil, fmt.Errorf("数据库连接失败: %w", err)
    }
    
    return &DatabaseExecutor{db: db}, nil
}

// Type 返回执行器类型
func (d *DatabaseExecutor) Type() string {
    return "database"
}

// Execute 执行数据库写入
func (d *DatabaseExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
    // 验证必需参数
    if action.URL == "" {
        return fmt.Errorf("database executor: SQL 语句不能为空（使用 URL 字段存储 SQL）")
    }
    
    // 渲染 SQL 模板
    sqlStr, err := utils.RenderTemplate(action.URL, event)
    if err != nil {
        return fmt.Errorf("渲染 SQL 模板失败: %w", err)
    }
    
    // 执行 SQL
    _, err = d.db.ExecContext(ctx, sqlStr)
    if err != nil {
        return fmt.Errorf("执行 SQL 失败: %w", err)
    }
    
    logger.Infow("数据库写入成功",
        "table", event.Table,
        "action", event.Action,
        "sql", sqlStr,
    )
    
    return nil
}

// Close 关闭数据库连接
func (d *DatabaseExecutor) Close() error {
    if d.db != nil {
        return d.db.Close()
    }
    return nil
}
```

### 示例 3：使用现有字段的简单执行器

如果不需要额外的配置字段，可以直接使用 `Action` 结构体中已有的字段：

```go
package executor

import (
    "context"
    "fmt"
    
    "bingo/engine"
    "bingo/internal/logger"
    "bingo/listener"
    "bingo/utils"
)

// SimpleExecutor 简单执行器
// 使用 Action 中已有的字段，无需修改 Action 结构体
type SimpleExecutor struct{}

// Type 返回执行器类型
func (s *SimpleExecutor) Type() string {
    return "simple"
}

// Execute 执行简单动作
func (s *SimpleExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
    // 使用 action.URL 存储自定义配置
    config, err := utils.RenderTemplate(action.URL, event)
    if err != nil {
        return fmt.Errorf("渲染配置失败: %w", err)
    }
    
    // 使用 action.Body 存储自定义数据
    data, err := utils.RenderTemplate(action.Body, event)
    if err != nil {
        return fmt.Errorf("渲染数据失败: %w", err)
    }
    
    logger.Infow("执行简单动作",
        "config", config,
        "data", data,
        "table", event.Table,
    )
    
    // 执行自定义逻辑
    // ...
    
    return nil
}
```

## 模板变量

在动作配置中，可以使用模板变量来动态生成值。模板使用 Go 的 `text/template` 语法。

### 可用变量

模板中可以访问以下变量：

- `.Table`：完整表名（格式：`database.table`）
- `.Schema`：数据库名
- `.TableName`：表名（不含数据库）
- `.Action`：操作类型（`INSERT`、`UPDATE`、`DELETE`）
- `.Timestamp`：事件时间戳
- `.NewRow`：新数据（INSERT/UPDATE 操作）
- `.OldRow`：旧数据（UPDATE/DELETE 操作）

### 字段访问

可以直接访问数据行中的字段：

- `.ID`：从 NewRow 或 OldRow 中获取 `id` 字段
- `.Email`：从 NewRow 或 OldRow 中获取 `email` 字段
- `.NewRow.Email`：明确从 NewRow 获取
- `.OldRow.Email`：明确从 OldRow 获取

### 模板函数

可用的模板函数：

- `toJson(v)`：将值转换为 JSON 字符串
- `upper(s)`：转换为大写
- `lower(s)`：转换为小写
- `trim(s)`：去除首尾空格

### 模板示例

```yaml
actions:
  - type: "custom"
    url: "https://api.example.com/users/{{ .ID }}"
    body: |
      {
        "user_id": {{ .ID }},
        "email": "{{ .Email }}",
        "action": "{{ .Action | lower }}",
        "data": {{ .NewRow | toJson }}
      }
```

## 最佳实践

### 1. 错误处理

- 始终返回有意义的错误信息
- 使用 `fmt.Errorf` 包装错误，保留错误链
- 记录详细的错误日志

```go
if err != nil {
    logger.Errorw("执行动作失败",
        "type", c.Type(),
        "table", event.Table,
        "error", err,
    )
    return fmt.Errorf("执行 %s 动作失败: %w", c.Type(), err)
}
```

### 2. 上下文处理

- 始终检查 `ctx.Done()` 以支持取消
- 使用 `context.WithTimeout` 设置超时
- 在长时间运行的操作中定期检查上下文

```go
select {
case <-ctx.Done():
    return ctx.Err()
default:
    // 继续执行
}
```

### 3. 资源管理

- 对于需要连接池的执行器（如数据库、消息队列），使用单例模式
- 在应用关闭时正确清理资源
- 使用连接池复用连接

```go
var (
    customClients map[string]*CustomClient
    customMutex   sync.RWMutex
)

func init() {
    customClients = make(map[string]*CustomClient)
}
```

### 4. 日志记录

- 使用结构化日志（`logger.Infow`、`logger.Errorw` 等）
- 记录关键操作和错误
- 避免记录敏感信息（密码、令牌等）

```go
logger.Infow("执行动作",
    "type", c.Type(),
    "table", event.Table,
    "action", event.Action,
    "key", "value",
)
```

### 5. 参数验证

- 在执行前验证所有必需参数
- 提供清晰的错误消息
- 使用合理的默认值

```go
if action.URL == "" {
    return fmt.Errorf("URL 不能为空")
}

timeout := action.Timeout
if timeout == 0 {
    timeout = 10 // 默认 10 秒
}
```

### 6. 性能优化

- 避免在 `Execute` 方法中创建重量级对象
- 使用对象池复用临时对象
- 对于 I/O 操作，考虑使用异步处理

### 7. 线程安全

- 如果执行器有共享状态，确保线程安全
- 使用 `sync.RWMutex` 保护共享资源
- 避免在 `Execute` 方法中修改共享状态（除非有适当的锁）

## 测试

为自定义执行器编写单元测试：

```go
package executor

import (
    "context"
    "testing"
    "time"
    
    "bingo/engine"
    "bingo/listener"
)

func TestCustomExecutor_Type(t *testing.T) {
    exec := &CustomExecutor{}
    if exec.Type() != "custom" {
        t.Errorf("Type() = %v, want 'custom'", exec.Type())
    }
}

func TestCustomExecutor_Execute(t *testing.T) {
    exec := &CustomExecutor{}
    
    action := engine.Action{
        Type: "custom",
        URL:  "https://example.com/{{ .ID }}",
    }
    
    event := &listener.Event{
        Table:  "test.users",
        Action: listener.ActionInsert,
        NewRow: map[string]interface{}{
            "id": 123,
        },
    }
    
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    err := exec.Execute(ctx, action, event)
    if err != nil {
        t.Errorf("Execute() error = %v", err)
    }
}
```

## 故障排查

### 执行器未注册

**症状**：执行规则时提示 "未知的动作类型: xxx"

**解决方案**：
1. 检查执行器的 `Type()` 返回值是否与规则文件中的 `type` 匹配
2. 确认执行器已通过 `exec.Register()` 注册
3. 检查注册代码是否在 `NewExecutor()` 之后执行

### 模板渲染失败

**症状**：执行时提示 "渲染模板失败"

**解决方案**：
1. 检查模板语法是否正确
2. 确认模板中使用的变量在事件中存在
3. 使用 `utils.RenderTemplate` 时处理错误

### 参数验证失败

**症状**：执行时提示参数不能为空

**解决方案**：
1. 在规则文件中提供所有必需参数
2. 检查参数名称是否正确（注意 YAML 字段名）
3. 为可选参数提供默认值

## 内置执行器

BinGo 内置了以下执行器，可以直接在规则文件中使用：

### Kafka 执行器

将数据库变更事件发送到 Kafka 主题。

**配置字段**：
- `brokers`：Kafka broker 地址列表（必需）
- `topic`：主题名称（必需，支持模板变量）
- `key`：消息 Key（可选，支持模板变量）
- `value`：消息 Value（可选，支持模板变量，如果不指定则默认使用 NewRow 的 JSON）
- `partition`：分区（可选，0 表示自动选择）
- `kafka_sasl_mechanism`：SASL 认证机制（可选，支持：PLAIN、SCRAM-SHA-256、SCRAM-SHA-512）
- `kafka_username`：Kafka 用户名（可选，使用 SASL 认证时必需）
- `kafka_password`：Kafka 密码（可选，使用 SASL 认证时必需）

**示例（无鉴权）**：
```yaml
actions:
  - type: "kafka"
    brokers:
      - "localhost:9092"
    topic: "events.{{ .Table }}"
    key: "{{ .ID }}"
    value: "{{ .NewRow | toJson }}"
    partition: 0
```

**示例（使用 SASL PLAIN 鉴权）**：
```yaml
actions:
  - type: "kafka"
    brokers:
      - "localhost:9092"
    topic: "events.{{ .Table }}"
    key: "{{ .ID }}"
    value: "{{ .NewRow | toJson }}"
    kafka_sasl_mechanism: "PLAIN"
    kafka_username: "your_username"
    kafka_password: "your_password"
```

**示例（使用 SASL SCRAM-SHA-256 鉴权）**：
```yaml
actions:
  - type: "kafka"
    brokers:
      - "localhost:9092"
    topic: "events.{{ .Table }}"
    key: "{{ .ID }}"
    value: "{{ .NewRow | toJson }}"
    kafka_sasl_mechanism: "SCRAM-SHA-256"
    kafka_username: "your_username"
    kafka_password: "your_password"
```

### RabbitMQ 执行器

将数据库变更事件发送到 RabbitMQ 队列或交换机。

**配置字段**：
- `rabbitmq_url`：RabbitMQ 连接 URL（必需，格式：`amqp://user:pass@host:port/vhost`）
- `rabbitmq_queue`：队列名称（可选，如果指定则直接发送到队列，支持模板变量）
- `rabbitmq_exchange`：交换机名称（可选，如果指定则发送到交换机，支持模板变量）
- `rabbitmq_routing_key`：路由键（可选，用于交换机路由，支持模板变量）
- `rabbitmq_message`：消息内容（可选，支持模板变量，如果不指定则默认使用 NewRow 的 JSON）

**使用队列的示例**：
```yaml
actions:
  - type: "rabbitmq"
    rabbitmq_url: "amqp://guest:guest@localhost:5672/"
    rabbitmq_queue: "events.{{ .Table }}"
    rabbitmq_message: "{{ .NewRow | toJson }}"
```

**使用交换机的示例**：
```yaml
actions:
  - type: "rabbitmq"
    rabbitmq_url: "amqp://guest:guest@localhost:5672/"
    rabbitmq_exchange: "events"
    rabbitmq_routing_key: "{{ .Table }}.{{ .Action }}"
    rabbitmq_message: "{{ .NewRow | toJson }}"
```

**注意**：必须指定 `rabbitmq_queue` 或 `rabbitmq_exchange` 其中之一。

### Webhook 执行器

发送 HTTP 请求到指定的 URL。

**配置字段**：
- `url`：请求 URL（必需，支持模板变量）
- `method`：HTTP 方法（可选，默认为 POST）
- `headers`：请求头（可选，支持模板变量）
- `body`：请求体（可选，支持模板变量）
- `timeout`：超时时间（秒，可选，默认 5 秒）
- `retry`：重试次数（可选，默认 1 次）

**示例**：
```yaml
actions:
  - type: "webhook"
    url: "http://api.example.com/events"
    method: "POST"
    headers:
      Content-Type: "application/json"
      X-Event-Type: "{{ .Action }}"
    body: "{{ .NewRow | toJson }}"
    timeout: 10
    retry: 3
```

### Redis 执行器

执行 Redis 命令。

**配置字段**：
- `cmd`：Redis 命令（必需）
- `key`：Redis key（必需，支持模板变量）
- `keys`：Redis keys 列表（批量操作时使用，支持模板变量）
- `value`：Redis value（可选，支持模板变量）
- `ttl`：过期时间（秒，可选）
- `redis_conn`：Redis 连接名称（可选，从规则配置的 `redis_connections` 中选择）

**示例**：
```yaml
actions:
  - type: "redis"
    cmd: "SET"
    key: "cache:user:{{ .ID }}"
    value: "{{ .NewRow | toJson }}"
    ttl: 3600
    redis_conn: "cache"
```

### Log 执行器

记录日志。

**配置字段**：
- `level`：日志级别（可选，默认为 info，可选值：debug、info、warn、error）
- `message`：日志消息（必需，支持模板变量）
- `format`：日志格式（可选，默认为 text，可选值：text、json）

**示例**：
```yaml
actions:
  - type: "log"
    level: "info"
    message: "事件发生: {{ .Table }}/{{ .Action }}, ID={{ .ID }}"
    format: "json"
```

## 相关文档

- [规则配置说明](../rules.yaml.template) - 了解如何在规则文件中配置动作
- [事件结构说明](../README.md) - 了解 Event 结构的详细信息
- [模板渲染说明](../README.md) - 了解模板变量的详细用法

## 总结

通过实现 `ActionExecutor` 接口，你可以轻松扩展 BinGo 的功能。关键步骤：

1. 实现 `ActionExecutor` 接口（`Execute` 和 `Type` 方法）
2. 在应用启动时注册执行器
3. 在规则文件中使用自定义执行器
4. 编写测试确保功能正常

遵循最佳实践，你的自定义执行器将能够无缝集成到 BinGo 中，为数据库变更事件提供强大的处理能力。

