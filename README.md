# BinGo - 基于 Binlog 的数据库变更订阅与事件触发引擎

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 📖 项目简介

**BinGo** 是一个基于 MySQL Binlog 的数据库变更订阅与事件触发引擎，它通过伪装成 MySQL Slave 来旁路监听数据变更，根据预定义的规则自动触发相应的动作。

### 核心价值

将数据库的"数据变更"转化为"业务事件"，实现彻底的业务解耦。

### 解决的问题

1. **缓存一致性难题**：自动删除 Redis 缓存，避免脏读
2. **业务耦合严重**：将业务逻辑从主流程中解耦，提升响应速度
3. **轮询效率低**：实时监听数据变更，替代低效的轮询机制

## 🚀 快速开始

### 前置要求

- Go 1.25 或更高版本
- MySQL 5.7+ 或 MySQL 8.0+（需要开启 Binlog）
- Redis（用于存储断点位点和执行动作）

### 安装

```bash
# 克隆项目
git clone <repository-url>
cd bingo

# 安装依赖
go mod download

# 编译
go build -o bingo main.go
```

### 配置 MySQL

确保 MySQL 已开启 Binlog：

```sql
-- 检查 Binlog 是否开启
SHOW VARIABLES LIKE 'log_bin';

-- 如果未开启，在 my.cnf 中添加：
[mysqld]
log-bin=mysql-bin
binlog-format=ROW
server-id=1
```

### 运行

```bash
# 使用默认配置运行
./bingo

# 或指定配置文件
./bingo -config config.yaml
```

## 🏗️ 架构设计

### 整体架构

```
┌─────────────────┐
│     MySQL       │
│  (Binlog)       │
└────────┬────────┘
         │ (Binlog Stream)
         ▼
┌─────────────────────────┐
│  1. Listener            │
│  (监听器)                │
│  - 伪装 MySQL Slave      │
│  - 清洗标准化数据        │
└────────┬────────────────┘
         │ (标准化 Event: {Table, Action, OldRow, NewRow})
         ▼
┌─────────────────────────┐
│  2. Rule Engine         │
│  (规则引擎)              │
│  - 加载 rules.yaml      │
│  - 条件匹配             │
│  - CEL 表达式评估       │
└────────┬────────────────┘
         │ (匹配的规则)
         ▼
┌─────────────────────────┐
│  3. Action Executor     │
│  (执行器)                │
│  - Redis Adapter        │
│  - Webhook Adapter      │
│  - gRPC Adapter         │
│  - Kafka/RabbitMQ       │
└─────────────────────────┘
```

### 核心组件

#### 1. Listener (监听器)

- **职责**：监听 MySQL Binlog 变更
- **实现**：基于 `go-mysql` 库，伪装成 MySQL Slave
- **输出**：标准化的变更事件

#### 2. Rule Engine (规则引擎)

- **职责**：根据预定义规则匹配变更事件
- **实现**：支持 CEL (Common Expression Language) 表达式
- **功能**：
  - 表过滤
  - 事件类型过滤（INSERT/UPDATE/DELETE）
  - 条件表达式过滤
  - 变量提取

#### 3. Action Executor (执行器)

- **职责**：执行匹配规则对应的动作
- **支持的动作类型**：
  - Redis 操作（DEL/SET/EXPIRE）
  - HTTP Webhook
  - gRPC 调用
  - 消息队列（Kafka/RabbitMQ）
  - 日志记录

## ⚙️ 配置说明

### 配置文件结构

创建 `config.yaml` 文件：

```yaml
# MySQL 连接配置
mysql:
  host: "192.168.177.100"
  port: 3306
  user: "root"
  password: "your_password"
  database: "test"

# Redis 配置
redis:
  addr: "localhost:6379"
  password: ""
  db: 0

# Binlog 位置（可选，用于断点续传）
binlog:
  file: ""  # 留空则从当前位置开始
  position: 0
  use_redis_store: true  # 是否使用 Redis 存储位置（断点续传）
  redis_store_key: "bingo:binlog:position"  # Redis 存储键名

# 规则文件路径
rules_file: "rules.yaml"

# 性能配置
performance:
  # 防抖窗口（毫秒）
  debounce_window: 100
  # 批量处理大小
  batch_size: 100
  # 并发处理数
  concurrency: 10
```

