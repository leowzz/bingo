# 高可用（HA）配置说明

本文档详细说明 BinGo 的高可用（High Availability）功能，包括配置、工作原理和使用场景。

## 概述

BinGo 的高可用功能通过 Redis 分布式锁实现主从切换机制，确保同一时间只有一个实例在处理 Binlog 事件，从而实现服务的高可用部署。

### 核心特性

- **主从自动切换**：通过分布式锁实现主节点选举，从节点自动进入 Standby 模式
- **锁内容校验**：每次续期和校验时检查锁内容，防止脑裂（Split-Brain）问题
- **自动续期**：主节点定期续期锁，防止因网络抖动导致锁过期
- **优雅降级**：如果 Redis 连接失败，服务仍可正常启动（单实例模式）
- **模板支持**：锁键名支持模板语法，确保不同 MySQL 连接使用不同的锁

## 工作原理

### 架构设计

```
┌─────────────────────────────────────────────────────────┐
│                    Redis (分布式锁)                      │
│  Key: bingo:ha:lock:{{ .ConnCfgHash }}                  │
│  Value: {"instance_id": "hostname-12345", ...}          │
└─────────────────────────────────────────────────────────┘
                    ▲              ▲
                    │              │
         ┌──────────┴──────┐      ┌┴───────────────┐
         │  主节点 (Leader) │      │ 从节点 (Standby)│
         │                 │      │                │
         │ - 持有锁         │      │ - 等待获取锁    │
         │ - 处理 Binlog    │      │ - 监控锁状态    │
         │ - 定期续期       │      │ - 定期重试      │
         └─────────────────┘      └────────────────┘
```

### 状态转换

```
启动
  │
  ├─[启用 HA]─→ 尝试获取锁
  │              │
  │              ├─[成功]─→ 主节点模式
  │              │           │
  │              │           ├─ 启动锁续期
  │              │           ├─ 启动锁校验
  │              │           ├─ 启动 Binlog 监听
  │              │           └─ 监控锁状态
  │              │
  │              └─[失败]─→ Standby 模式
  │                            │
  │                            └─ 定期重试获取锁
  │
  └─[未启用 HA]─→ 单实例模式（直接启动 Binlog 监听）

主节点模式
  │
  ├─[锁续期失败]─→ Standby 模式
  ├─[锁校验失败]─→ Standby 模式
  └─[锁被其他实例获取]─→ Standby 模式
```

## 配置说明

### 基本配置

在 `config.yaml` 中添加 `ha` 配置项：

```yaml
# 高可用配置
ha:
  enabled: true                    # 是否启用高可用，默认 false
  lock_key: "bingo:ha:lock:{{ .ConnCfgHash }}"  # 锁的 Redis key
  lock_ttl: 15                      # 锁过期时间（秒），默认 15
  refresh_interval: 5                # 锁续期间隔（秒），默认 5
```

### 配置项说明

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `enabled` | bool | `false` | 是否启用高可用功能 |
| `lock_key` | string | `"bingo:ha:lock:{{ .ConnCfgHash }}"` | 锁的 Redis key，支持模板语法 |
| `lock_ttl` | int | `15` | 锁过期时间（秒），建议 15-30 秒 |
| `refresh_interval` | int | `5` | 锁续期间隔（秒），建议为 `lock_ttl` 的 1/3 |

### 模板语法

`lock_key` 支持模板语法，可以使用 `{{ .ConnCfgHash }}` 自动替换为 MySQL 连接配置的 hash 值。

**为什么需要模板？**

不同 MySQL 连接应该使用不同的锁，避免多个 BinGo 实例监听不同 MySQL 时相互干扰。

**示例：**

```yaml
# 配置 1：监听 MySQL A
mysql:
  host: "mysql-a.example.com"
  port: 3306
  user: "repl"
  password: "password1"

ha:
  lock_key: "bingo:ha:lock:{{ .ConnCfgHash }}"
  # 实际生成的 key: bingo:ha:lock:abc123def456

# 配置 2：监听 MySQL B
mysql:
  host: "mysql-b.example.com"
  port: 3306
  user: "repl"
  password: "password2"

ha:
  lock_key: "bingo:ha:lock:{{ .ConnCfgHash }}"
  # 实际生成的 key: bingo:ha:lock:789ghi012jkl
```

