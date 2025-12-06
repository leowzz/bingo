package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	MySQL       MySQLConfig       `yaml:"mysql"`
	Redis       RedisConfig       `yaml:"redis"`
	Binlog      BinlogConfig      `yaml:"binlog"`
	RulesFile   string            `yaml:"rules_file"`
	Performance PerformanceConfig `yaml:"performance"`
	Logging     LoggingConfig     `yaml:"logging"`
}

// MySQLConfig MySQL 连接配置
type MySQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

// Addr 返回 MySQL 地址
func (m MySQLConfig) Addr() string {
	return fmt.Sprintf("%s:%d", m.Host, m.Port)
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// BinlogConfig Binlog 位置配置
type BinlogConfig struct {
	File          string `yaml:"file"`
	Position      uint32 `yaml:"position"`
	UseRedisStore bool   `yaml:"use_redis_store"` // 是否使用 Redis 存储位置
	RedisStoreKey string `yaml:"redis_store_key"` // Redis 存储键名
}

// PerformanceConfig 性能配置
type PerformanceConfig struct {
	DebounceWindow int `yaml:"debounce_window"`  // 防抖窗口（毫秒）
	BatchSize      int `yaml:"batch_size"`       // 批量处理大小
	Concurrency    int `yaml:"concurrency"`      // 并发处理数
	QueueSize      int `yaml:"queue_size"`       // 事件队列大小
	WorkerPoolSize int `yaml:"worker_pool_size"` // 工作池大小
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level      string `yaml:"level"`       // debug, info, warn, error
	Format     string `yaml:"format"`      // json, text
	Output     string `yaml:"output"`      // stdout, file
	File       string `yaml:"file"`        // 日志文件路径
	MaxSize    int    `yaml:"max_size"`    // 最大文件大小（MB）
	MaxBackups int    `yaml:"max_backups"` // 最大备份数
	MaxAge     int    `yaml:"max_age"`     // 最大保留天数
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	cfg.setDefaults()

	return &cfg, nil
}

// setDefaults 设置默认值
func (c *Config) setDefaults() {
	if c.Performance.DebounceWindow == 0 {
		c.Performance.DebounceWindow = 100
	}
	if c.Performance.BatchSize == 0 {
		c.Performance.BatchSize = 100
	}
	if c.Performance.Concurrency == 0 {
		c.Performance.Concurrency = 10
	}
	if c.Performance.QueueSize == 0 {
		c.Performance.QueueSize = 10000
	}
	if c.Performance.WorkerPoolSize == 0 {
		c.Performance.WorkerPoolSize = 50
	}
	if c.RulesFile == "" {
		c.RulesFile = "rules.yaml"
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "text"
	}
	if c.Logging.Output == "" {
		c.Logging.Output = "stdout"
	}
	if c.Binlog.RedisStoreKey == "" {
		c.Binlog.RedisStoreKey = "bingo:binlog:position"
	}
}
