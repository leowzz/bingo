package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"bingo/engine"
	"bingo/internal/logger"
	"bingo/listener"
	"bingo/utils"

	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	// rabbitmqConnections 存储 RabbitMQ 连接，key 为 URL
	rabbitmqConnections map[string]*amqp.Connection
	rabbitmqMutex       sync.RWMutex
)

func init() {
	rabbitmqConnections = make(map[string]*amqp.Connection)
}

// RabbitMQExecutor RabbitMQ 执行器
type RabbitMQExecutor struct{}

// Type 返回执行器类型
func (r *RabbitMQExecutor) Type() string {
	return "rabbitmq"
}

// getConnection 获取或创建 RabbitMQ 连接
func (r *RabbitMQExecutor) getConnection(url string) (*amqp.Connection, error) {
	if url == "" {
		return nil, fmt.Errorf("RabbitMQ URL 不能为空")
	}

	rabbitmqMutex.RLock()
	if conn, exists := rabbitmqConnections[url]; exists {
		// 检查连接是否仍然有效
		if !conn.IsClosed() {
			rabbitmqMutex.RUnlock()
			return conn, nil
		}
		// 连接已关闭，从 map 中移除
		delete(rabbitmqConnections, url)
	}
	rabbitmqMutex.RUnlock()

	// 创建新的连接
	rabbitmqMutex.Lock()
	defer rabbitmqMutex.Unlock()

	// 双重检查
	if conn, exists := rabbitmqConnections[url]; exists && !conn.IsClosed() {
		return conn, nil
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("创建 RabbitMQ 连接失败: %w", err)
	}

	rabbitmqConnections[url] = conn
	logger.Infow("创建 RabbitMQ 连接成功", "url", url)

	return conn, nil
}

// Execute 执行 RabbitMQ 动作
func (r *RabbitMQExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
	if action.RabbitMQUrl == "" {
		return fmt.Errorf("RabbitMQ URL 不能为空")
	}

	// 获取连接
	conn, err := r.getConnection(action.RabbitMQUrl)
	if err != nil {
		return err
	}

	// 创建通道
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("创建 RabbitMQ 通道失败: %w", err)
	}
	defer ch.Close()

	// 渲染消息内容
	var messageBody []byte
	if action.RabbitMQMessage != "" {
		messageStr, err := utils.RenderTemplate(action.RabbitMQMessage, event.ToMap())
		if err != nil {
			return fmt.Errorf("渲染消息模板失败: %w", err)
		}
		messageBody = []byte(messageStr)
	} else {
		// 如果没有指定消息，使用默认的 JSON 格式
		messageStr, err := utils.RenderTemplate("{{ .NewRow | toJson }}", event.ToMap())
		if err != nil {
			return fmt.Errorf("渲染默认消息模板失败: %w", err)
		}
		messageBody = []byte(messageStr)
	}

	// 如果指定了队列，直接发送到队列
	if action.RabbitMQQueue != "" {
		// 渲染队列名称（支持模板变量）
		queueName, err := utils.RenderTemplate(action.RabbitMQQueue, event.ToMap())
		if err != nil {
			return fmt.Errorf("渲染队列名称模板失败: %w", err)
		}

		// 声明队列（如果不存在则创建）
		_, err = ch.QueueDeclare(
			queueName, // 队列名称
			true,      // 持久化
			false,     // 非自动删除
			false,     // 非排他
			false,     // 无等待
			nil,       // 额外参数
		)
		if err != nil {
			return fmt.Errorf("声明队列失败: %w", err)
		}

		// 发送消息到队列
		err = ch.PublishWithContext(
			ctx,
			"",        // 不使用交换机
			queueName, // 路由键（队列名称）
			false,     // 非强制
			false,     // 非立即
			amqp.Publishing{
				ContentType:  "application/json",
				Body:         messageBody,
				DeliveryMode: amqp.Persistent, // 持久化消息
				Timestamp:    time.Now(),
			},
		)
		if err != nil {
			logger.Errorw("发送 RabbitMQ 消息到队列失败",
				"url", action.RabbitMQUrl,
				"queue", queueName,
				"error", err,
				"table", event.Table,
				"action", event.Action,
			)
			return fmt.Errorf("发送 RabbitMQ 消息到队列失败: %w", err)
		}

		logger.Debugw("RabbitMQ 消息发送到队列成功",
			"url", action.RabbitMQUrl,
			"queue", queueName,
			"table", event.Table,
			"action", event.Action,
		)
		return nil
	}

	// 使用交换机发送消息
	if action.RabbitMQExchange == "" {
		return fmt.Errorf("RabbitMQ 必须指定 exchange 或 queue")
	}

	// 渲染交换机和路由键（支持模板变量）
	exchange, err := utils.RenderTemplate(action.RabbitMQExchange, event.ToMap())
	if err != nil {
		return fmt.Errorf("渲染交换机名称模板失败: %w", err)
	}

	routingKey := ""
	if action.RabbitMQRoutingKey != "" {
		routingKey, err = utils.RenderTemplate(action.RabbitMQRoutingKey, event.ToMap())
		if err != nil {
			return fmt.Errorf("渲染路由键模板失败: %w", err)
		}
	}

	// 声明交换机（如果不存在则创建，使用 topic 类型）
	err = ch.ExchangeDeclare(
		exchange, // 交换机名称
		"topic",  // 交换机类型
		true,     // 持久化
		false,    // 非自动删除
		false,    // 非内部
		false,    // 无等待
		nil,      // 额外参数
	)
	if err != nil {
		return fmt.Errorf("声明交换机失败: %w", err)
	}

	// 发送消息到交换机
	err = ch.PublishWithContext(
		ctx,
		exchange,   // 交换机名称
		routingKey, // 路由键
		false,      // 非强制
		false,      // 非立即
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         messageBody,
			DeliveryMode: amqp.Persistent, // 持久化消息
			Timestamp:    time.Now(),
		},
	)
	if err != nil {
		logger.Errorw("发送 RabbitMQ 消息到交换机失败",
			"url", action.RabbitMQUrl,
			"exchange", exchange,
			"routing_key", routingKey,
			"error", err,
			"table", event.Table,
			"action", event.Action,
		)
		return fmt.Errorf("发送 RabbitMQ 消息到交换机失败: %w", err)
	}

	logger.Debugw("RabbitMQ 消息发送到交换机成功",
		"url", action.RabbitMQUrl,
		"exchange", exchange,
		"routing_key", routingKey,
		"table", event.Table,
		"action", event.Action,
	)

	return nil
}

// Close 关闭所有 RabbitMQ 连接（用于优雅关闭）
func CloseRabbitMQConnections() {
	rabbitmqMutex.Lock()
	defer rabbitmqMutex.Unlock()

	for url, conn := range rabbitmqConnections {
		if err := conn.Close(); err != nil {
			logger.Warnw("关闭 RabbitMQ 连接失败", "url", url, "error", err)
		} else {
			logger.Debugw("关闭 RabbitMQ 连接成功", "url", url)
		}
		delete(rabbitmqConnections, url)
	}
}
