package executor

import (
	"context"
	"fmt"

	"bingo/engine"
	"bingo/listener"
)

// ActionExecutor 动作执行器接口
type ActionExecutor interface {
	Execute(ctx context.Context, action engine.Action, event *listener.Event) error
	Type() string
}

// Executor 执行器管理器
type Executor struct {
	executors map[string]ActionExecutor
}

// NewExecutor 创建新的执行器管理器
func NewExecutor() *Executor {
	exec := &Executor{
		executors: make(map[string]ActionExecutor),
	}

	// 注册默认执行器
	exec.Register(NewWebhookExecutor())
	exec.Register(&LogExecutor{})
	exec.Register(&KafkaExecutor{})
	exec.Register(&RabbitMQExecutor{})

	return exec
}

// Register 注册执行器
func (e *Executor) Register(executor ActionExecutor) {
	e.executors[executor.Type()] = executor
}

// Execute 执行动作
func (e *Executor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
	executor, ok := e.executors[action.Type]
	if !ok {
		return fmt.Errorf("未知的动作类型: %s", action.Type)
	}

	return executor.Execute(ctx, action, event)
}

// ExecuteActions 批量执行动作
func (e *Executor) ExecuteActions(ctx context.Context, actions []engine.Action, event *listener.Event) error {
	for _, action := range actions {
		if err := e.Execute(ctx, action, event); err != nil {
			return fmt.Errorf("执行动作失败: %w", err)
		}
	}
	return nil
}