### 配置建议

1. **锁过期时间（lock_ttl）**
   - 建议值：15-30 秒
   - 太短：网络抖动可能导致频繁切换
   - 太长：故障恢复时间较长

2. **续期间隔（refresh_interval）**
   - 建议值：`lock_ttl` 的 1/3
   - 例如：`lock_ttl=15` 时，`refresh_interval=5`
   - 确保在锁过期前完成续期

3. **系统 Redis 配置**
   - 高可用功能依赖系统 Redis
   - 确保系统 Redis 配置正确且连接稳定
   - 建议使用 Redis 集群或哨兵模式提高可用性

## 锁内容校验（防止脑裂）

### 什么是脑裂？

脑裂（Split-Brain）是指在高可用系统中，由于网络分区等原因，多个节点都认为自己是主节点，导致数据不一致。

### BinGo 如何防止脑裂？

BinGo 通过锁内容校验机制防止脑裂：

1. **锁内容结构**
   ```json
   {
     "instance_id": "hostname-12345",
     "timestamp": 1234567890,
     "acquired_at": "2024-01-01T12:00:00Z"
   }
   ```

2. **校验时机**
   - **续期时**：每次续期前，先读取锁内容，校验实例ID是否匹配
   - **定期校验**：独立的校验 goroutine 定期检查锁内容

3. **校验逻辑**
   - 如果锁内容中的 `instance_id` 与当前实例不匹配，说明锁已被其他实例获取
   - 当前实例立即释放资源，进入 Standby 模式

### 示例场景

```
时间线：
T1: 实例 A 获取锁，instance_id = "host-A-12345"
T2: 实例 A 网络抖动，锁续期失败
T3: 锁过期（15秒后）
T4: 实例 B 获取锁，instance_id = "host-B-67890"
T5: 实例 A 网络恢复，尝试续期
T6: 实例 A 读取锁内容，发现 instance_id = "host-B-67890" ≠ "host-A-12345"
T7: 实例 A 立即停止 Binlog 监听，进入 Standby 模式
```

## 使用场景

### 场景 1：多实例部署

在生产环境中部署多个 BinGo 实例，确保服务高可用：

```yaml
# 实例 1 配置
ha:
  enabled: true
  lock_key: "bingo:ha:lock:{{ .ConnCfgHash }}"

# 实例 2 配置（相同配置）
ha:
  enabled: true
  lock_key: "bingo:ha:lock:{{ .ConnCfgHash }}"
```

**行为：**
- 启动时，两个实例同时尝试获取锁
- 只有一个实例能获取到锁，成为主节点
- 另一个实例进入 Standby 模式，定期重试
- 如果主节点故障，Standby 节点自动接管

### 场景 2：滚动更新

在 Kubernetes 等容器环境中进行滚动更新：

1. 新 Pod 启动，尝试获取锁
2. 如果锁被旧 Pod 持有，新 Pod 进入 Standby 模式
3. 旧 Pod 停止时释放锁
4. 新 Pod 获取锁，成为主节点

### 场景 3：单实例模式

如果不需要高可用，可以禁用 HA：

```yaml
ha:
  enabled: false  # 或直接不配置 ha 项
```

**行为：**
- 服务直接启动 Binlog 监听
- 不进行锁获取和续期
- 适合开发环境或单实例部署

## 故障处理

### 主节点故障

**场景：** 主节点进程崩溃或网络中断

**处理流程：**
1. 锁过期（15秒后）
2. Standby 节点检测到锁已释放
3. Standby 节点获取锁，成为新的主节点
4. 新主节点从 Redis 加载 Binlog 位置，继续处理

**恢复时间：** 约 15-20 秒（锁过期时间 + 重试间隔）

### Redis 连接失败

**场景：** 系统 Redis 连接失败

**处理流程：**
1. 初始化时检测到 Redis 连接失败
2. 记录警告日志，但不阻止服务启动
3. 如果 HA 已启用，会禁用 HA 功能
4. 服务以单实例模式运行

**日志示例：**
```
WARN 系统 Redis 连接失败: connection refused
WARN 高可用已启用，但系统 Redis 未配置或连接失败，将禁用高可用
```

### 锁续期失败

**场景：** 主节点锁续期失败（网络抖动、Redis 临时不可用）