### 规则文件 (rules.yaml)

```yaml
rules:
  # 示例 1: 用户缓存失效
  - id: "user_cache_invalidate"
    name: "用户缓存失效"
    table: "users"
    events: ["UPDATE", "DELETE"]
    # CEL 表达式过滤条件
    filter: "NewRow['status'] != OldRow['status'] || Action == 'DELETE'"
    actions:
      - type: "redis"
        cmd: "DEL"
        key: "cache:user:{{ .ID }}"
    
  # 示例 2: 订单状态变更通知
  - id: "order_status_notify"
    name: "订单状态变更通知"
    table: "orders"
    events: ["UPDATE"]
    filter: "NewRow['status'] == 'completed' && OldRow['status'] != 'completed'"
    actions:
      - type: "webhook"
        url: "https://api.example.com/order/completed"
        method: "POST"
        headers:
          Authorization: "Bearer {{ .Token }}"
        body: |
          {
            "order_id": "{{ .ID }}",
            "user_id": "{{ .UserID }}",
            "status": "{{ .Status }}"
          }
    
  # 示例 3: 用户注册事件
  - id: "user_registered"
    name: "用户注册事件"
    table: "users"
    events: ["INSERT"]
    filter: "true"  # 所有插入都触发
    actions:
      - type: "kafka"
        topic: "user.events"
        key: "{{ .ID }}"
        value: |
          {
            "event": "user.registered",
            "user_id": "{{ .ID }}",
            "email": "{{ .Email }}",
            "timestamp": "{{ .Timestamp }}"
          }
    
  # 示例 4: 批量操作（防抖聚合）
  - id: "batch_cache_clear"
    name: "批量缓存清理"
    table: "products"
    events: ["UPDATE", "DELETE"]
    filter: "true"
    # 启用批量处理
    batch:
      enabled: true
      window: 200  # 200ms 窗口
    actions:
      - type: "redis"
        cmd: "DEL"
        # 批量操作时，key 会从多个事件中聚合
        keys: ["cache:product:{{ .ID }}"]
```

### 规则字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `id` | string | 是 | 规则唯一标识 |
| `name` | string | 否 | 规则名称（用于日志） |
| `table` | string | 是 | 监控的表名（格式：`database.table` 或 `table`） |
| `events` | []string | 是 | 监听的事件类型：`INSERT`、`UPDATE`、`DELETE` |
| `filter` | string | 否 | CEL 表达式，用于条件过滤 |
| `actions` | []Action | 是 | 匹配后执行的动作列表 |
| `batch` | BatchConfig | 否 | 批量处理配置 |

### CEL 表达式

规则引擎支持使用 CEL (Common Expression Language) 进行条件过滤。

#### 可用变量

- `Action`: 事件类型（"INSERT"、"UPDATE"、"DELETE"）
- `Table`: 表名
- `OldRow`: 更新前的行数据（UPDATE/DELETE 时可用）
- `NewRow`: 更新后的行数据（INSERT/UPDATE 时可用）
- `Timestamp`: 事件时间戳

#### 示例

```yaml
# 只处理状态变更
filter: "NewRow['status'] != OldRow['status']"

# 只处理特定用户
filter: "NewRow['user_id'] == 1001"

# 组合条件
filter: "NewRow['status'] == 'active' && NewRow['age'] >= 18"

# 检查字段是否存在
filter: "'email' in NewRow && NewRow['email'] != ''"
```

## 🎯 动作类型详解

### 1. Redis 动作

```yaml
actions:
  - type: "redis"
    cmd: "DEL"           # 支持: DEL, SET, EXPIRE, INCR, DECR
    key: "cache:user:{{ .ID }}"
    value: ""            # SET 命令时使用
    ttl: 3600           # EXPIRE 时使用（秒）
```

