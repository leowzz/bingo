package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"bingo/listener"
)

// RenderTemplate 渲染模板字符串
func RenderTemplate(tmplStr string, event *listener.Event) (string, error) {
	if tmplStr == "" {
		return "", nil
	}

	// 创建模板函数映射
	funcMap := template.FuncMap{
		"toJson": func(v interface{}) string {
			// 使用 encoding/json 进行真正的 JSON 序列化
			if v == nil {
				return "null"
			}
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				// 如果序列化失败，返回错误信息（但不会中断模板渲染）
				return fmt.Sprintf(`{"error": "json_marshal_failed: %v"}`, err)
			}
			return string(jsonBytes)
		},
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"trim":  strings.TrimSpace,
	}

	// 解析模板
	tmpl, err := template.New("action").Funcs(funcMap).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("解析模板失败: %w", err)
	}

	// 准备模板数据
	data := event.ToMap()

	// 添加字段访问的快捷方式（支持 .ID, .Email 等）
	// 从 NewRow 或 OldRow 中提取字段
	if event.NewRow != nil {
		for k, v := range event.NewRow {
			addFieldVariants(data, k, v)
		}
	}
	if event.OldRow != nil {
		for k, v := range event.OldRow {
			// 如果 NewRow 中没有，才使用 OldRow 的值
			if !hasFieldVariants(data, k) {
				addFieldVariants(data, k, v)
			}
		}
	}

	// 执行模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("执行模板失败: %w", err)
	}

	return buf.String(), nil
}

// addFieldVariants 添加字段的多种大小写变体到 data map
//
// 对于字段名 "id"，会添加：
//   - "Id" (首字母大写，Go 约定)
//   - "ID" (全大写，常见缩写格式)
//
// 对于其他字段如 "email"，会添加：
//   - "Email" (首字母大写)
func addFieldVariants(data map[string]interface{}, fieldName string, value interface{}) {
	if fieldName == "" {
		return
	}

	// 首字母大写格式（Go 约定）：id -> Id, email -> Email
	capitalized := strings.ToUpper(fieldName[:1]) + fieldName[1:]
	data[capitalized] = value

	// 对于短字段名（1-3个字符），同时添加全大写版本
	// 这样可以支持 .ID, .URL, .API 等常见缩写
	if len(fieldName) <= 3 {
		upper := strings.ToUpper(fieldName)
		// 如果全大写版本与首字母大写版本不同，也添加
		if upper != capitalized {
			data[upper] = value
		}
	}
}

// hasFieldVariants 检查 data map 中是否已存在该字段的任意变体
func hasFieldVariants(data map[string]interface{}, fieldName string) bool {
	if fieldName == "" {
		return false
	}

	capitalized := strings.ToUpper(fieldName[:1]) + fieldName[1:]
	if _, exists := data[capitalized]; exists {
		return true
	}

	// 检查全大写版本（如果是短字段）
	if len(fieldName) <= 3 {
		upper := strings.ToUpper(fieldName)
		if upper != capitalized {
			if _, exists := data[upper]; exists {
				return true
			}
		}
	}

	return false
}
