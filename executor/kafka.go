package executor

import (
	"context"
	"fmt"
	"sync"

	"bingo/engine"
	"bingo/internal/logger"
	"bingo/listener"
	"bingo/utils"

	"github.com/IBM/sarama"
	"github.com/xdg-go/scram"
)

var (
	// kafkaProducers 存储 Kafka 生产者，key 为 brokers 的字符串表示
	kafkaProducers map[string]sarama.SyncProducer
	kafkaMutex     sync.RWMutex
)

func init() {
	kafkaProducers = make(map[string]sarama.SyncProducer)
}

// KafkaExecutor Kafka 执行器
type KafkaExecutor struct{}

// Type 返回执行器类型
func (k *KafkaExecutor) Type() string {
	return "kafka"
}

// getProducer 获取或创建 Kafka 生产者
func (k *KafkaExecutor) getProducer(brokers []string, action engine.Action) (sarama.SyncProducer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("Kafka brokers 不能为空")
	}

	// 使用 brokers 和认证信息作为 key（不同认证信息使用不同的 producer）
	key := fmt.Sprintf("%v|%s|%s|%s", brokers, action.KafkaSASLMechanism, action.KafkaUsername, action.KafkaPassword)

	kafkaMutex.RLock()
	if producer, exists := kafkaProducers[key]; exists {
		kafkaMutex.RUnlock()
		return producer, nil
	}
	kafkaMutex.RUnlock()

	// 创建新的生产者
	kafkaMutex.Lock()
	defer kafkaMutex.Unlock()

	// 双重检查
	if producer, exists := kafkaProducers[key]; exists {
		return producer, nil
	}

	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 3

	// 配置 SASL 认证（如果提供了认证信息）
	if action.KafkaSASLMechanism != "" && action.KafkaUsername != "" && action.KafkaPassword != "" {
		config.Net.SASL.Enable = true
		config.Net.SASL.Mechanism = sarama.SASLMechanism(action.KafkaSASLMechanism)
		config.Net.SASL.User = action.KafkaUsername
		config.Net.SASL.Password = action.KafkaPassword
		config.Net.SASL.Handshake = true

		// 根据机制类型设置 SCRAM 客户端生成器
		switch action.KafkaSASLMechanism {
		case "SCRAM-SHA-256":
			config.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &XDGSCRAMClient{HashGeneratorFcn: scram.SHA256}
			}
		case "SCRAM-SHA-512":
			config.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &XDGSCRAMClient{HashGeneratorFcn: scram.SHA512}
			}
		case "PLAIN":
			// PLAIN 机制不需要特殊配置
		default:
			return nil, fmt.Errorf("不支持的 SASL 机制: %s，支持: PLAIN, SCRAM-SHA-256, SCRAM-SHA-512", action.KafkaSASLMechanism)
		}

		logger.Debugw("配置 Kafka SASL 认证",
			"mechanism", action.KafkaSASLMechanism,
			"username", action.KafkaUsername,
		)
	}

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("创建 Kafka 生产者失败: %w", err)
	}

	kafkaProducers[key] = producer
	logger.Infow("创建 Kafka 生产者成功",
		"brokers", brokers,
		"sasl_enabled", action.KafkaSASLMechanism != "",
		"mechanism", action.KafkaSASLMechanism,
	)

	return producer, nil
}

// Execute 执行 Kafka 动作
func (k *KafkaExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
	if len(action.Brokers) == 0 {
		return fmt.Errorf("Kafka brokers 不能为空")
	}

	if action.Topic == "" {
		return fmt.Errorf("Kafka topic 不能为空")
	}

	// 获取生产者
	producer, err := k.getProducer(action.Brokers, action)
	if err != nil {
		return err
	}

	// 渲染 topic（支持模板变量）
	topic, err := utils.RenderTemplate(action.Topic, event.ToMap())
	if err != nil {
		return fmt.Errorf("渲染 topic 模板失败: %w", err)
	}

	// 渲染 key（可选）
	var key []byte
	if action.Key != "" {
		keyStr, err := utils.RenderTemplate(action.Key, event.ToMap())
		if err != nil {
			return fmt.Errorf("渲染 key 模板失败: %w", err)
		}
		key = []byte(keyStr)
	}

	// 渲染 value
	var value []byte
	if action.Value != "" {
		valueStr, err := utils.RenderTemplate(action.Value, event.ToMap())
		if err != nil {
			return fmt.Errorf("渲染 value 模板失败: %w", err)
		}
		value = []byte(valueStr)
	} else {
		// 如果没有指定 value，使用默认的 JSON 格式
		valueStr, err := utils.RenderTemplate("{{ .NewRow | toJson }}", event.ToMap())
		if err != nil {
			return fmt.Errorf("渲染默认 value 模板失败: %w", err)
		}
		value = []byte(valueStr)
	}

	// 构建消息
	msg := &sarama.ProducerMessage{
		Topic:     topic,
		Key:       sarama.ByteEncoder(key),
		Value:     sarama.ByteEncoder(value),
		Partition: action.Partition, // 如果为 0，Kafka 会自动选择分区
	}

	logger.Debugw("发送 Kafka 消息",
		"brokers", action.Brokers,
		"topic", topic,
		"partition", action.Partition,
		"has_key", len(key) > 0,
		"value_length", len(value),
		"table", event.Table,
		"action", event.Action,
	)

	// 发送消息
	partition, offset, err := producer.SendMessage(msg)
	if err != nil {
		logger.Errorw("发送 Kafka 消息失败",
			"brokers", action.Brokers,
			"topic", topic,
			"error", err,
			"table", event.Table,
			"action", event.Action,
		)
		return fmt.Errorf("发送 Kafka 消息失败: %w", err)
	}

	logger.Debugw("Kafka 消息发送成功",
		"brokers", action.Brokers,
		"topic", topic,
		"partition", partition,
		"offset", offset,
		"table", event.Table,
		"action", event.Action,
	)

	return nil
}

// XDGSCRAMClient 实现 SCRAM 客户端
type XDGSCRAMClient struct {
	*scram.Client
	*scram.ClientConversation
	scram.HashGeneratorFcn
}

// Begin 开始 SCRAM 认证
func (x *XDGSCRAMClient) Begin(userName, password, authzID string) (err error) {
	x.Client, err = x.HashGeneratorFcn.NewClient(userName, password, authzID)
	if err != nil {
		return err
	}
	x.ClientConversation = x.Client.NewConversation()
	return nil
}

// Step 执行 SCRAM 步骤
func (x *XDGSCRAMClient) Step(challenge string) (response string, err error) {
	response, err = x.ClientConversation.Step(challenge)
	return
}

// Done 完成 SCRAM 认证
func (x *XDGSCRAMClient) Done() bool {
	return x.ClientConversation.Done()
}


// Close 关闭所有 Kafka 生产者（用于优雅关闭）
func CloseKafkaProducers() {
	kafkaMutex.Lock()
	defer kafkaMutex.Unlock()

	for key, producer := range kafkaProducers {
		if err := producer.Close(); err != nil {
			logger.Warnw("关闭 Kafka 生产者失败", "brokers", key, "error", err)
		} else {
			logger.Debugw("关闭 Kafka 生产者成功", "brokers", key)
		}
		delete(kafkaProducers, key)
	}
}
