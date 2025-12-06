package listener

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/redis/go-redis/v9"
)

// PositionStore 位置存储接口
type PositionStore interface {
	// Load 加载保存的位置
	Load(ctx context.Context) (*mysql.Position, error)
	// Save 保存位置
	Save(ctx context.Context, pos mysql.Position) error
}

// RedisPositionStore Redis 位置存储实现
type RedisPositionStore struct {
	client *redis.Client
	key    string
}

// NewRedisPositionStore 创建 Redis 位置存储
func NewRedisPositionStore(client *redis.Client, key string) *RedisPositionStore {
	if key == "" {
		key = "bingo:binlog:position"
	}
	return &RedisPositionStore{
		client: client,
		key:    key,
	}
}

// Load 从 Redis 加载位置
func (r *RedisPositionStore) Load(ctx context.Context) (*mysql.Position, error) {
	data, err := r.client.Get(ctx, r.key).Result()
	if err == redis.Nil {
		// 键不存在，返回 nil 表示没有保存的位置
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("从 Redis 读取位置失败: %w", err)
	}

	var pos mysql.Position
	if err := json.Unmarshal([]byte(data), &pos); err != nil {
		return nil, fmt.Errorf("解析位置数据失败: %w", err)
	}

	return &pos, nil
}

// Save 保存位置到 Redis
func (r *RedisPositionStore) Save(ctx context.Context, pos mysql.Position) error {
	data, err := json.Marshal(pos)
	if err != nil {
		return fmt.Errorf("序列化位置数据失败: %w", err)
	}

	if err := r.client.Set(ctx, r.key, data, 0).Err(); err != nil {
		return fmt.Errorf("保存位置到 Redis 失败: %w", err)
	}

	return nil
}

// MemoryPositionStore 内存位置存储（用于测试或不需要持久化的场景）
type MemoryPositionStore struct {
	pos *mysql.Position
}

// NewMemoryPositionStore 创建内存位置存储
func NewMemoryPositionStore() *MemoryPositionStore {
	return &MemoryPositionStore{}
}

// Load 从内存加载位置
func (m *MemoryPositionStore) Load(ctx context.Context) (*mysql.Position, error) {
	return m.pos, nil
}

// Save 保存位置到内存
func (m *MemoryPositionStore) Save(ctx context.Context, pos mysql.Position) error {
	m.pos = &pos
	return nil
}
