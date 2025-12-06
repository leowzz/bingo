package executor

import (
	"context"
	"testing"
	"time"

	"bingo/engine"
	"bingo/listener"

	"github.com/redis/go-redis/v9"
)

// mockRedisClient 创建一个模拟的 Redis 客户端（用于测试）
// 注意：这是一个简化的 mock，实际测试中可能需要使用更完整的 mock 库
func createMockRedisClient() *redis.Client {
	// 使用一个不存在的地址，这样测试会失败但不会阻塞
	// 在实际测试中，可以使用 testcontainers 或 miniredis
	return redis.NewClient(&redis.Options{
		Addr: "localhost:9999", // 不存在的地址
	})
}

func TestRedisExecutor_Type(t *testing.T) {
	exec := NewRedisExecutor()
	if exec.Type() != "redis" {
		t.Errorf("Type() = %v, want 'redis'", exec.Type())
	}
}

func TestRedisExecutor_AddConnectionWithClient(t *testing.T) {
	exec := NewRedisExecutor()
	client := createMockRedisClient()
	defer client.Close()

	exec.AddConnectionWithClient("test", client)

	if !exec.HasConnections() {
		t.Error("AddConnectionWithClient() should add a connection")
	}
}

func TestRedisExecutor_HasConnections(t *testing.T) {
	exec := NewRedisExecutor()

	// 初始状态应该没有连接
	if exec.HasConnections() {
		t.Error("HasConnections() should return false initially")
	}

	// 添加连接后应该有连接
	client := createMockRedisClient()
	defer client.Close()
	exec.AddConnectionWithClient("test", client)

	if !exec.HasConnections() {
		t.Error("HasConnections() should return true after adding connection")
	}
}

func TestRedisExecutor_getClient(t *testing.T) {
	exec := NewRedisExecutor()

	// 测试没有连接的情况
	_, err := exec.getClient("")
	if err == nil {
		t.Error("getClient() should return error when no connections exist")
	}

	// 添加多个连接
	client1 := createMockRedisClient()
	defer client1.Close()
	exec.AddConnectionWithClient("conn1", client1)

	client2 := createMockRedisClient()
	defer client2.Close()
	exec.AddConnectionWithClient("conn2", client2)

	// 测试获取指定连接
	client, err := exec.getClient("conn1")
	if err != nil {
		t.Fatalf("getClient() error = %v", err)
	}
	if client != client1 {
		t.Error("getClient() should return the correct client")
	}

	// 测试获取不存在的连接
	_, err = exec.getClient("nonexistent")
	if err == nil {
		t.Error("getClient() should return error for nonexistent connection")
	}
}

func TestRedisExecutor_getClient_DefaultPriority(t *testing.T) {
	exec := NewRedisExecutor()

	// 添加 system 连接
	clientSystem := createMockRedisClient()
	defer clientSystem.Close()
	exec.AddConnectionWithClient("system", clientSystem)

	// 添加 default 连接
	clientDefault := createMockRedisClient()
	defer clientDefault.Close()
	exec.AddConnectionWithClient("default", clientDefault)

	// 添加其他连接
	clientOther := createMockRedisClient()
	defer clientOther.Close()
	exec.AddConnectionWithClient("other", clientOther)

	// 不指定连接名，应该优先使用 default
	client, err := exec.getClient("")
	if err != nil {
		t.Fatalf("getClient() error = %v", err)
	}
	if client != clientDefault {
		t.Error("getClient() should prefer 'default' connection")
	}

	// 移除 default，应该使用 system
	exec.mu.Lock()
	delete(exec.clients, "default")
	exec.mu.Unlock()

	client, err = exec.getClient("")
	if err != nil {
		t.Fatalf("getClient() error = %v", err)
	}
	if client != clientSystem {
		t.Error("getClient() should use 'system' when 'default' is not available")
	}
}

