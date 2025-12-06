package listener

import (
	"fmt"
	"sync"
	"time"
)

// Action 事件动作类型
type Action string

const (
	ActionInsert Action = "INSERT"
	ActionUpdate Action = "UPDATE"
	ActionDelete Action = "DELETE"
	
	// Action字符串常量（避免string转换分配）
	ActionInsertStr = "INSERT"
	ActionUpdateStr = "UPDATE"
	ActionDeleteStr = "DELETE"
)

var (
	// eventPool Event对象池，用于复用Event对象，减少内存分配
	eventPool = sync.Pool{
		New: func() interface{} {
			return &Event{
				OldRow: make(map[string]interface{}),
				NewRow: make(map[string]interface{}),
			}
		},
	}
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

// GetEventFromPool 从对象池获取Event对象
//
// 使用对象池复用Event对象，减少内存分配。
// 使用完成后必须调用 PutEventToPool 归还对象。
func GetEventFromPool() *Event {
	return eventPool.Get().(*Event)
}

// PutEventToPool 将Event对象归还到对象池
//
// 清空Event的所有字段但保留map容量，以便下次复用。
func PutEventToPool(e *Event) {
	// 清空基本字段
	e.Table = ""
	e.Action = ""
	e.Timestamp = time.Time{}
	e.Schema = ""
	e.TableName = ""
	
	// 清空map但保留容量（减少重新分配）
	if e.OldRow != nil {
		for k := range e.OldRow {
			delete(e.OldRow, k)
		}
	} else {
		e.OldRow = make(map[string]interface{})
	}
	
	if e.NewRow != nil {
		for k := range e.NewRow {
			delete(e.NewRow, k)
		}
	} else {
		e.NewRow = make(map[string]interface{})
	}
	
	eventPool.Put(e)
}

// GetActionString 获取Action的字符串表示（零分配）
//
// 使用常量字符串，避免string转换时的内存分配。
func GetActionString(action Action) string {
	switch action {
	case ActionInsert:
		return ActionInsertStr
	case ActionUpdate:
		return ActionUpdateStr
	case ActionDelete:
		return ActionDeleteStr
	default:
		return string(action)
	}
}
