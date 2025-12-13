package executor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"bingo/engine"
	"bingo/internal/logger"
	"bingo/listener"
	"bingo/utils"

	"github.com/redis/go-redis/v9"
)

// RedisExecutor Redis 执行器
type RedisExecutor struct {
	clients    map[string]*redis.Client // 多个 Redis 连接，key 为连接名称
	connHashes map[string]string        // 每个连接名对应的配置 hash，用于避免意外重连
	mu         sync.RWMutex
}

// Type 返回执行器类型
func (r *RedisExecutor) Type() string {
	return "redis"
}

// NewRedisExecutor 创建新的 Redis 执行器
func NewRedisExecutor() *RedisExecutor {
	return &RedisExecutor{
		clients:    make(map[string]*redis.Client),
		connHashes: make(map[string]string),
	}
}

// calculateConfigHash 计算 Redis 连接配置的 hash 值
//
// 使用与 RedisConnectionConfig.ConnCfgHash 相同的算法。
func (r *RedisExecutor) calculateConfigHash(addr, username, password string, db int) string {
	cfg := engine.RedisConnectionConfig{
		Addr:     addr,
		Username: username,
		Password: password,
		DB:       db,
	}
	return cfg.ConnCfgHash()
}

// AddConnection 添加 Redis 连接
func (r *RedisExecutor) AddConnection(name, addr, username, password string, db int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 计算配置 hash
	cfgHash := r.calculateConfigHash(addr, username, password, db)

	// 如果连接已存在且配置相同，不需要重连
	if oldHash, exists := r.connHashes[name]; exists && oldHash == cfgHash {
		logger.Debugw("Redis 连接配置未变化，跳过重连", "name", name, "hash", cfgHash)
		return nil
	}

	// 如果连接已存在，先关闭旧连接
	if oldClient, exists := r.clients[name]; exists {
		err := oldClient.Close()
		if err != nil {
			logger.Errorf("关闭 Redis 链接失败 [%s]: %w", name, err)
		}
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Username: username, // Redis 6.0+ ACL 支持
		Password: password,
		DB:       db,
	})

	// 测试连接
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis 连接失败 [%s]: %w", name, err)
	}

	r.clients[name] = client
	r.connHashes[name] = cfgHash
	if username != "" {
		logger.Infow("添加 Redis 连接成功", "name", name, "addr", addr, "username", username, "db", db, "hash", cfgHash)
	} else {
		logger.Infow("添加 Redis 连接成功", "name", name, "addr", addr, "db", db, "hash", cfgHash)
	}
	return nil
}

// AddConnectionWithClient 使用已存在的 Redis 客户端添加连接（用于系统 Redis）
func (r *RedisExecutor) AddConnectionWithClient(name string, client *redis.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 如果连接已存在，先关闭旧连接（但不关闭传入的客户端）
	if oldClient, exists := r.clients[name]; exists && oldClient != client {
		err := oldClient.Close()
		if err != nil {
			logger.Errorf("关闭 Redis 链接失败 [%s]: %w", name, err)
		}
	}

	r.clients[name] = client
	// 系统连接不存储 hash，因为配置在 config.yaml 中管理
	logger.Debugw("添加 Redis 连接成功（使用已有客户端）", "name", name)
}

// Init 初始化 Redis 客户端（兼容旧接口，使用默认连接名 "default"）
func (r *RedisExecutor) Init(addr, password string, db int) error {
	return r.AddConnection("default", addr, "", password, db)
}

// getClient 获取 Redis 客户端
func (r *RedisExecutor) getClient(connName string) (*redis.Client, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 如果没有指定连接名，尝试使用默认连接，如果没有则使用第一个可用连接
	if connName == "" {
		// 优先使用 default
		if client, exists := r.clients["default"]; exists {
			return client, nil
		}
		// 如果没有 default，使用 system
		if client, exists := r.clients["system"]; exists {
			return client, nil
		}
		// 如果都没有，使用第一个可用连接
		for _, client := range r.clients {
			return client, nil
		}
		return nil, fmt.Errorf("没有可用的 Redis 连接")
	}

	client, exists := r.clients[connName]
	if !exists {
		// 列出所有可用的连接名称，方便调试
		available := make([]string, 0, len(r.clients))
		for name := range r.clients {
			available = append(available, name)
		}
		return nil, fmt.Errorf("Redis 连接不存在: %s，可用连接: %v", connName, available)
	}

	return client, nil
}

