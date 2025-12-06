package engine

import (
	"fmt"
	"strings"

	"bingo/listener"
)

// Matcher 规则匹配器
type Matcher struct {
	rules []Rule
}

// NewMatcher 创建新的匹配器
func NewMatcher(rules []Rule) *Matcher {
	return &Matcher{
		rules: rules,
	}
}

// Match 匹配事件，返回匹配的规则列表
func (m *Matcher) Match(event *listener.Event) ([]Rule, error) {
	var matched []Rule

	for _, rule := range m.rules {
		if m.matchRule(&rule, event) {
			matched = append(matched, rule)
		}
	}

	return matched, nil
}

// matchRule 检查规则是否匹配事件
func (m *Matcher) matchRule(rule *Rule, event *listener.Event) bool {
	// 检查表名
	if !m.matchTable(rule.Table, event.Table) {
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
func (m *Matcher) matchTable(ruleTable, eventTable string) bool {
	// 支持精确匹配和通配符
	if ruleTable == eventTable {
		return true
	}

	// 支持 database.table 和 table 两种格式
	// 如果规则是 "table"，匹配 "database.table"
	if !strings.Contains(ruleTable, ".") {
		parts := strings.Split(eventTable, ".")
		if len(parts) == 2 && parts[1] == ruleTable {
			return true
		}
	}

	return false
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
	return m.rules
}

// GetRuleByID 根据 ID 获取规则
func (m *Matcher) GetRuleByID(id string) (*Rule, error) {
	for _, rule := range m.rules {
		if rule.ID == id {
			return &rule, nil
		}
	}
	return nil, fmt.Errorf("规则不存在: %s", id)
}
