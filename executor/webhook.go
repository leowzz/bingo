package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"bingo/engine"
	"bingo/internal/logger"
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
	urlStr, err := utils.RenderTemplate(action.URL, event.ToMap())
	if err != nil {
		return fmt.Errorf("渲染 URL 模板失败: %w", err)
	}

	// 验证 URL 格式
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("URL 格式无效: %w, url: %s", err, urlStr)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("不支持的 URL 协议: %s, 仅支持 http/https", parsedURL.Scheme)
	}

	// 渲染请求体
	var body io.Reader
	var bodyStr string
	if action.Body != "" {
		bodyStr, err = utils.RenderTemplate(action.Body, event.ToMap())
		if err != nil {
			return fmt.Errorf("渲染 Body 模板失败: %w", err)
		}
		body = bytes.NewBufferString(bodyStr)
		logger.Debugw("Webhook 请求体", "url", urlStr, "body", bodyStr)
	}

	// 创建请求
	method := action.Method
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	if action.Headers != nil {
		for k, v := range action.Headers {
			headerValue, err := utils.RenderTemplate(v, event.ToMap())
			if err != nil {
				return fmt.Errorf("渲染 Header 模板失败: %w, header: %s", err, k)
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

	logger.Debugw("发送 Webhook 请求",
		"url", urlStr,
		"method", method,
		"timeout", timeout,
		"retry", maxRetry,
		"table", event.Table,
		"action", event.Action,
	)

	var lastErr error
	var lastResp *http.Response
	for i := 0; i < maxRetry; i++ {
		if i > 0 {
			backoff := time.Second * time.Duration(i)
			logger.Debugw("Webhook 重试", "attempt", i+1, "backoff", backoff)
			time.Sleep(backoff)
		}

		resp, err := w.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("请求失败: %w", err)
			logger.Warnw("Webhook 请求失败",
				"url", urlStr,
				"attempt", i+1,
				"error", err,
			)
			if i < maxRetry-1 {
				continue
			}
			return lastErr
		}

		lastResp = resp

		// 读取响应体（用于错误日志）
		var respBody []byte
		if resp.Body != nil {
			respBody, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			logger.Debugw("Webhook 请求成功",
				"url", urlStr,
				"status_code", resp.StatusCode,
				"table", event.Table,
				"action", event.Action,
			)
			return nil
		}

		lastErr = fmt.Errorf("HTTP 状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
		logger.Warnw("Webhook 请求返回错误状态码",
			"url", urlStr,
			"status_code", resp.StatusCode,
			"response_body", string(respBody),
			"attempt", i+1,
		)

		if i < maxRetry-1 {
			continue
		}
	}

	// 最后一次尝试也失败了
	if lastResp != nil {
		logger.Errorw("Webhook 请求最终失败",
			"url", urlStr,
			"status_code", lastResp.StatusCode,
			"retries", maxRetry,
			"table", event.Table,
			"action", event.Action,
		)
	} else {
		logger.Errorw("Webhook 请求最终失败（网络错误）",
			"url", urlStr,
			"retries", maxRetry,
			"error", lastErr,
			"table", event.Table,
			"action", event.Action,
		)
	}

	return lastErr
}
