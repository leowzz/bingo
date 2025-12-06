package engine

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Rule 规则定义
type Rule struct {
	ID      string       `yaml:"id"`
	Name    string       `yaml:"name"`
	Table   string       `yaml:"table"`
	Events  []string     `yaml:"events"`
	Filter  string       `yaml:"filter,omitempty"`
	Actions []Action     `yaml:"actions"`
	Batch   *BatchConfig `yaml:"batch,omitempty"`
}

// Action 动作定义
type Action struct {
	Type       string            `yaml:"type"`
	Cmd        string            `yaml:"cmd,omitempty"`        // Redis 命令
	Key        string            `yaml:"key,omitempty"`        // Redis key
	Keys       []string          `yaml:"keys,omitempty"`       // Redis keys (批量)
	Value      string            `yaml:"value,omitempty"`      // Redis value
	TTL        int               `yaml:"ttl,omitempty"`        // Redis TTL
	RedisConn  string            `yaml:"redis_conn,omitempty"` // Redis 连接名称（从规则配置的 redis_connections 中选择）
	URL        string            `yaml:"url,omitempty"`        // Webhook URL
	Method     string            `yaml:"method,omitempty"`     // HTTP Method
	Headers    map[string]string `yaml:"headers,omitempty"`
	Body       string            `yaml:"body,omitempty"`
	Timeout    int               `yaml:"timeout,omitempty"`
	Retry      int               `yaml:"retry,omitempty"`
	Brokers    []string          `yaml:"brokers,omitempty"`     // Kafka brokers
	Topic      string            `yaml:"topic,omitempty"`       // Kafka topic
	Partition  int32             `yaml:"partition,omitempty"`   // Kafka partition
	Endpoint   string            `yaml:"endpoint,omitempty"`    // gRPC endpoint
	Service    string            `yaml:"service,omitempty"`     // gRPC service name
	GrpcMethod string            `yaml:"grpc_method,omitempty"` // gRPC method name
	Request    string            `yaml:"request,omitempty"`     // gRPC request (JSON)
	Message    string            `yaml:"message,omitempty"`     // Log message
	Level      string            `yaml:"level,omitempty"`       // Log level
}

// BatchConfig 批量处理配置
type BatchConfig struct {
	Enabled bool `yaml:"enabled"`
	Window  int  `yaml:"window"`   // 时间窗口（毫秒）
	MaxSize int  `yaml:"max_size"` // 最大聚合数量
}

// RulesConfig 规则配置
type RulesConfig struct {
	RedisConnections []RedisConnectionConfig `yaml:"redis_connections,omitempty"` // Redis 连接配置
	Rules            []Rule                  `yaml:"rules"`
}

// RedisConnectionConfig Redis 连接配置（在规则文件中定义）
type RedisConnectionConfig struct {
	Name     string `yaml:"name"`     // 连接名称
	Addr     string `yaml:"addr"`     // Redis 服务器地址
	Password string `yaml:"password"` // Redis 密码
	DB       int    `yaml:"db"`       // Redis 数据库编号
}

// LoadRules 从文件加载规则
func LoadRules(path string) ([]Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取规则文件失败: %w", err)
	}

	var config RulesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析规则文件失败: %w", err)
	}

	// 验证规则
	for i := range config.Rules {
		if err := validateRule(&config.Rules[i]); err != nil {
			return nil, fmt.Errorf("规则 %s 验证失败: %w", config.Rules[i].ID, err)
		}
	}

	return config.Rules, nil
}

// LoadRulesWithRedisConnections 从文件加载规则和 Redis 连接配置
func LoadRulesWithRedisConnections(path string) (*RulesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取规则文件失败: %w", err)
	}

	var config RulesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析规则文件失败: %w", err)
	}

	// 验证规则
	for i := range config.Rules {
		if err := validateRule(&config.Rules[i]); err != nil {
			return nil, fmt.Errorf("规则 %s 验证失败: %w", config.Rules[i].ID, err)
		}
	}

	return &config, nil
}

// validateRule 验证规则
func validateRule(rule *Rule) error {
	if rule.ID == "" {
		return fmt.Errorf("规则 ID 不能为空")
	}
	if rule.Table == "" {
		return fmt.Errorf("规则表名不能为空")
	}
	if len(rule.Events) == 0 {
		return fmt.Errorf("规则事件列表不能为空")
	}
	if len(rule.Actions) == 0 {
		return fmt.Errorf("规则动作列表不能为空")
	}

	// 验证事件类型
	validEvents := map[string]bool{
		"INSERT": true,
		"UPDATE": true,
		"DELETE": true,
	}
	for _, event := range rule.Events {
		if !validEvents[event] {
			return fmt.Errorf("无效的事件类型: %s", event)
		}
	}

	return nil
}
