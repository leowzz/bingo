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
4. **配置更新不便**：支持规则文件热重载，无需重启即可更新规则

## 🚀 快速开始

### 前置要求

- Go 1.25 或更高版本
- MySQL 5.7+ 或 MySQL 8.0+（需要开启 Binlog）
- Redis（用于存储断点位置和执行动作，支持多个 Redis 连接）

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
┌──────────────────────────────┐
│  1. Listener                 │
│  - Acts as MySQL Slave       │
│  - Cleans & normalizes data  │
└────────────┬─────────────────┘
             │ (Normalized Event: {Table, Action, OldRow, NewRow})
             ▼
┌──────────────────────────────┐
│  2. Rule Engine              │
│  - Loads rules.yaml          │
│  - Condition matching        │
│  - CEL expression evaluation │
└────────────┬─────────────────┘
             │ (Matched rules)
             ▼
┌─────────────────────────┐
│  3. Action Executor     │
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

### 快速开始

1. **复制配置文件模板**：
   ```bash
   cp config.yaml.template config.yaml
   cp rules.yaml.template rules.yaml
   ```

2. **编辑配置文件**：
   - 修改 `config.yaml` 中的 MySQL、Redis 等连接信息
   - 根据业务需求在 `rules.yaml` 中添加规则

3. **详细配置说明**：
   - 配置文件字段说明请查看 `config.yaml.template` 中的注释
   - 规则配置说明请查看 `rules.yaml.template` 中的注释

4. **规则文件热重载**：
   - 修改 `rules.yaml` 后保存，系统会自动检测并重载规则
   - 无需重启应用，规则变更实时生效

### 配置文件结构

#### config.yaml
应用主配置文件，包含：
- MySQL 连接配置
- 系统 Redis 配置（用于存储 Binlog 位置等系统功能）
- Binlog 位置配置
- 性能参数配置
- 日志配置

详细字段说明请参考 `config.yaml.template` 文件中的注释。

#### rules.yaml
规则配置文件，定义数据变更触发的动作：
- Redis 连接配置（可选，支持多个连接）
- 表过滤规则
- 事件类型过滤（INSERT/UPDATE/DELETE）
- 条件过滤（CEL 表达式）
- 执行动作（Redis/Webhook/Log 等）

**Redis 连接配置**：
- 可以在 `rules.yaml` 中定义多个 Redis 连接
- 规则中的 Redis 动作可以指定使用哪个连接
- 也可以使用 `redis_conn: "system"` 使用系统 Redis

详细配置说明请参考 `rules.yaml.template` 文件中的注释。

### 规则文件热重载

BinGo 支持规则文件的热重载功能，无需重启应用即可更新规则配置。

#### 功能特性

- **自动检测**：自动监听 `rules.yaml` 文件的变化
- **实时生效**：文件修改后自动重新加载规则
- **防抖机制**：500ms 防抖时间，避免频繁触发
- **错误处理**：如果新规则加载失败，自动保持旧规则不变
- **连接管理**：自动更新 Redis 连接配置（添加、更新、删除）

#### 使用方法

1. **启动应用**：正常启动 BinGo 应用
2. **修改规则**：直接编辑 `rules.yaml` 文件
3. **自动重载**：保存文件后，系统会自动检测并重载规则

#### 重载日志

当规则文件发生变化时，会在日志中看到类似以下信息：

```
检测到规则文件变化，开始重载...
已更新 5 条规则
更新 Redis 连接: cache
规则文件重载成功，共 5 条规则
```

#### 注意事项

- 规则文件路径由 `config.yaml` 中的 `rules_file` 配置项指定
- 如果规则文件格式错误，会记录错误日志并保持旧规则
- 如果规则文件被删除，会记录警告日志并保持旧规则
- Redis 连接配置会自动同步更新（系统保留的连接 "system" 和 "default" 不会被删除）

### 规则配置要点

- **表名格式**：支持 `database.table` 或 `table` 两种格式
- **事件类型**：`INSERT`、`UPDATE`、`DELETE`
- **过滤条件**：使用 CEL 表达式，留空或 `"true"` 表示匹配所有
- **模板变量**：在动作配置中使用 `{{ .FieldName }}` 访问事件数据
  - `{{ .ID }}` - 主键字段值
  - `{{ .Table }}` - 表名
  - `{{ .Action }}` - 操作类型
  - `{{ .NewRow }}` / `{{ .OldRow }}` - 行数据
  - `{{ .FieldName }}` - 任意字段（首字母大写）

## 🎯 动作类型详解

### 1. Redis 动作

Redis 动作支持多个 Redis 连接配置，可以在规则中指定使用哪个连接。

#### 配置 Redis 连接

在 `rules.yaml` 中定义 Redis 连接：

