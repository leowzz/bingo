package executor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bingo/engine"
	"bingo/listener"
	"bingo/utils"

	"github.com/redis/go-redis/v9"
)

// RedisExecutor Redis 执行器
type RedisExecutor struct {
	client *redis.Client
}

// Type 返回执行器类型
func (r *RedisExecutor) Type() string {
	return "redis"
}

// Init 初始化 Redis 客户端
func (r *RedisExecutor) Init(addr, password string, db int) error {
	r.client = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// 测试连接
	ctx := context.Background()
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis 连接失败: %w", err)
	}

	return nil
}

// Execute 执行 Redis 动作
func (r *RedisExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
	if r.client == nil {
		return fmt.Errorf("Redis 客户端未初始化")
	}

	cmd := strings.ToUpper(action.Cmd)
	if cmd == "" {
		cmd = "DEL" // 默认命令
	}

	switch cmd {
	case "DEL":
		return r.executeDel(ctx, action, event)
	case "SET":
		return r.executeSet(ctx, action, event)
	case "EXPIRE":
		return r.executeExpire(ctx, action, event)
	case "INCR", "DECR":
		return r.executeIncrDecr(ctx, action, event)
	default:
		return fmt.Errorf("不支持的 Redis 命令: %s", cmd)
	}
}

// executeDel 执行 DEL 命令
func (r *RedisExecutor) executeDel(ctx context.Context, action engine.Action, event *listener.Event) error {
	// 支持单个 key 或批量 keys
	if len(action.Keys) > 0 {
		// 批量删除
		keys := make([]string, 0, len(action.Keys))
		for _, keyTemplate := range action.Keys {
			key, err := utils.RenderTemplate(keyTemplate, event)
			if err != nil {
				return fmt.Errorf("渲染 key 模板失败: %w", err)
			}
			keys = append(keys, key)
		}
		return r.client.Del(ctx, keys...).Err()
	}

	// 单个 key
	key, err := utils.RenderTemplate(action.Key, event)
	if err != nil {
		return fmt.Errorf("渲染 key 模板失败: %w", err)
	}

	return r.client.Del(ctx, key).Err()
}

// executeSet 执行 SET 命令
func (r *RedisExecutor) executeSet(ctx context.Context, action engine.Action, event *listener.Event) error {
	key, err := utils.RenderTemplate(action.Key, event)
	if err != nil {
		return fmt.Errorf("渲染 key 模板失败: %w", err)
	}

	value, err := utils.RenderTemplate(action.Value, event)
	if err != nil {
		return fmt.Errorf("渲染 value 模板失败: %w", err)
	}

	return r.client.Set(ctx, key, value, 0).Err()
}

// executeExpire 执行 EXPIRE 命令
func (r *RedisExecutor) executeExpire(ctx context.Context, action engine.Action, event *listener.Event) error {
	key, err := utils.RenderTemplate(action.Key, event)
	if err != nil {
		return fmt.Errorf("渲染 key 模板失败: %w", err)
	}

	ttl := action.TTL
	if ttl == 0 {
		ttl = 3600 // 默认 1 小时
	}

	return r.client.Expire(ctx, key, time.Duration(ttl)*time.Second).Err()
}

// executeIncrDecr 执行 INCR/DECR 命令
func (r *RedisExecutor) executeIncrDecr(ctx context.Context, action engine.Action, event *listener.Event) error {
	key, err := utils.RenderTemplate(action.Key, event)
	if err != nil {
		return fmt.Errorf("渲染 key 模板失败: %w", err)
	}

	cmd := strings.ToUpper(action.Cmd)
	if cmd == "INCR" {
		return r.client.Incr(ctx, key).Err()
	}
	return r.client.Decr(ctx, key).Err()
}
