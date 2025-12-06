package listener

import (
	"fmt"
	"time"
)

// Action 事件动作类型
type Action string

const (
	ActionInsert Action = "INSERT"
	ActionUpdate Action = "UPDATE"
	ActionDelete Action = "DELETE"
)

// Event 标准化的事件结构
type Event struct {
	// 基本信息
	Table     string    `json:"table"`     // 表名（格式: database.table）
	Action    Action    `json:"action"`    // 操作类型
	Timestamp time.Time `json:"timestamp"` // 事件时间戳

	// 数据
	OldRow map[string]interface{} `json:"old_row,omitempty"` // 更新前的数据（UPDATE/DELETE）
	NewRow map[string]interface{} `json:"new_row,omitempty"` // 更新后的数据（INSERT/UPDATE）

	// 元数据
	Schema    string `json:"schema"`     // 数据库名
	TableName string `json:"table_name"` // 表名（不含数据库）
}

// GetField 获取字段值（优先从 NewRow，如果不存在则从 OldRow）
func (e *Event) GetField(name string) interface{} {
	if e.NewRow != nil {
		if val, ok := e.NewRow[name]; ok {
			return val
		}
	}
	if e.OldRow != nil {
		if val, ok := e.OldRow[name]; ok {
			return val
		}
	}
	return nil
}

// GetFieldString 获取字段的字符串值
func (e *Event) GetFieldString(name string) string {
	val := e.GetField(name)
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

// ToMap 将事件转换为 map，用于模板渲染
func (e *Event) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"Table":     e.Table,
		"Action":    string(e.Action),
		"Timestamp": e.Timestamp,
		"OldRow":    e.OldRow,
		"NewRow":    e.NewRow,
		"Schema":    e.Schema,
		"TableName": e.TableName,
	}
}
