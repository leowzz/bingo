### 项目标题 BinGo

**基于 Binlog 的数据库变更订阅与事件触发引擎 (DB-Event-Trigger)**

### 1. 项目背景与痛点 (Why this?)

在传统的业务开发中，我们经常遇到以下痛点：

1. **缓存一致性难题**：更新了数据库，必须在代码里手动删除 Redis 缓存。如果数据库更新成功但缓存删除失败，或者代码逻辑遗漏，就会导致脏读。
2. **业务耦合严重**：比如“用户注册成功”后，需要发邮件、送积分、建档案。这些逻辑全部写在注册接口里，导致接口臃肿，响应变慢。
3. **轮询效率低**：为了监控订单状态变化，后端服务不得不频繁轮询数据库，造成资源浪费。

**本项目 (Pulse)**  旨在解决上述问题：它伪装成 MySQL Slave，**旁路监听**数据变更，根据预定义的**规则 (Rules)**  自动触发**动作 (Actions)** 。

 **—— 核心价值：将数据库的“数据变更”转化为“业务事件”，实现彻底的业务解耦。**

### 2. 技术栈 (Tech Stack)

- **语言**：Go (Golang) —— 高并发、低延迟，适合处理事件流。
- **核心库**：`go-mysql` (用于监听 Binlog)。
- **规则引擎**：`cel-go` (Google Common Expression Language) 或简单的 YAML 规则匹配。
- **组件**：

  - **Source**: MySQL
  - **State**: Redis (用于存断点位点，以及作为执行操作的目标)
  - **Actions**: Redis (DEL/SET), HTTP Webhook, gRPC, Kafka/RabbitMQ。

### 3. 核心架构设计

系统不再是简单的 A 到 B 的管道，而是一个  **"监听 -> 匹配 -> 执行"**  的闭环。

```text
[ MySQL ]
   | (Binlog Stream)
   v
[ 1. Listener (监听器) ] <--- 伪装 Slave，清洗数据
   | (标准化 Event: {Table, Action, OldRow, NewRow})
   v
[ 2. Rule Engine (规则引擎) ] <--- 核心差异点！
   | (加载 rules.yaml: "If Table=users AND Age > 18...")
   | Matches?
   v
[ 3. Action Executor (执行器) ]
   |---> [ Redis Adapter ] -> 执行 DEL user:1001
   |---> [ Webhook Adapter ] -> POST /api/callback
   |---> [ Logger Adapter ] -> 审计日志
```

### 4. 核心功能与难点 (Key Features & Challenges)

#### A. 动态规则引擎 (The Brain)

**功能**：不是所有变更都需要处理，只有命中规则的才处理。
**难点**：如何灵活定义规则？

- **配置示例**：

  ```yaml
  rules:
    - id: "user_cache_invalidate"
      table: "users"
      events: ["UPDATE", "DELETE"]
      # 难点：支持条件过滤
      filter: "NewRow['status'] != OldRow['status']" 
      actions:
        - type: "redis"
          cmd: "DEL"
          # 难点：支持变量提取
          key: "cache:user:{{ .ID }}" 
  ```

#### B. 变量提取与模板渲染 (Context Extraction)

**功能**：执行动作时，需要用到数据里的字段。
**实现**：

- 当监听到 `UPDATE users SET name='Alice' WHERE id=101`。
- 系统需要从变更行中提取 `id=101`。
- 将配置中的 `cache:user:{{ .ID }}` 渲染为 `cache:user:101`，然后执行 Redis 命令。

#### C. 防抖与聚合 (Debounce & Batching)

**场景**：如果执行了 `UPDATE users SET status=1`（批量更新了 10,000 个用户）。
**问题**：瞬间触发 10,000 次 Redis DEL 操作或 10,000 次 Webhook，可能会打挂下游服务。
**解决方案**：

- **窗口聚合**：在 100ms 窗口内，如果是同一个 Rule 的触发，聚合为一次批量操作（Pipeline DEL）。

#### D. 顺序性保障 (Ordering)

**场景**：先 Insert 后 Update。如果处理顺序反了，可能导致缓存里存的是旧数据。
**方案**：基于 **主键 Hash** 将事件分发到不同的 Goroutine，确保同一个 ID 的变更串行处理。