// Execute 执行 Redis 动作
func (r *RedisExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
	// 获取 Redis 客户端
	client, err := r.getClient(action.RedisConn)
	if err != nil {
		return err
	}

	cmd := strings.ToUpper(action.Cmd)
	if cmd == "" {
		cmd = "DEL" // 默认命令
	}

	logger.Debugw("执行 Redis 命令",
		"cmd", cmd,
		"conn", action.RedisConn,
		"table", event.Table,
		"action", event.Action,
	)

	switch cmd {
	case "DEL":
		err = r.executeDel(ctx, client, action, event)
	case "SET", "SETEX":
		err = r.executeSet(ctx, client, action, event)
	case "EXPIRE":
		err = r.executeExpire(ctx, client, action, event)
	case "INCR", "DECR":
		err = r.executeIncrDecr(ctx, client, action, event)
	default:
		err = fmt.Errorf("不支持的 Redis 命令: %s", cmd)
	}

	if err != nil {
		logger.Errorw("Redis 命令执行失败",
			"cmd", cmd,
			"conn", action.RedisConn,
			"table", event.Table,
			"action", event.Action,
			"error", err,
		)
		return fmt.Errorf("Redis %s 命令执行失败: %w", cmd, err)
	}

	logger.Debugw("Redis 命令执行成功",
		"cmd", cmd,
		"conn", action.RedisConn,
		"table", event.Table,
		"action", event.Action,
	)

	return nil
}

// executeDel 执行 DEL 命令
func (r *RedisExecutor) executeDel(ctx context.Context, client *redis.Client, action engine.Action, event *listener.Event) error {
	// 支持单个 key 或批量 keys
	if len(action.Keys) > 0 {
		// 批量删除
		keys := make([]string, 0, len(action.Keys))
		for _, keyTemplate := range action.Keys {
			key, err := utils.RenderTemplate(keyTemplate, event.ToMap())
			if err != nil {
				return fmt.Errorf("渲染 key 模板失败 (批量): %w", err)
			}
			keys = append(keys, key)
		}
		result := client.Del(ctx, keys...)
		if err := result.Err(); err != nil {
			return fmt.Errorf("批量删除 keys 失败: %w, keys: %v", err, keys)
		}
		logger.Debugw("批量删除 Redis keys 成功", "keys", keys, "deleted_count", result.Val())
		return nil
	}

	// 单个 key
	if action.Key == "" {
		return fmt.Errorf("DEL 命令需要指定 key 或 keys")
	}

	key, err := utils.RenderTemplate(action.Key, event.ToMap())
	if err != nil {
		return fmt.Errorf("渲染 key 模板失败: %w", err)
	}

	result := client.Del(ctx, key)
	if err := result.Err(); err != nil {
		return fmt.Errorf("删除 key 失败: %w, key: %s", err, key)
	}
	logger.Debugw("删除 Redis key 成功", "key", key, "deleted", result.Val() > 0)
	return nil
}

// executeSet 执行 SET 或 SETEX 命令
func (r *RedisExecutor) executeSet(ctx context.Context, client *redis.Client, action engine.Action, event *listener.Event) error {
	if action.Key == "" {
		return fmt.Errorf("SET 命令需要指定 key")
	}

	key, err := utils.RenderTemplate(action.Key, event.ToMap())
	if err != nil {
		return fmt.Errorf("渲染 key 模板失败: %w", err)
	}

	value, err := utils.RenderTemplate(action.Value, event.ToMap())
	if err != nil {
		return fmt.Errorf("渲染 value 模板失败: %w", err)
	}

	// 支持 TTL：如果指定了 TTL，使用 SetEX；否则使用 Set
	ttl := time.Duration(0)
	if action.TTL > 0 {
		ttl = time.Duration(action.TTL) * time.Second
	}

	var setErr error
	if ttl > 0 {
		// 使用 SetEx（原子操作，设置值的同时设置过期时间）
		setErr = client.SetEx(ctx, key, value, ttl).Err()
		logger.Debugw("执行 Redis SETEX 命令", "key", key, "ttl", ttl)
	} else {
		// 使用 Set（永不过期）
		setErr = client.Set(ctx, key, value, 0).Err()
		logger.Debugw("执行 Redis SET 命令", "key", key)
	}

	if setErr != nil {
		return fmt.Errorf("设置 key 失败: %w, key: %s, has_ttl: %v", setErr, key, ttl > 0)
	}

	logger.Debugw("设置 Redis key 成功", "key", key, "has_ttl", ttl > 0, "ttl_seconds", ttl.Seconds())
	return nil
}