**支持的命令**：
- `DEL`: 删除键
- `SET`: 设置键值
- `EXPIRE`: 设置过期时间
- `INCR`: 递增
- `DECR`: 递减

### 2. Webhook 动作

```yaml
actions:
  - type: "webhook"
    url: "https://api.example.com/webhook"
    method: "POST"       # GET, POST, PUT, DELETE
    headers:
      Content-Type: "application/json"
      Authorization: "Bearer {{ .Token }}"
    body: |
      {
        "event": "{{ .Action }}",
        "table": "{{ .Table }}",
        "data": {{ .NewRow | toJson }}
      }
    timeout: 5          # 超时时间（秒）
    retry: 3            # 重试次数
```

### 3. Kafka 动作

```yaml
actions:
  - type: "kafka"
    brokers: ["localhost:9092"]
    topic: "user.events"
    key: "{{ .ID }}"
    value: "{{ .NewRow | toJson }}"
    partition: 0        # 可选，指定分区
```

### 4. gRPC 动作

```yaml
actions:
  - type: "grpc"
    endpoint: "localhost:50051"
    service: "UserService"
    method: "NotifyUserChange"
    request: |
      {
        "user_id": "{{ .ID }}",
        "action": "{{ .Action }}"
      }
```

### 5. 日志动作

```yaml
actions:
  - type: "log"
    level: "info"       # debug, info, warn, error
    format: "json"      # json, text
    message: "User {{ .ID }} status changed to {{ .NewRow['status'] }}"
```

## 🔧 开发指南

### 项目结构

```
bingo/
├── main.go              # 入口文件
├── config/
│   ├── config.go        # 配置加载
│   └── config.yaml      # 配置文件
├── listener/
│   ├── binlog.go        # Binlog 监听器
│   └── event.go         # 事件定义
├── engine/
│   ├── rule.go          # 规则定义
│   ├── matcher.go       # 规则匹配器
│   └── cel.go           # CEL 表达式引擎
├── executor/
│   ├── executor.go      # 执行器接口
│   ├── redis.go         # Redis 执行器
│   ├── webhook.go       # Webhook 执行器
│   ├── kafka.go         # Kafka 执行器
│   └── grpc.go          # gRPC 执行器
├── utils/
│   ├── template.go      # 模板渲染
│   └── debounce.go      # 防抖聚合
└── rules.yaml           # 规则文件
```

### 添加新的动作类型

1. 在 `executor/` 目录下创建新的执行器文件
2. 实现 `ActionExecutor` 接口：

```go
type ActionExecutor interface {
    Execute(ctx context.Context, action Action, event Event) error
    Type() string
}
```

3. 在 `executor/executor.go` 中注册新的执行器

### 扩展规则引擎

规则引擎支持通过插件方式扩展：

```go
// 自定义过滤器
type CustomFilter interface {
    Match(event Event) bool
}

// 注册自定义过滤器
engine.RegisterFilter("custom", customFilter)
```

## 📊 性能优化

### 1. 防抖与聚合

对于高频变更，系统支持在时间窗口内聚合操作：

```yaml
rules:
  - id: "high_frequency_rule"
    batch:
      enabled: true
      window: 100  # 100ms 窗口
      max_size: 1000  # 最大聚合数量
```

### 2. 顺序性保障

基于主键 Hash 分发事件，确保同一实体的变更串行处理：

```yaml
rules:
  - id: "ordered_rule"
    ordering:
      enabled: true
      key_field: "id"  # 用于分片的主键字段
      shards: 10       # 分片数量
```

### 3. 并发控制

```yaml
performance:
  concurrency: 10      # 并发处理数
  queue_size: 10000    # 事件队列大小
  worker_pool_size: 50 # 工作池大小
```

## 🚢 部署指南