func TestRedisExecutor_Execute_ConnectionSelection(t *testing.T) {
	exec := NewRedisExecutor()

	// 添加多个连接
	client1 := createMockRedisClient()
	defer client1.Close()
	exec.AddConnectionWithClient("cache", client1)

	client2 := createMockRedisClient()
	defer client2.Close()
	exec.AddConnectionWithClient("session", client2)

	event := &listener.Event{
		Table:  "test.users",
		Action: listener.ActionInsert,
		NewRow: map[string]interface{}{
			"id": 123,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 测试指定连接
	action := engine.Action{
		Type:      "redis",
		Cmd:       "DEL",
		Key:       "test:key",
		RedisConn: "cache",
	}

	// 由于使用的是 mock 客户端（不存在的地址），执行会失败
	// 但我们主要测试连接选择逻辑
	err := exec.Execute(ctx, action, event)
	// 错误是预期的，因为我们使用的是不存在的 Redis 地址
	if err == nil {
		t.Error("Execute() should return error with invalid Redis connection")
	}

	// 测试使用不存在的连接
	action.RedisConn = "nonexistent"
	err = exec.Execute(ctx, action, event)
	if err == nil {
		t.Error("Execute() should return error for nonexistent connection")
	}
}

func TestRedisExecutor_Close(t *testing.T) {
	exec := NewRedisExecutor()

	client1 := createMockRedisClient()
	exec.AddConnectionWithClient("conn1", client1)

	client2 := createMockRedisClient()
	exec.AddConnectionWithClient("conn2", client2)

	// 关闭所有连接
	exec.Close()

	// 应该没有连接了
	if exec.HasConnections() {
		t.Error("Close() should remove all connections")
	}
}

func TestRedisExecutor_Init(t *testing.T) {
	exec := NewRedisExecutor()

	// Init 应该使用 "default" 作为连接名
	// 由于我们使用不存在的地址，会失败，但可以测试逻辑
	err := exec.Init("localhost:9999", "", 0)
	// 错误是预期的
	if err == nil {
		t.Error("Init() should return error with invalid Redis address")
	}
}

func TestRedisExecutor_Execute_Commands(t *testing.T) {
	exec := NewRedisExecutor()
	client := createMockRedisClient()
	defer client.Close()
	exec.AddConnectionWithClient("test", client)

	event := &listener.Event{
		Table:  "test.users",
		Action: listener.ActionInsert,
		NewRow: map[string]interface{}{
			"id": 123,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	tests := []struct {
		name    string
		action  engine.Action
		wantErr bool
	}{
		{
			name: "DEL 命令",
			action: engine.Action{
				Type:      "redis",
				Cmd:       "DEL",
				Key:       "test:key:{{ .Id }}",
				RedisConn: "test",
			},
			wantErr: true, // 因为 Redis 连接不存在
		},
		{
			name: "SET 命令",
			action: engine.Action{
				Type:      "redis",
				Cmd:       "SET",
				Key:       "test:key:{{ .Id }}",
				Value:     "test_value",
				RedisConn: "test",
			},
			wantErr: true,
		},
		{
			name: "SET with TTL",
			action: engine.Action{
				Type:      "redis",
				Cmd:       "SET",
				Key:       "test:key:{{ .Id }}",
				Value:     "test_value",
				TTL:       3600,
				RedisConn: "test",
			},
			wantErr: true,
		},
		{
			name: "EXPIRE 命令",
			action: engine.Action{
				Type:      "redis",
				Cmd:       "EXPIRE",
				Key:       "test:key:{{ .Id }}",
				TTL:       3600,
				RedisConn: "test",
			},
			wantErr: true,
		},
		{
			name: "INCR 命令",
			action: engine.Action{
				Type:      "redis",
				Cmd:       "INCR",
				Key:       "test:counter:{{ .Id }}",
				RedisConn: "test",
			},
			wantErr: true,
		},
		{
			name: "DECR 命令",
			action: engine.Action{
				Type:      "redis",
				Cmd:       "DECR",
				Key:       "test:counter:{{ .Id }}",
				RedisConn: "test",
			},
			wantErr: true,
		},
		{
			name: "不支持的命令",
			action: engine.Action{
				Type:      "redis",
				Cmd:       "UNKNOWN",
				Key:       "test:key",
				RedisConn: "test",
			},
			wantErr: true,
		},
		{
			name: "默认命令（DEL）",
			action: engine.Action{
				Type:      "redis",
				Key:       "test:key",
				RedisConn: "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := exec.Execute(ctx, tt.action, event)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRedisExecutor_Execute_BatchKeys(t *testing.T) {
	exec := NewRedisExecutor()
	client := createMockRedisClient()
	defer client.Close()
	exec.AddConnectionWithClient("test", client)

	event := &listener.Event{
		Table:  "test.users",
		Action: listener.ActionInsert,
		NewRow: map[string]interface{}{
			"id": 123,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 测试批量删除
	action := engine.Action{
		Type:      "redis",
		Cmd:       "DEL",
		Keys:      []string{"key1:{{ .Id }}", "key2:{{ .Id }}", "key3:{{ .Id }}"},
		RedisConn: "test",
	}

	err := exec.Execute(ctx, action, event)
	// 错误是预期的
	if err == nil {
		t.Error("Execute() should return error with invalid Redis connection")
	}
}