```yaml
redis_connections:
  - name: "cache"                    # 连接名称
    addr: "localhost:6379"          # Redis 服务器地址
    password: ""                     # Redis 密码
    db: 1                            # Redis 数据库编号
  - name: "session"                  # 另一个连接
    addr: "localhost:6379"
    password: ""
    db: 2
```

#### 使用 Redis 动作

```yaml
actions:
  - type: "redis"
    cmd: "DEL"                       # 支持: DEL, SET, EXPIRE, INCR, DECR
    key: "cache:user:{{ .ID }}"
    redis_conn: "cache"              # 指定使用哪个连接（可选）
                                     # - 不指定或 "default": 使用默认连接
                                     # - "system": 使用系统 Redis（config.yaml 中的 system_redis）
                                     # - 其他: 使用 rules.yaml 中定义的连接名称
    value: ""                        # SET 命令时使用
    ttl: 3600                        # SET/EXPIRE 时使用（秒）
    keys: []                         # DEL 批量删除时使用（可选）
```

**支持的命令**：
- `DEL`: 删除键（支持单个 key 或批量 keys）
- `SET`: 设置键值（支持 TTL，使用 SetEx）
- `EXPIRE`: 设置过期时间
- `INCR`: 递增
- `DECR`: 递减

**连接选择优先级**：
1. 如果指定了 `redis_conn`，使用指定的连接
2. 如果未指定，按以下顺序选择：
   - `default` 连接（如果存在）
   - `system` 连接（如果存在）
   - 第一个可用的连接

**系统 Redis**：
- 系统 Redis 在 `config.yaml` 中配置为 `system_redis`
- 主要用于存储 Binlog 位置等系统功能
- 规则可以通过 `redis_conn: "system"` 使用系统 Redis

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
│   ├── reloader.go      # 规则文件热重载
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

**功能说明**：
- 当规则启用 `batch` 配置时，匹配的事件会被收集到批量收集器中
- 在时间窗口（`window`）内或达到最大数量（`max_size`）时，批量执行动作
- 批量 key 基于表名和动作类型生成，确保同一表同一动作类型的事件被聚合

**实现细节**：
- ✅ `BatchConfig` 结构体已定义（`engine/rule.go`）
- ✅ `BatchCollector` 工具类已实现（`utils/debounce.go`）
- ✅ 事件处理流程中已集成（`main.go` 的 `EventHandler`）

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

**功能说明**：
- 当规则启用 `ordering` 配置时，事件会根据主键字段值进行 Hash 分片
- 相同主键值的事件会被分发到同一个分片队列，确保串行处理
- 每个分片有独立的 goroutine 处理，不同分片可以并行处理

**实现细节**：
- ✅ `OrderingConfig` 结构体已定义（`engine/rule.go`）
- ✅ 基于 FNV-1a Hash 算法的分片计算（`main.go` 的 `calculateShard`）
- ✅ 分片队列和独立处理 goroutine 已实现（`main.go` 的 `orderingShardWorker`）

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
- 确保系统 Redis 正常运行（用于存储断点）
- 检查系统 Redis 中的 `bingo:binlog:position` 键（支持模板变量 `{{ .ConnCfgHash }}`）
- 手动设置起始位置：

```yaml
binlog:
  file: "mysql-bin.000001"
  position: 12345
  redis_store_key: "bingo:binlog:position:{{ .ConnCfgHash }}"  # 支持模板变量
```

#### 2.1. 规则文件热重载失败

**症状**：修改规则文件后未生效

**解决方案**：
- 检查日志中是否有重载错误信息
- 验证规则文件格式是否正确（YAML 语法）
- 确认规则文件路径是否正确（`config.yaml` 中的 `rules_file`）
- 检查文件权限，确保应用有读取权限

#### 2.2. Redis 连接不存在

**症状**：规则执行时提示 "Redis 连接不存在: xxx"

**解决方案**：
- 检查 `rules.yaml` 中的 `redis_connections` 是否正确定义
- 检查规则中的 `redis_conn` 字段是否拼写正确
- 如果使用 `system`，确保 `config.yaml` 中配置了 `system_redis`
- 查看日志中的可用连接列表

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
- 使用规则文件热重载，无需重启即可更新规则

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

## 📚 更多文档

- [Redis 连接配置说明](REDIS_CONNECTIONS.md) - 详细的 Redis 连接配置和使用指南
- [测试文档](TESTING.md) - 单元测试说明和覆盖率报告

## 📧 联系方式

如有问题或建议，请通过以下方式联系：

- 提交 Issue
- 发送邮件至 [leo03wzz@gmail.com]

---

**BinGo** - 让数据库变更成为业务事件的桥梁 🚀

