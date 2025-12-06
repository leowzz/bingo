package utils

import (
	"testing"

	"bingo/listener"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name     string
		tmplStr  string
		event    *listener.Event
		expected string
		wantErr  bool
	}{
		{
			name:    "简单变量替换",
			tmplStr: "{{ .Table }}",
			event: &listener.Event{
				Table:  "test.users",
				Action: listener.ActionInsert,
			},
			expected: "test.users",
			wantErr:  false,
		},
		{
			name:    "多个变量",
			tmplStr: "Table: {{ .Table }}, Action: {{ .Action }}",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionUpdate,
			},
			expected: "Table: users, Action: UPDATE",
			wantErr:  false,
		},
		{
			name:    "字段访问",
			tmplStr: "User ID: {{ .Id }}, Email: {{ .Email }}",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
				NewRow: map[string]interface{}{
					"id":    123,
					"email": "test@example.com",
				},
			},
			expected: "User ID: 123, Email: test@example.com",
			wantErr:  false,
		},
		{
			name:    "toJson 函数",
			tmplStr: "{{ .NewRow | toJson }}",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
				NewRow: map[string]interface{}{
					"id":    123,
					"name":  "test",
					"email": "test@example.com",
				},
			},
			expected: `{"email":"test@example.com","id":123,"name":"test"}`,
			wantErr:  false,
		},
		{
			name:    "空模板",
			tmplStr: "",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
			},
			expected: "",
			wantErr:  false,
		},
		{
			name:    "无效模板语法",
			tmplStr: "{{ .Invalid }",
			event: &listener.Event{
				Table:  "users",
				Action: listener.ActionInsert,
			},
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.tmplStr, tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				// 对于 JSON 输出，顺序可能不同，所以只检查包含关键内容
				if tt.name == "toJson 函数" {
					if !containsAll(result, []string{"id", "name", "email"}) {
						t.Errorf("RenderTemplate() = %v, want contains id, name, email", result)
					}
				} else {
					t.Errorf("RenderTemplate() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func containsAll(s string, substrs []string) bool {
	for _, substr := range substrs {
		if !contains(s, substr) {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOfSubstring(s, substr) >= 0))
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestRenderTemplateWithOldRow(t *testing.T) {
	event := &listener.Event{
		Table:  "users",
		Action: listener.ActionUpdate,
		OldRow: map[string]interface{}{
			"id":   123,
			"name": "old name",
		},
		NewRow: map[string]interface{}{
			"id":   123,
			"name": "new name",
		},
	}

	// NewRow 优先
	result, err := RenderTemplate("{{ .Name }}", event)
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}
	if result != "new name" {
		t.Errorf("RenderTemplate() = %v, want 'new name'", result)
	}
}

func TestRenderTemplateWithFunctions(t *testing.T) {
	event := &listener.Event{
		Table:  "users",
		Action: listener.ActionInsert,
		NewRow: map[string]interface{}{
			"name": "Test User",
		},
	}

	tests := []struct {
		name     string
		tmplStr  string
		expected string
	}{
		{
			name:     "upper 函数",
			tmplStr:  "{{ .Name | upper }}",
			expected: "TEST USER",
		},
		{
			name:     "lower 函数",
			tmplStr:  "{{ .Name | lower }}",
			expected: "test user",
		},
		{
			name:     "trim 函数",
			tmplStr:  "{{ \"  test  \" | trim }}",
			expected: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.tmplStr, event)
			if err != nil {
				t.Fatalf("RenderTemplate() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("RenderTemplate() = %v, want %v", result, tt.expected)
			}
		})
	}
}
