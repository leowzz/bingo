package engine

import (
	"fmt"
	"os"
	"sort"

	"github.com/google/cel-go/cel"
	"gopkg.in/yaml.v3"
)

// 事件类型位标志常量
const (
	EventBitInsert uint8 = 1 << iota // 1
	EventBitUpdate                   // 2
	EventBitDelete                   // 4
)

// Rule 规则定义
type Rule struct {
	ID       string       `yaml:"id"`
	Name     string       `yaml:"name"`
	Database string       `yaml:"database"` // 数据库名（必填）
	Table    string       `yaml:"table"`    // 表名（必填）
	Events   []string     `yaml:"events"`
	Filter   string       `yaml:"filter,omitempty"`
	Actions  []Action     `yaml:"actions"`
	Batch    *BatchConfig `yaml:"batch,omitempty"`

	// 预处理字段（运行时优化使用，不序列化）
	FullTableName    string      `yaml:"-"` // 预计算的 database.table
	InterestedEvents uint8       `yaml:"-"` // 位标志：INSERT=1, UPDATE=2, DELETE=4
	CompiledFilter   cel.Program `yaml:"-"` // 缓存的CEL程序（简单表达式预编译）
	FilterCacheKey   string      `yaml:"-"` // 用于缓存查找的key
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
		// 预处理规则（计算优化字段）
		if err := preprocessRule(&config.Rules[i]); err != nil {
			return nil, fmt.Errorf("规则 %s 预处理失败: %w", config.Rules[i].ID, err)
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
		// 预处理规则（计算优化字段）
		if err := preprocessRule(&config.Rules[i]); err != nil {
			return nil, fmt.Errorf("规则 %s 预处理失败: %w", config.Rules[i].ID, err)
		}
	}

	return &config, nil
}

// validateRule 验证规则
func validateRule(rule *Rule) error {
	if rule.ID == "" {
		return fmt.Errorf("规则 ID 不能为空")
	}
	if rule.Database == "" {
		return fmt.Errorf("规则数据库名不能为空")
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

// preprocessRule 预处理规则，计算优化字段
//
// 计算 FullTableName、InterestedEvents 等字段，用于性能优化。
// 如果表达式简单，也会预编译 CEL 程序。
func preprocessRule(rule *Rule) error {
	// 计算完整表名
	rule.FullTableName = fmt.Sprintf("%s.%s", rule.Database, rule.Table)

	// 计算事件类型位标志
	rule.InterestedEvents = 0
	for _, event := range rule.Events {
		switch event {
		case "INSERT":
			rule.InterestedEvents |= EventBitInsert
		case "UPDATE":
			rule.InterestedEvents |= EventBitUpdate
		case "DELETE":
			rule.InterestedEvents |= EventBitDelete
		}
	}

	// 设置缓存key
	rule.FilterCacheKey = rule.Filter

	// 简单表达式预编译（在规则加载时完成）
	// 复杂表达式将在运行时按需编译并缓存
	if rule.Filter != "" && rule.Filter != "true" {
		if isSimpleFilter(rule.Filter) {
			prg, err := compileFilter(rule.Filter)
			if err == nil {
				rule.CompiledFilter = prg
			}
			// 如果编译失败，将在运行时按需编译
		}
	}

	return nil
}

// isSimpleFilter 判断是否为简单表达式
//
// 简单表达式可以在规则加载时预编译，复杂表达式按需编译。
func isSimpleFilter(filter string) bool {
	// 简单表达式的特征：
	// 1. 长度较短（<100字符）
	// 2. 不包含复杂操作符（如 &&, ||, 嵌套括号等）
	if len(filter) > 100 {
		return false
	}

	// 检查是否包含复杂操作符
	complexOps := []string{"&&", "||", "(", ")", "[", "]"}
	opCount := 0
	for _, op := range complexOps {
		for i := 0; i < len(filter); i++ {
			if i+len(op) <= len(filter) && filter[i:i+len(op)] == op {
				opCount++
			}
		}
	}

	// 如果操作符数量超过阈值，认为是复杂表达式
	return opCount <= 3
}

// compileFilter 编译CEL表达式
//
// 这是一个辅助函数，用于预编译简单表达式。
func compileFilter(filter string) (cel.Program, error) {
	if err := initCELEnv(); err != nil {
		return nil, err
	}

	ast, issues := celEnv.Compile(filter)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("编译 CEL 表达式失败: %w", issues.Err())
	}

	prg, err := celEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("创建 CEL 程序失败: %w", err)
	}

	return prg, nil
}

// ExtractMonitoredTables 从规则列表中提取所有需要监控的表
// 返回去重后的表列表，格式为 []string{"database.table"}
func ExtractMonitoredTables(rules []Rule) []string {
	tableMap := make(map[string]bool)
	for _, rule := range rules {
		tableName := fmt.Sprintf("%s.%s", rule.Database, rule.Table)
		tableMap[tableName] = true
	}

	// 转换为切片并排序
	tables := make([]string, 0, len(tableMap))
	for table := range tableMap {
		tables = append(tables, table)
	}

	// 排序以便输出更有序
	sort.Strings(tables)

	return tables
}
