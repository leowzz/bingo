package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	MySQL       MySQLConfig       `yaml:"mysql"`
	SystemRedis RedisConfig       `yaml:"system_redis"` // 系统使用的 Redis（用于位置存储等）
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

// ConnCfgHash 计算 MySQL 连接配置的 hash 值
//
// 使用 SHA256 计算连接配置的 hash，用于区分不同的 MySQL 连接。
// hash 基于 host、port、user、password 和 database 计算。
//
// :return: 16 进制字符串形式的 hash 值（前 12 个字符）
func (m MySQLConfig) ConnCfgHash() string {
	// 构建用于 hash 的字符串
	hashInput := fmt.Sprintf("%s:%d:%s:%s:%s", m.Host, m.Port, m.User, m.Password, m.Database)
	
	// 计算 SHA256 hash
	hash := sha256.Sum256([]byte(hashInput))
	
	// 返回前 12 个字符（24 个十六进制字符的一半，足够区分）
	return hex.EncodeToString(hash[:])[:12]
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// RedisConnection Redis 连接配置（用于规则中的 Redis 动作）
type RedisConnection struct {
	Name     string `yaml:"name"`     // 连接名称
	Addr     string `yaml:"addr"`     // Redis 服务器地址
	Password string `yaml:"password"` // Redis 密码
	DB       int    `yaml:"db"`       // Redis 数据库编号
}

// BinlogConfig Binlog 位置配置
type BinlogConfig struct {
	File              string `yaml:"file"`
	Position          uint32 `yaml:"position"`
	UseRedisStore     bool   `yaml:"use_redis_store"`     // 是否使用 Redis 存储位置
	RedisStoreKey     string `yaml:"redis_store_key"`     // Redis 存储键名
	SaveInterval      int    `yaml:"save_interval"`       // 位置保存间隔（秒），默认 5 秒，设置为 0 禁用定期保存
	SaveOnTransaction bool   `yaml:"save_on_transaction"` // 是否在事务提交时保存位置，默认 true
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
	Level             string `yaml:"level"`               // debug, info, warn, error
	Format            string `yaml:"format"`              // json, text
	Output            string `yaml:"output"`              // stdout, file
	File              string `yaml:"file"`                // 日志文件路径
	MaxSize           int    `yaml:"max_size"`            // 最大文件大小（MB）
	MaxBackups        int    `yaml:"max_backups"`         // 最大备份数
	MaxAge            int    `yaml:"max_age"`             // 最大保留天数
	EnableGoroutineID bool   `yaml:"enable_goroutine_id"` // 是否在日志中打印 goroutine ID（会影响性能），默认 true
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

	// 解析 redis_store_key 中的模板
	if err := cfg.resolveRedisStoreKeyTemplate(); err != nil {
		return nil, fmt.Errorf("解析 redis_store_key 模板失败: %w", err)
	}

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
	// EnableGoroutineID 默认为 true
	// 注意：由于 bool 类型无法区分"未设置"和"设置为 false"（零值都是 false）
	// 我们采用在配置模板中默认设置为 true 的方式
	// 如果用户需要禁用，必须在配置文件中显式设置 enable_goroutine_id: false
	// 这里不做默认值设置，保持配置文件中的值
	if c.Binlog.RedisStoreKey == "" {
		c.Binlog.RedisStoreKey = "bingo:binlog:position"
	}
	if c.Binlog.SaveInterval == 0 {
		c.Binlog.SaveInterval = 5 // 默认 5 秒
	}
	if c.Binlog.SaveOnTransaction {
		// 默认值已经是 true，但如果显式设置为 false，则保持 false
		// 这里不需要额外处理
	} else {
		// 如果未设置，默认为 true
		c.Binlog.SaveOnTransaction = true
	}
}

// resolveRedisStoreKeyTemplate 解析 redis_store_key 中的模板
//
// 支持 {{ .ConnCfgHash }} 模板变量，会自动替换为 MySQL 连接配置的 hash 值。
func (c *Config) resolveRedisStoreKeyTemplate() error {
	if c.Binlog.RedisStoreKey == "" {
		return nil
	}

	// 检查是否包含模板语法
	if !containsTemplate(c.Binlog.RedisStoreKey) {
		return nil
	}

	// 准备模板数据
	tmplData := struct {
		ConnCfgHash string
	}{
		ConnCfgHash: c.MySQL.ConnCfgHash(),
	}

	// 解析并执行模板
	tmpl, err := template.New("redis_store_key").Parse(c.Binlog.RedisStoreKey)
	if err != nil {
		return fmt.Errorf("解析模板失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, tmplData); err != nil {
		return fmt.Errorf("执行模板失败: %w", err)
	}

	c.Binlog.RedisStoreKey = buf.String()
	return nil
}

// containsTemplate 检查字符串是否包含模板语法
func containsTemplate(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}
