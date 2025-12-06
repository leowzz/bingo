package engine

import (
	"testing"
	"time"

	"bingo/listener"
)

func TestEvaluateFilter(t *testing.T) {
	tests := []struct {
		name     string
		filter   string
		event    *listener.Event
		expected bool
		wantErr  bool
	}{
		{
			name:   "空过滤器",
			filter: "",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:   "true 过滤器",
			filter: "true",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:   "false 过滤器",
			filter: "false",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
			},
			expected: false,
			wantErr:  false,
		},
		{
			name:   "比较 Action",
			filter: "Action == 'INSERT'",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:   "比较 Table",
			filter: "Table == 'users'",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:   "访问 NewRow 字段",
			filter: "NewRow['status'] == 'active'",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
				NewRow: map[string]interface{}{
					"status": "active",
				},
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:   "访问 OldRow 字段",
			filter: "OldRow['status'] == 'inactive'",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionUpdate,
				OldRow: map[string]interface{}{
					"status": "inactive",
				},
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:   "复杂表达式",
			filter: "Action == 'UPDATE' && NewRow['status'] == 'completed' && OldRow['status'] != 'completed'",
			event: &listener.Event{
				Table:  "orders",
				Action: listener.ActionUpdate,
				OldRow: map[string]interface{}{
					"status": "pending",
				},
				NewRow: map[string]interface{}{
					"status": "completed",
				},
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:   "数值比较",
			filter: "NewRow['id'] > 100",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
				NewRow: map[string]interface{}{
					"id": 200,
				},
			},
			expected: true,
			wantErr:  false,
		},
		{
			name:   "无效表达式",
			filter: "Invalid syntax {{",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
			},
			expected: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EvaluateFilter(tt.filter, tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("EvaluateFilter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEvaluateFilterWithComplexData(t *testing.T) {
	event := &listener.Event{
		Table:     "test.orders",
		Action:    listener.ActionUpdate,
		Timestamp: time.Now(),
		Schema:    "test",
		TableName: "orders",
		OldRow: map[string]interface{}{
			"id":     100,
			"status": "pending",
			"amount": 99.99,
		},
		NewRow: map[string]interface{}{
			"id":     100,
			"status": "completed",
			"amount": 99.99,
		},
	}

	// 测试状态变更检测
	filter := "NewRow['status'] == 'completed' && OldRow['status'] == 'pending'"
	result, err := EvaluateFilter(filter, event)
	if err != nil {
		t.Fatalf("EvaluateFilter() error = %v", err)
	}
	if !result {
		t.Error("EvaluateFilter() should return true for status change")
	}

	// 测试金额比较
	filter2 := "NewRow['amount'] > 50.0"
	result2, err := EvaluateFilter(filter2, event)
	if err != nil {
		t.Fatalf("EvaluateFilter() error = %v", err)
	}
	if !result2 {
		t.Error("EvaluateFilter() should return true for amount > 50")
	}
}
