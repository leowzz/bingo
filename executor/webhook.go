package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"bingo/engine"
	"bingo/listener"
	"bingo/utils"
)

// WebhookExecutor Webhook 执行器
type WebhookExecutor struct {
	client *http.Client
}

// Type 返回执行器类型
func (w *WebhookExecutor) Type() string {
	return "webhook"
}

// NewWebhookExecutor 创建新的 Webhook 执行器
func NewWebhookExecutor() *WebhookExecutor {
	return &WebhookExecutor{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Execute 执行 Webhook 动作
func (w *WebhookExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
	if action.URL == "" {
		return fmt.Errorf("Webhook URL 不能为空")
	}

	// 渲染 URL
	url, err := utils.RenderTemplate(action.URL, event)
	if err != nil {
		return fmt.Errorf("渲染 URL 模板失败: %w", err)
	}

	// 渲染请求体
	var body io.Reader
	if action.Body != "" {
		bodyStr, err := utils.RenderTemplate(action.Body, event)
		if err != nil {
			return fmt.Errorf("渲染 Body 模板失败: %w", err)
		}
		body = bytes.NewBufferString(bodyStr)
	}

	// 创建请求
	method := action.Method
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	if action.Headers != nil {
		for k, v := range action.Headers {
			headerValue, err := utils.RenderTemplate(v, event)
			if err != nil {
				return fmt.Errorf("渲染 Header 模板失败: %w", err)
			}
			req.Header.Set(k, headerValue)
		}
	}

	// 如果没有设置 Content-Type，默认设置为 application/json
	if req.Header.Get("Content-Type") == "" && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// 执行请求
	timeout := time.Duration(action.Timeout) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	w.client.Timeout = timeout

	// 重试逻辑
	maxRetry := action.Retry
	if maxRetry == 0 {
		maxRetry = 1
	}

	var lastErr error
	for i := 0; i < maxRetry; i++ {
		resp, err := w.client.Do(req)
		if err != nil {
			lastErr = err
			if i < maxRetry-1 {
				time.Sleep(time.Second * time.Duration(i+1)) // 指数退避
				continue
			}
			return fmt.Errorf("请求失败: %w", err)
		}

		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
		if i < maxRetry-1 {
			time.Sleep(time.Second * time.Duration(i+1))
			continue
		}
	}

	return lastErr
}
