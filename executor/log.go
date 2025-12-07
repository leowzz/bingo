package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"bingo/engine"
	"bingo/internal/logger"
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

	// 准备日志字段
	logFields := []interface{}{
		"table", event.Table,
		"action", event.Action,
		"schema", event.Schema,
		"table_name", event.TableName,
	}

	// 如果 format 为 json，输出 JSON 格式
	format := action.Format
	if format == "" {
		format = "text" // 默认文本格式
	}

	if format == "json" {
		// JSON 格式输出
		logData := map[string]interface{}{
			"message":    renderedMsg,
			"table":      event.Table,
			"action":     string(event.Action),
			"schema":     event.Schema,
			"table_name": event.TableName,
			"timestamp":  event.Timestamp,
		}
		if event.NewRow != nil {
			logData["new_row"] = event.NewRow
		}
		if event.OldRow != nil {
			logData["old_row"] = event.OldRow
		}

		jsonBytes, err := json.Marshal(logData)
		if err != nil {
			// 如果 JSON 序列化失败，回退到文本格式
			logger.Infow(renderedMsg, logFields...)
		} else {
			// 使用 logger 输出 JSON
			logFields = append(logFields, "json_data", string(jsonBytes))
			logger.Infow(renderedMsg, logFields...)
		}
	} else {
		// 文本格式输出
		switch level {
		case "debug":
			logger.Debugw(renderedMsg, logFields...)
		case "info":
			logger.Infow(renderedMsg, logFields...)
		case "warn":
			logger.Warnw(renderedMsg, logFields...)
		case "error":
			logger.Errorw(renderedMsg, logFields...)
		default:
			logger.Infow(renderedMsg, logFields...)
		}
	}

	return nil
}
