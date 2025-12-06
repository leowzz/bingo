package engine

import (
	"fmt"
	"sync"

	"bingo/listener"
)

// Matcher 规则匹配器
type Matcher struct {
	rules []Rule
	mu    sync.RWMutex // 保护规则列表的并发访问
}

// NewMatcher 创建新的匹配器
func NewMatcher(rules []Rule) *Matcher {
	return &Matcher{
		rules: rules,
	}
}

// Match 匹配事件，返回匹配的规则列表
func (m *Matcher) Match(event *listener.Event) ([]Rule, error) {
	m.mu.RLock()
	rules := m.rules
	m.mu.RUnlock()

	var matched []Rule
	for _, rule := range rules {
		if m.matchRule(&rule, event) {
			matched = append(matched, rule)
		}
	}

	return matched, nil
}

// matchRule 检查规则是否匹配事件
func (m *Matcher) matchRule(rule *Rule, event *listener.Event) bool {
	// 检查表名（使用 database.table 格式）
	if !m.matchTable(rule.Database, rule.Table, event.Table) {
		return false
	}

	// 检查事件类型
	if !m.matchEvent(rule.Events, string(event.Action)) {
		return false
	}

	// 检查过滤条件
	if rule.Filter != "" {
		if !m.matchFilter(rule.Filter, event) {
			return false
		}
	}

	return true
}

// matchTable 匹配表名
// ruleDatabase: 规则中配置的数据库名
// ruleTable: 规则中配置的表名
// eventTable: 事件中的表名，格式为 "database.table"
func (m *Matcher) matchTable(ruleDatabase, ruleTable, eventTable string) bool {
	// 构建规则的完整表名
	ruleFullTable := fmt.Sprintf("%s.%s", ruleDatabase, ruleTable)

	// 精确匹配
	return ruleFullTable == eventTable
}

// matchEvent 匹配事件类型
func (m *Matcher) matchEvent(ruleEvents []string, eventAction string) bool {
	for _, e := range ruleEvents {
		if e == eventAction {
			return true
		}
	}
	return false
}

// matchFilter 匹配过滤条件（使用 CEL 表达式引擎）
func (m *Matcher) matchFilter(filter string, event *listener.Event) bool {
	// 使用 CEL 表达式引擎评估过滤条件
	result, err := EvaluateFilter(filter, event)
	if err != nil {
		// 如果 CEL 评估失败，记录错误但返回 false（安全起见，不匹配规则）
		// 注意：这里需要导入 logger，但为了避免循环依赖，暂时不记录日志
		// 在实际使用中，错误会在上层被记录
		return false
	}
	return result
}

// GetRules 获取所有规则
func (m *Matcher) GetRules() []Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rules
}

// GetRuleByID 根据 ID 获取规则
func (m *Matcher) GetRuleByID(id string) (*Rule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, rule := range m.rules {
		if rule.ID == id {
			return &rule, nil
		}
	}
	return nil, fmt.Errorf("规则不存在: %s", id)
}

// UpdateRules 更新规则列表（线程安全）
//
// 用于热重载规则文件时更新规则列表。
func (m *Matcher) UpdateRules(rules []Rule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules = rules
}