### Docker 部署

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o bingo main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/bingo .
COPY --from=builder /app/rules.yaml .
CMD ["./bingo"]
```

### Kubernetes 部署

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bingo
spec:
  replicas: 2
  selector:
    matchLabels:
      app: bingo
  template:
    metadata:
      labels:
        app: bingo
    spec:
      containers:
      - name: bingo
        image: bingo:latest
        env:
        - name: MYSQL_HOST
          value: "mysql-service"
        - name: REDIS_ADDR
          value: "redis-service:6379"
        volumeMounts:
        - name: config
          mountPath: /etc/bingo
      volumes:
      - name: config
        configMap:
          name: bingo-config
```

## 🔍 监控与日志

### 指标监控

系统暴露以下 Prometheus 指标：

- `bingo_events_total`: 总事件数
- `bingo_rules_matched_total`: 规则匹配数
- `bingo_actions_executed_total`: 动作执行数
- `bingo_actions_failed_total`: 动作失败数
- `bingo_processing_duration_seconds`: 处理耗时

### 日志配置

```yaml
logging:
  level: "info"  # debug, info, warn, error
  format: "json" # json, text
  output: "stdout" # stdout, file
  file: "/var/log/bingo/bingo.log"
  max_size: 100  # MB
  max_backups: 10
  max_age: 30    # days
```

## 🐛 故障排查

### 常见问题

#### 1. 无法连接到 MySQL

**症状**：启动时提示连接失败

**解决方案**：
- 检查 MySQL 地址和端口
- 确认 MySQL 用户权限（需要 REPLICATION SLAVE 权限）
- 检查防火墙设置

```sql
-- 检查用户权限
SHOW GRANTS FOR 'root'@'%';

-- 授予必要权限
GRANT REPLICATION SLAVE ON *.* TO 'root'@'%';
FLUSH PRIVILEGES;
```

#### 2. Binlog 位置丢失

**症状**：重启后从错误位置开始读取

**解决方案**：
- 确保 Redis 正常运行（用于存储断点）
- 检查 Redis 中的 `bingo:binlog:position` 键
- 手动设置起始位置：

```yaml
binlog:
  file: "mysql-bin.000001"
  position: 12345
```

#### 3. 规则不匹配

**症状**：数据变更但规则未触发

**解决方案**：
- 检查规则配置中的 `table` 字段是否正确
- 验证 `filter` 表达式语法
- 查看日志中的事件详情

#### 4. 动作执行失败

**症状**：规则匹配但动作未执行

**解决方案**：
- 检查动作配置（URL、认证信息等）
- 查看错误日志
- 验证网络连接

## 📝 最佳实践

### 1. 规则设计

- **单一职责**：每个规则只处理一种场景
- **精确过滤**：使用 `filter` 减少不必要的处理
- **幂等性**：确保动作可以安全重试

### 2. 性能优化

- 对于高频表，使用批量处理
- 合理设置并发数，避免过载
- 使用 Redis Pipeline 批量执行

### 3. 可靠性保障

- 启用断点续传，避免数据丢失
- 配置动作重试机制
- 监控关键指标，及时告警

### 4. 安全建议

- 使用最小权限原则配置 MySQL 用户
- Webhook 使用 HTTPS 和认证
- 敏感信息使用环境变量或密钥管理

## 🤝 贡献指南

欢迎贡献代码！请遵循以下步骤：

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

### 代码规范

- 遵循 Go 官方代码规范
- 使用 `gofmt` 格式化代码
- 添加必要的注释和测试

## 📄 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

## 🔗 相关资源

- [go-mysql 文档](https://github.com/go-mysql-org/go-mysql)
- [CEL 表达式语言](https://github.com/google/cel-spec)
- [MySQL Binlog 格式](https://dev.mysql.com/doc/internals/en/binary-log.html)

## 📧 联系方式

如有问题或建议，请通过以下方式联系：

- 提交 Issue
- 发送邮件至 [leo03wzz@gmail.com]

---

**BinGo** - 让数据库变更成为业务事件的桥梁 🚀

