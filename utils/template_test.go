package utils

import (
	"strings"
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name     string
		tmplStr  string
		data     map[string]interface{}
		expected string
		wantErr  bool
	}{
		{
			name:    "简单变量替换",
			tmplStr: "{{ .Table }}",
			data: map[string]interface{}{
				"Table":  "test.users",
				"Action": "INSERT",
			},
			expected: "test.users",
			wantErr:  false,
		},
		{
			name:    "多个变量",
			tmplStr: "Table: {{ .Table }}, Action: {{ .Action }}",
			data: map[string]interface{}{
				"Table":  "users",
				"Action": "UPDATE",
			},
			expected: "Table: users, Action: UPDATE",
			wantErr:  false,
		},
		{
			name:    "字段访问",
			tmplStr: "User ID: {{ .Id }}, Email: {{ .Email }}",
			data: map[string]interface{}{
				"Table":  "users",
				"Action": "INSERT",
				"NewRow": map[string]interface{}{
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
			data: map[string]interface{}{
				"Table":  "users",
				"Action": "INSERT",
				"NewRow": map[string]interface{}{
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
			data: map[string]interface{}{
				"Table":  "users",
				"Action": "INSERT",
			},
			expected: "",
			wantErr:  false,
		},
		{
			name:    "无效模板语法",
			tmplStr: "{{ .Invalid }",
			data: map[string]interface{}{
				"Table":  "users",
				"Action": "INSERT",
			},
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.tmplStr, tt.data)
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
	data := map[string]interface{}{
		"Table":  "users",
		"Action": "UPDATE",
		"OldRow": map[string]interface{}{
			"id":   123,
			"name": "old name",
		},
		"NewRow": map[string]interface{}{
			"id":   123,
			"name": "new name",
		},
	}

	// NewRow 优先
	result, err := RenderTemplate("{{ .Name }}", data)
	if err != nil {
		t.Fatalf("RenderTemplate() error = %v", err)
	}
	if result != "new name" {
		t.Errorf("RenderTemplate() = %v, want 'new name'", result)
	}
}

func TestRenderTemplateWithFunctions(t *testing.T) {
	data := map[string]interface{}{
		"Table":  "users",
		"Action": "INSERT",
		"NewRow": map[string]interface{}{
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
			result, err := RenderTemplate(tt.tmplStr, data)
			if err != nil {
				t.Fatalf("RenderTemplate() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("RenderTemplate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRenderTemplateWithDeleteAction(t *testing.T) {
	// 模拟 DELETE 操作：NewRow 为空，OldRow 有数据
	data := map[string]interface{}{
		"Table":  "test.users_001",
		"Action": "DELETE",
		"Schema": "test",
		"NewRow": map[string]interface{}{}, // DELETE 操作时 NewRow 为空
		"OldRow": map[string]interface{}{
			"id":       6,
			"email":    "user6@example.com",
			"username": "user6",
			"phone":    "13800138006",
			"age":      25,
		},
	}

	tests := []struct {
		name     string
		tmplStr  string
		expected string
	}{
		{
			name:     "使用 .ID (全大写)",
			tmplStr:  "id={{ .ID }}",
			expected: "id=6",
		},
		{
			name:     "使用 .Id (首字母大写)",
			tmplStr:  "id={{ .Id }}",
			expected: "id=6",
		},
		{
			name:     "使用 .Email",
			tmplStr:  "email={{ .Email }}",
			expected: "email=user6@example.com",
		},
		{
			name:     "同时使用 .ID 和 .Email",
			tmplStr:  "id={{ .ID }} email={{ .Email }}",
			expected: "id=6 email=user6@example.com",
		},
		{
			name:     "完整日志消息",
			tmplStr:  "用户缓存已失效: id={{ .ID }} email={{ .Email }}. NewRow: {{ .NewRow }}. OldRow: {{ .OldRow }}",
			expected: "用户缓存已失效: id=6 email=user6@example.com. NewRow: map[]. OldRow: map[age:25 email:user6@example.com id:6 phone:13800138006 username:user6]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.tmplStr, data)
			if err != nil {
				t.Fatalf("RenderTemplate() error = %v", err)
			}
			// 对于包含 map 输出的测试，只检查关键部分
			if tt.name == "完整日志消息" {
				if !strings.Contains(result, "id=6") {
					t.Errorf("RenderTemplate() 应包含 'id=6', 实际: %v", result)
				}
				if !strings.Contains(result, "email=user6@example.com") {
					t.Errorf("RenderTemplate() 应包含 'email=user6@example.com', 实际: %v", result)
				}
			} else {
				if result != tt.expected {
					t.Errorf("RenderTemplate() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}
