package listener

import (
	"testing"
	"time"
)

func TestEvent_GetField(t *testing.T) {
	tests := []struct {
		name     string
		event    *Event
		field    string
		expected interface{}
	}{
		{
			name: "从 NewRow 获取字段",
			event: &Event{
				NewRow: map[string]interface{}{
					"id":   123,
					"name": "test",
				},
			},
			field:    "id",
			expected: 123,
		},
		{
			name: "从 OldRow 获取字段（NewRow 不存在）",
			event: &Event{
				OldRow: map[string]interface{}{
					"id":   456,
					"name": "old",
				},
			},
			field:    "id",
			expected: 456,
		},
		{
			name: "NewRow 优先于 OldRow",
			event: &Event{
				NewRow: map[string]interface{}{
					"id": 123,
				},
				OldRow: map[string]interface{}{
					"id": 456,
				},
			},
			field:    "id",
			expected: 123,
		},
		{
			name: "字段不存在",
			event: &Event{
				NewRow: map[string]interface{}{
					"name": "test",
				},
			},
			field:    "id",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.event.GetField(tt.field)
			if result != tt.expected {
				t.Errorf("GetField() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEvent_GetFieldString(t *testing.T) {
	event := &Event{
		NewRow: map[string]interface{}{
			"id":   123,
			"name": "test",
		},
	}

	tests := []struct {
		name     string
		field    string
		expected string
	}{
		{
			name:     "整数转字符串",
			field:    "id",
			expected: "123",
		},
		{
			name:     "字符串字段",
			field:    "name",
			expected: "test",
		},
		{
			name:     "不存在的字段",
			field:    "email",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := event.GetFieldString(tt.field)
			if result != tt.expected {
				t.Errorf("GetFieldString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEvent_ToMap(t *testing.T) {
	event := &Event{
		Table:     "test.users",
		Action:    ActionInsert,
		Timestamp: time.Now(),
		Schema:    "test",
		TableName: "users",
		NewRow: map[string]interface{}{
			"id":   123,
			"name": "test",
		},
		OldRow: map[string]interface{}{
			"id": 122,
		},
	}

	result := event.ToMap()

	if result["Table"] != event.Table {
		t.Errorf("ToMap() Table = %v, want %v", result["Table"], event.Table)
	}
	if result["Action"] != string(event.Action) {
		t.Errorf("ToMap() Action = %v, want %v", result["Action"], event.Action)
	}
	if result["Schema"] != event.Schema {
		t.Errorf("ToMap() Schema = %v, want %v", result["Schema"], event.Schema)
	}
	if result["TableName"] != event.TableName {
		t.Errorf("ToMap() TableName = %v, want %v", result["TableName"], event.TableName)
	}
	if result["NewRow"] == nil {
		t.Error("ToMap() NewRow should not be nil")
	}
	if result["OldRow"] == nil {
		t.Error("ToMap() OldRow should not be nil")
	}
}