// executeExpire 执行 EXPIRE 命令
func (r *RedisExecutor) executeExpire(ctx context.Context, client *redis.Client, action engine.Action, event *listener.Event) error {
	if action.Key == "" {
		return fmt.Errorf("EXPIRE 命令需要指定 key")
	}

	key, err := utils.RenderTemplate(action.Key, event.ToMap())
	if err != nil {
		return fmt.Errorf("渲染 key 模板失败: %w", err)
	}

	ttl := action.TTL
	if ttl == 0 {
		ttl = 3600 // 默认 1 小时
		logger.Debugw("EXPIRE 命令使用默认 TTL", "key", key, "ttl", ttl)
	}

	result := client.Expire(ctx, key, time.Duration(ttl)*time.Second)
	if err := result.Err(); err != nil {
		return fmt.Errorf("设置过期时间失败: %w, key: %s, ttl: %d", err, key, ttl)
	}

	if !result.Val() {
		logger.Warnw("EXPIRE 命令执行但 key 不存在", "key", key)
		return fmt.Errorf("key 不存在或已过期: %s", key)
	}

	logger.Debugw("设置 Redis key 过期时间成功", "key", key, "ttl_seconds", ttl)
	return nil
}

// executeIncrDecr 执行 INCR/DECR 命令
func (r *RedisExecutor) executeIncrDecr(ctx context.Context, client *redis.Client, action engine.Action, event *listener.Event) error {
	if action.Key == "" {
		return fmt.Errorf("INCR/DECR 命令需要指定 key")
	}

	key, err := utils.RenderTemplate(action.Key, event.ToMap())
	if err != nil {
		return fmt.Errorf("渲染 key 模板失败: %w", err)
	}

	cmd := strings.ToUpper(action.Cmd)
	var result *redis.IntCmd
	if cmd == "INCR" {
		result = client.Incr(ctx, key)
	} else {
		result = client.Decr(ctx, key)
	}

	if err := result.Err(); err != nil {
		return fmt.Errorf("%s 命令执行失败: %w, key: %s", cmd, err, key)
	}

	logger.Debugw("Redis INCR/DECR 命令执行成功",
		"cmd", cmd,
		"key", key,
		"new_value", result.Val(),
	)
	return nil
}

// HasConnections 检查是否有可用的连接
func (r *RedisExecutor) HasConnections() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients) > 0
}

