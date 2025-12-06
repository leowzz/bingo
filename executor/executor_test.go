package executor

import (
	"context"
	"testing"
	"time"

	"bingo/engine"
	"bingo/listener"
)

func TestExecutor_Register(t *testing.T) {
	exec := NewExecutor()

	// 测试注册新的执行器
	redisExec := &RedisExecutor{}
	exec.Register(redisExec)

	// 尝试执行一个 Redis 动作（会失败因为没有初始化，但可以测试注册）
	action := engine.Action{
		Type: "redis",
		Cmd:  "DEL",
		Key:  "test:key",
	}
	event := &listener.Event{
		Table:  "test.users",
		Action: listener.ActionInsert,
	}

	ctx := context.Background()
	err := exec.Execute(ctx, action, event)
	// 应该返回错误，因为 Redis 客户端未初始化
	if err == nil {
		t.Error("Execute() should return error when executor not initialized")
	}
}

func TestExecutor_ExecuteActions(t *testing.T) {
	exec := NewExecutor()

	// 测试执行多个动作
	actions := []engine.Action{
		{
			Type:    "log",
			Message: "Test message 1",
			Level:   "info",
		},
		{
			Type:    "log",
			Message: "Test message 2",
			Level:   "debug",
		},
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

	err := exec.ExecuteActions(ctx, actions, event)
	if err != nil {
		t.Errorf("ExecuteActions() error = %v", err)
	}
}

func TestExecutor_ExecuteUnknownAction(t *testing.T) {
	exec := NewExecutor()

	action := engine.Action{
		Type: "unknown_type",
	}

	event := &listener.Event{
		Table:  "test.users",
		Action: listener.ActionInsert,
	}

	ctx := context.Background()
	err := exec.Execute(ctx, action, event)
	if err == nil {
		t.Error("Execute() should return error for unknown action type")
	}
}
