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

// matchFilter 匹配过滤条件（简化版，后续可集成 CEL）
func (m *Matcher) matchFilter(filter string, event *listener.Event) bool {
	// 简化版过滤：目前只支持简单的字段比较
	// TODO: 集成 CEL 表达式引擎

	// 临时实现：如果 filter 为 "true"，总是匹配
	if filter == "true" {
		return true
	}

	// 简单的字段存在性检查
	// 例如: "NewRow['status']" 检查 status 字段是否存在
	if strings.Contains(filter, "NewRow['") {
		// 提取字段名
		start := strings.Index(filter, "NewRow['")
		if start != -1 {
			start += len("NewRow['")
			end := strings.Index(filter[start:], "']")
			if end != -1 {
				fieldName := filter[start : start+end]
				val := event.GetField(fieldName)
				return val != nil
			}
		}
	}

	// 默认返回 true，避免过于严格的过滤导致规则不匹配
	// 后续集成 CEL 后会有完整的表达式评估
	return true
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
