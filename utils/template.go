package utils

import (
	"bytes"
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
			// 简单的 JSON 序列化（实际项目中可以使用 encoding/json）
			return fmt.Sprintf("%v", v)
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
			// 转换为大写开头的字段名（Go 模板约定）
			fieldName := strings.ToUpper(k[:1]) + k[1:]
			data[fieldName] = v
		}
	}
	if event.OldRow != nil {
		for k, v := range event.OldRow {
			fieldName := strings.ToUpper(k[:1]) + k[1:]
			// 如果 NewRow 中没有，才使用 OldRow 的值
			if _, exists := data[fieldName]; !exists {
				data[fieldName] = v
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
