package executor

import (
	"context"
	"fmt"
	"log"

	"bingo/engine"
	"bingo/listener"
	"bingo/utils"
)

// LogExecutor 日志执行器
type LogExecutor struct{}

// Type 返回执行器类型
func (l *LogExecutor) Type() string {
	return "log"
}

// Execute 执行日志动作
func (l *LogExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
	// 渲染日志消息
	message := action.Message
	if message == "" {
		message = fmt.Sprintf("Event: %s on table %s", event.Action, event.Table)
	}

	renderedMsg, err := utils.RenderTemplate(message, event)
	if err != nil {
		return fmt.Errorf("渲染日志消息失败: %w", err)
	}

	// 根据日志级别输出
	level := action.Level
	if level == "" {
		level = "info"
	}

	switch level {
	case "debug":
		log.Printf("[DEBUG] %s", renderedMsg)
	case "info":
		log.Printf("[INFO] %s", renderedMsg)
	case "warn":
		log.Printf("[WARN] %s", renderedMsg)
	case "error":
		log.Printf("[ERROR] %s", renderedMsg)
	default:
		log.Printf("[INFO] %s", renderedMsg)
	}

	return nil
}
