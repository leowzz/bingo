# Redis 连接配置说明

本文档详细说明 BinGo 中 Redis 连接的配置和使用方式。

## 概述

BinGo 支持两种类型的 Redis 连接：

1. **系统 Redis**：在 `config.yaml` 中配置，用于系统功能（如 Binlog 位置存储）
2. **规则 Redis**：在 `rules.yaml` 中配置，用于规则中的 Redis 动作

## 系统 Redis 配置

系统 Redis 在 `config.yaml` 中配置，主要用于：

- 存储 Binlog 位置信息（断点续传）
- 作为默认连接（如果没有配置规则 Redis）

### 配置示例

```yaml
system_redis:
  addr: "localhost:6379"      # Redis 服务器地址
  password: ""                 # Redis 密码（如果未设置密码则留空）
  db: 0                        # Redis 数据库编号
```

### 使用场景

系统 Redis 主要用于：
- **Binlog 位置存储**：如果启用了 `use_redis_store`，系统会将 Binlog 位置信息存储到系统 Redis 中
- **默认连接**：如果规则中没有指定 `redis_conn`，且没有配置规则 Redis，系统会使用系统 Redis 作为默认连接

## 规则 Redis 配置

规则 Redis 在 `rules.yaml` 中配置，用于规则中的 Redis 动作。

### 配置示例

```yaml
redis_connections:
  - name: "cache"                    # 连接名称
    addr: "localhost:6379"          # Redis 服务器地址
    password: ""                     # Redis 密码（如果未设置密码则留空）
    db: 1                           # Redis 数据库编号

  - name: "session"                  # 另一个连接
    addr: "localhost:6379"
    password: ""
    db: 2
```

### 在规则中使用

```yaml
rules:
  - id: "user_cache_invalidate"
    database: "test_db"
    table: "users"
    events: ["UPDATE", "DELETE"]
    actions:
      - type: "redis"
        cmd: "DEL"
        key: "cache:user:{{ .ID }}"
        redis_conn: "cache"           # 指定使用名为 "cache" 的连接
```

## 连接选择优先级

当规则中的 Redis 动作执行时，连接选择按以下优先级：

1. **如果指定了 `redis_conn`**：
   - 使用指定的连接名称（从规则 Redis 或系统 Redis 中查找）
   - 如果连接不存在，返回错误

2. **如果未指定 `redis_conn`**：
   - 优先使用 `default` 连接（如果存在）
   - 其次使用 `system` 连接（如果存在）
   - 最后使用第一个可用的连接

## 特殊连接名称

### `system`

`system` 是系统保留的连接名称，指向系统 Redis。

```yaml
actions:
  - type: "redis"
    cmd: "INCR"
    key: "stats:orders:total"
    redis_conn: "system"          # 使用系统 Redis
```

**注意**：在 `rules.yaml` 中定义的 Redis 连接不能使用 `system` 作为名称，系统会自动跳过。

### `default`

`default` 是默认连接名称。如果没有配置规则 Redis，系统会自动将系统 Redis 添加为 `default` 连接。

```yaml
actions:
  - type: "redis"
    cmd: "SET"
    key: "key:{{ .ID }}"
    value: "{{ .Value }}"
    # 不指定 redis_conn，使用默认连接
```

## 连接管理

### 连接池

每个 Redis 连接使用独立的连接池，由 `go-redis` 库自动管理。

### 连接更新

当规则文件热重载时，Redis 连接配置会自动更新：

- **新增连接**：自动添加新配置的连接
- **更新连接**：如果连接地址或配置发生变化，会创建新连接并替换旧连接
- **删除连接**：如果连接从配置中移除，会自动关闭并删除（`system` 和 `default` 连接不会被删除）

### 连接测试

在添加连接时，系统会自动测试连接是否可用：

- 如果连接失败，会记录警告日志，但不会中断应用启动
- 如果连接成功，会记录信息日志

## 配置最佳实践

### 1. 分离系统 Redis 和业务 Redis

建议将系统 Redis 和业务 Redis 分开：

```yaml
# config.yaml
system_redis:
  addr: "redis-system:6379"    # 系统 Redis，用于存储 Binlog 位置
  db: 0

# rules.yaml
redis_connections:
  - name: "cache"
    addr: "redis-cache:6379"   # 业务 Redis，用于缓存操作
    db: 1
```

### 2. 使用多个 Redis 实例

可以为不同的业务场景配置不同的 Redis 连接：

```yaml
redis_connections:
  - name: "cache"              # 缓存 Redis
    addr: "redis-cache:6379"
    db: 1
  - name: "session"            # Session Redis
    addr: "redis-session:6379"
    db: 2
  - name: "counter"             # 计数器 Redis
    addr: "redis-counter:6379"
    db: 3
```

### 3. 连接名称规范

建议使用有意义的连接名称：

- ✅ 好的命名：`cache`、`session`、`counter`、`user_data`
- ❌ 不好的命名：`redis1`、`r1`、`test`

### 4. 安全性

- 生产环境建议使用密码保护
- 使用环境变量或密钥管理工具管理敏感信息
- 限制 Redis 的网络访问权限

## 故障排查

### 连接失败

**症状**：日志中出现 "Redis 连接失败" 错误

**解决方案**：
1. 检查 Redis 服务器是否正常运行
2. 检查网络连接（防火墙、端口）
3. 验证地址和端口是否正确
4. 检查密码是否正确

### 连接不存在

**症状**：执行规则时提示 "Redis 连接不存在: xxx"

**解决方案**：
1. 检查 `rules.yaml` 中的 `redis_connections` 是否正确定义
2. 检查规则中的 `redis_conn` 字段是否拼写正确
3. 如果使用 `system`，确保 `config.yaml` 中配置了 `system_redis`
4. 查看日志中的可用连接列表

### 连接断开

**症状**：运行一段时间后 Redis 操作失败

**解决方案**：
1. 检查 Redis 服务器是否稳定
2. 检查网络连接是否稳定
3. `go-redis` 库会自动重连，但如果频繁断开，需要检查 Redis 配置

## 示例配置

### 完整示例

```yaml
# config.yaml
system_redis:
  addr: "localhost:6379"
  password: ""
  db: 0

# rules.yaml
redis_connections:
  - name: "cache"
    addr: "localhost:6379"
    password: ""
    db: 1
  - name: "session"
    addr: "localhost:6379"
    password: ""
    db: 2

rules:
  - id: "user_cache_invalidate"
    database: "test_db"
    table: "users"
    events: ["UPDATE", "DELETE"]
    actions:
      - type: "redis"
        cmd: "DEL"
        key: "cache:user:{{ .ID }}"
        redis_conn: "cache"           # 使用规则中定义的 "cache" 连接

  - id: "order_statistics"
    database: "test_db"
    table: "orders"
    events: ["INSERT"]
    actions:
      - type: "redis"
        cmd: "INCR"
        key: "stats:orders:total"
        redis_conn: "system"          # 使用系统 Redis
```

## 相关文档

- [规则配置说明](../rules.yaml.template)
- [配置文件说明](../config.yaml.template)
- [README](../README.md)