// RemoveConnection 删除指定的 Redis 连接
//
// 不会删除 "system" 和 "default" 连接，这些是系统保留的连接。
func (r *RedisExecutor) RemoveConnection(name string) error {
	// 保护系统连接
	if name == "system" || name == "default" {
		return fmt.Errorf("不能删除系统保留的连接: %s", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	client, exists := r.clients[name]
	if !exists {
		return fmt.Errorf("连接不存在: %s", name)
	}

	if err := client.Close(); err != nil {
		logger.Warnw("关闭 Redis 连接失败", "name", name, "error", err)
	} else {
		logger.Debugw("关闭 Redis 连接成功", "name", name)
	}

	delete(r.clients, name)
	delete(r.connHashes, name) // 同时删除对应的 hash
	logger.Infow("删除 Redis 连接", "name", name)
	return nil
}

// GetConnectionNames 获取所有连接名称
func (r *RedisExecutor) GetConnectionNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	return names
}

// UpdateConnections 批量更新 Redis 连接配置
//
// 根据新的连接配置列表，添加新的连接、更新已存在的连接、删除不存在的连接。
// 不会删除 "system" 和 "default" 连接。
func (r *RedisExecutor) UpdateConnections(connections []engine.RedisConnectionConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 构建新连接名称集合
	newConnNames := make(map[string]bool)
	for _, conn := range connections {
		// 跳过系统保留的连接名
		if conn.Name == "system" || conn.Name == "default" {
			logger.Warnf("跳过系统保留的连接名: %s", conn.Name)
			continue
		}
		newConnNames[conn.Name] = true
	}

	// 找出需要删除的连接（不在新配置中的，且不是系统保留的）
	toRemove := make([]string, 0)
	for name := range r.clients {
		if name != "system" && name != "default" && !newConnNames[name] {
			toRemove = append(toRemove, name)
		}
	}

	// 删除不需要的连接
	for _, name := range toRemove {
		if client, exists := r.clients[name]; exists {
			if err := client.Close(); err != nil {
				logger.Warnw("关闭 Redis 连接失败", "name", name, "error", err)
			}
			delete(r.clients, name)
			delete(r.connHashes, name) // 同时删除对应的 hash
			logger.Infow("删除 Redis 连接", "name", name)
		}
	}

	// 添加或更新连接
	for _, conn := range connections {
		// 跳过系统保留的连接名
		if conn.Name == "system" || conn.Name == "default" {
			continue
		}

		// 计算新配置的 hash
		newHash := conn.ConnCfgHash()

		// 检查连接是否已存在且配置未变化
		if oldHash, exists := r.connHashes[conn.Name]; exists && oldHash == newHash {
			logger.Debugw("Redis 连接配置未变化，跳过重连", "name", conn.Name, "hash", newHash)
			continue
		}

		// 如果连接已存在，先创建新连接，再替换旧连接
		// 这样可以避免关闭旧连接时影响正在执行的命令
		var oldClient *redis.Client
		if existingClient, exists := r.clients[conn.Name]; exists {
			oldClient = existingClient
		}

		// 创建新连接
		client := redis.NewClient(&redis.Options{
			Addr:     conn.Addr,
			Username: conn.Username, // Redis 6.0+ ACL 支持
			Password: conn.Password,
			DB:       conn.DB,
		})

		// 测试连接
		ctx := context.Background()
		if err := client.Ping(ctx).Err(); err != nil {
			// 如果新连接失败，保持旧连接
			err := client.Close()
			if err != nil {
				logger.Warnw("Redis 连接关闭失败")
			}
			logger.Warnw("Redis 连接失败，保持旧连接", "name", conn.Name, "addr", conn.Addr, "error", err)
			continue
		}

		// 先更新 map 中的连接（原子操作）
		r.clients[conn.Name] = client
		r.connHashes[conn.Name] = newHash // 更新 hash
		if conn.Username != "" {
			logger.Infow("更新 Redis 连接", "name", conn.Name, "addr", conn.Addr, "username", conn.Username, "db", conn.DB, "hash", newHash)
		} else {
			logger.Infow("更新 Redis 连接", "name", conn.Name, "addr", conn.Addr, "db", conn.DB, "hash", newHash)
		}

		// 在锁外异步关闭旧连接，给正在执行的命令一些时间完成
		// 注意：这里不能使用 defer，因为需要在锁外执行
		if oldClient != nil {
			go func(clientToClose *redis.Client, connName string) {
				// 等待一小段时间，让正在执行的命令完成
				time.Sleep(100 * time.Millisecond)
				if err := clientToClose.Close(); err != nil {
					logger.Warnw("关闭旧 Redis 连接失败", "name", connName, "error", err)
				} else {
					logger.Debugw("关闭旧 Redis 连接成功", "name", connName)
				}
			}(oldClient, conn.Name)
		}
	}

	return nil
}

// Close 关闭所有 Redis 连接
func (r *RedisExecutor) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, client := range r.clients {
		if err := client.Close(); err != nil {
			logger.Warnw("关闭 Redis 连接失败", "name", name, "error", err)
		} else {
			logger.Debugw("关闭 Redis 连接成功", "name", name)
		}
	}
	r.clients = make(map[string]*redis.Client)
	r.connHashes = make(map[string]string)
}