**处理流程：**
1. 续期失败，记录错误日志
2. 续期 goroutine 退出
3. 锁监控 goroutine 检测到不再是 Leader
4. 停止 Binlog 监听，进入 Standby 模式
5. 定期重试获取锁

**注意事项：**
- 如果只是短暂网络抖动，锁可能还未过期
- Standby 节点可能无法立即获取锁（需要等待锁过期）
- 建议配置 Redis 高可用（集群或哨兵）减少此类问题

## 监控和日志

### 关键日志

**启动时：**
```
INFO 已初始化分布式锁管理器，锁键: bingo:ha:lock:abc123, TTL: 15秒
INFO 成功获取分布式锁 instance_id=hostname-12345
INFO 已启动锁续期和校验机制 refresh_interval=5s validate_interval=2s
```

**Standby 模式：**
```
INFO 未获取到分布式锁，进入 Standby 模式，将定期重试
INFO 进入 Standby 模式，等待获取分布式锁...
```

**锁状态变化：**
```
WARN 当前实例不再是 Leader，停止 Binlog 监听
INFO 成功获取分布式锁，开始启动服务
```

**续期和校验：**
```
DEBUG 成功续期分布式锁 key=bingo:ha:lock:abc123 instance_id=hostname-12345
WARN 锁内容校验失败，实例ID不匹配 expected=hostname-12345 actual=hostname-67890
```

### Prometheus 指标

高可用功能相关的指标（未来可扩展）：

- `bingo_ha_lock_acquired_total` - 锁获取次数
- `bingo_ha_lock_refresh_total` - 锁续期次数
- `bingo_ha_lock_validation_total` - 锁校验次数
- `bingo_ha_is_leader` - 当前是否为 Leader（0/1）

## 最佳实践

### 1. Redis 高可用

- 使用 Redis 集群或哨兵模式
- 避免单点故障
- 监控 Redis 连接状态

### 2. 网络配置

- 确保 BinGo 实例与 Redis 之间的网络稳定
- 避免网络分区
- 配置合理的超时时间

### 3. 锁参数调优

- `lock_ttl` 根据网络环境调整
  - 稳定网络：15-20 秒
  - 不稳定网络：20-30 秒
- `refresh_interval` 设置为 `lock_ttl` 的 1/3

### 4. 监控告警

- 监控锁获取失败次数
- 监控主从切换频率
- 监控 Redis 连接状态
- 设置告警规则

### 5. 测试验证

- 测试主节点故障场景
- 测试网络分区场景
- 测试 Redis 故障场景
- 验证数据一致性

## 常见问题

### Q1: 为什么需要锁内容校验？

**A:** 防止脑裂问题。如果只检查锁是否存在，可能出现多个实例都认为自己是主节点的情况。通过校验锁内容中的实例ID，确保只有持有锁的实例才是主节点。

### Q2: 锁续期失败会立即切换吗？

**A:** 不会立即切换。锁续期失败后，续期 goroutine 会退出，但锁可能还未过期。锁监控 goroutine 会检测到不再是 Leader，停止 Binlog 监听。Standby 节点需要等待锁过期后才能获取锁。

### Q3: 多个实例监听不同 MySQL 会冲突吗？

**A:** 不会。通过模板语法 `{{ .ConnCfgHash }}`，不同 MySQL 连接会生成不同的锁键名，互不干扰。

### Q4: 可以手动释放锁吗？

**A:** 可以。通过 Redis 客户端删除锁键即可：
```bash
redis-cli DEL "bingo:ha:lock:abc123"
```

### Q5: 如何查看当前锁状态？

**A:** 通过 Redis 客户端查看：
```bash
# 查看锁内容
redis-cli GET "bingo:ha:lock:abc123"

# 查看锁剩余过期时间
redis-cli TTL "bingo:ha:lock:abc123"
```

## 总结

BinGo 的高可用功能通过 Redis 分布式锁实现主从自动切换，确保服务的高可用性。关键特性包括：

- ✅ 主从自动切换
- ✅ 锁内容校验防止脑裂
- ✅ 自动续期机制
- ✅ 模板支持多 MySQL 连接
- ✅ 优雅降级

通过合理配置和监控，可以确保 BinGo 在生产环境中的稳定运行。

