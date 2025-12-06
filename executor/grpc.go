package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"bingo/engine"
	"bingo/internal/logger"
	"bingo/listener"
	"bingo/utils"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GrpcExecutor gRPC 执行器
// 注意：这是一个简化实现，使用 JSON 格式的请求
// 实际使用中，gRPC 服务需要支持 JSON 编码，或者通过 grpc-gateway 等工具
type GrpcExecutor struct {
	connections map[string]*grpc.ClientConn
}

// Type 返回执行器类型
func (g *GrpcExecutor) Type() string {
	return "grpc"
}

// getConnection 获取或创建 gRPC 连接
func (g *GrpcExecutor) getConnection(endpoint string) (*grpc.ClientConn, error) {
	if g.connections == nil {
		g.connections = make(map[string]*grpc.ClientConn)
	}

	if conn, exists := g.connections[endpoint]; exists {
		return conn, nil
	}

	// 创建新的连接
	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 gRPC 连接失败: %w", err)
	}

	g.connections[endpoint] = conn
	logger.Infow("创建 gRPC 连接成功", "endpoint", endpoint)

	return conn, nil
}

// Execute 执行 gRPC 动作
// 注意：这是一个简化实现，实际使用中可能需要根据具体的 proto 定义进行调整
func (g *GrpcExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
	if action.Endpoint == "" {
		return fmt.Errorf("gRPC endpoint 不能为空")
	}

	if action.Service == "" {
		return fmt.Errorf("gRPC service 不能为空")
	}

	if action.GrpcMethod == "" {
		return fmt.Errorf("gRPC method 不能为空")
	}

	// 渲染请求体
	var requestJSON string
	if action.Request != "" {
		rendered, err := utils.RenderTemplate(action.Request, event)
		if err != nil {
			return fmt.Errorf("渲染 request 模板失败: %w", err)
		}
		requestJSON = rendered
	} else {
		// 如果没有指定 request，使用默认格式
		defaultRequest := map[string]interface{}{
			"table":   event.Table,
			"action":  string(event.Action),
			"new_row": event.NewRow,
			"old_row": event.OldRow,
		}
		jsonBytes, err := json.Marshal(defaultRequest)
		if err != nil {
			return fmt.Errorf("序列化默认请求失败: %w", err)
		}
		requestJSON = string(jsonBytes)
	}

	// 验证 JSON 格式
	var requestData map[string]interface{}
	if err := json.Unmarshal([]byte(requestJSON), &requestData); err != nil {
		return fmt.Errorf("request 不是有效的 JSON 格式: %w", err)
	}

	logger.Debugw("准备调用 gRPC 服务",
		"endpoint", action.Endpoint,
		"service", action.Service,
		"method", action.GrpcMethod,
		"request", requestJSON,
		"table", event.Table,
		"action", event.Action,
	)

	// 注意：这里是一个简化实现
	// 实际的 gRPC 调用需要知道具体的 proto 定义
	// 这里我们记录日志并返回一个说明，实际使用时需要根据具体的服务定义实现
	logger.Warnw("gRPC 执行器简化实现",
		"endpoint", action.Endpoint,
		"service", action.Service,
		"method", action.GrpcMethod,
		"note", "需要根据具体的 proto 定义实现实际的 gRPC 调用",
	)

	// TODO: 实现实际的 gRPC 调用
	// 这需要：
	// 1. 加载 proto 文件或使用反射
	// 2. 根据 service 和 method 构建调用
	// 3. 将 JSON 请求转换为 protobuf 消息
	// 4. 执行调用并处理响应

	// 当前实现：尝试建立连接以验证配置
	conn, err := g.getConnection(action.Endpoint)
	if err != nil {
		return err
	}

	// 创建一个带超时的上下文
	timeout := time.Duration(action.Timeout) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 添加元数据（如果需要）
	md := metadata.New(map[string]string{
		"table":  event.Table,
		"action": string(event.Action),
	})
	callCtx = metadata.NewOutgoingContext(callCtx, md)

	// 这里应该执行实际的 gRPC 调用
	// 由于需要 proto 定义，我们暂时只验证连接
	_ = conn
	_ = callCtx

	logger.Infow("gRPC 调用准备完成（简化实现）",
		"endpoint", action.Endpoint,
		"service", action.Service,
		"method", action.GrpcMethod,
		"table", event.Table,
		"action", event.Action,
	)

	// 返回一个提示，说明需要完整实现
	return fmt.Errorf("gRPC 执行器需要根据具体的 proto 定义实现，当前为简化版本。请参考文档实现完整的 gRPC 调用逻辑")
}

// Close 关闭所有 gRPC 连接
func (g *GrpcExecutor) Close() {
	if g.connections == nil {
		return
	}

	for endpoint, conn := range g.connections {
		if err := conn.Close(); err != nil {
			logger.Warnw("关闭 gRPC 连接失败", "endpoint", endpoint, "error", err)
		} else {
			logger.Debugw("关闭 gRPC 连接成功", "endpoint", endpoint)
		}
	}
	g.connections = nil
}
