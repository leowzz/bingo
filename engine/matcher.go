package engine

import (
	"fmt"
	"sync"

	"bingo/listener"

	"github.com/google/cel-go/cel"
)

// Matcher 规则匹配器
type Matcher struct {
	rules []Rule
	index map[string][]*Rule // 表名 -> 规则列表的倒排索引
	mu    sync.RWMutex       // 保护规则列表和索引的并发访问
}

// NewMatcher 创建新的匹配器
func NewMatcher(rules []Rule) *Matcher {
	m := &Matcher{
		rules: rules,
		index: make(map[string][]*Rule),
	}
	m.buildIndex()
	return m
}

// buildIndex 构建倒排索引
//
// 将规则按表名索引，实现 O(1) 查找。
func (m *Matcher) buildIndex() {
	// 预分配map容量，减少重新分配
	index := make(map[string][]*Rule, len(m.rules))

	// 构建索引：表名 -> 规则指针列表
	for i := range m.rules {
		rule := &m.rules[i]
		tableName := rule.FullTableName
		if tableName == "" {
			// 如果FullTableName未设置，回退到计算方式（向后兼容）
			tableName = fmt.Sprintf("%s.%s", rule.Database, rule.Table)
		}
		index[tableName] = append(index[tableName], rule)
	}

	m.index = index
}

// Match 匹配事件，返回匹配的规则列表
//
// 使用两级索引优化：
// Level 1: 通过表名从索引中直接获取相关规则（O(1)）
// Level 2: 仅对命中表名的规则进行事件类型和CEL匹配
func (m *Matcher) Match(event *listener.Event) ([]Rule, error) {
	m.mu.RLock()
	// 从索引中获取该表的所有规则（O(1)查找）
	candidateRules, exists := m.index[event.Table]
	m.mu.RUnlock()

	if !exists || len(candidateRules) == 0 {
		// 没有该表的规则，直接返回
		return nil, nil
	}

	// 预分配匹配结果切片容量
	matched := make([]Rule, 0, len(candidateRules))

	// Level 2: 仅对候选规则进行匹配
	for _, rule := range candidateRules {
		if m.matchRule(rule, event) {
			matched = append(matched, *rule)
		}
	}

	return matched, nil
}

// matchRule 检查规则是否匹配事件
//
// 优化后的匹配流程：
// 1. 表名匹配（已在索引阶段完成，这里可以跳过或快速验证）
// 2. 事件类型位运算预筛选（快速短路）
// 3. CEL表达式匹配（如果存在）
func (m *Matcher) matchRule(rule *Rule, event *listener.Event) bool {
	// 快速验证表名（虽然索引已过滤，但为了安全仍做验证）
	if rule.FullTableName != "" {
		if rule.FullTableName != event.Table {
			return false
		}
	} else {
		// 向后兼容：如果FullTableName未设置，使用旧方法
		if !m.matchTable(rule.Database, rule.Table, event.Table) {
			return false
		}
	}

	// 事件类型位运算预筛选（快速短路，避免进入CEL）
	eventBit := m.getEventBit(event.Action)
	if (eventBit & rule.InterestedEvents) == 0 {
		return false
	}

	// 检查过滤条件（CEL表达式）
	if rule.Filter != "" {
		if !m.matchFilterOptimized(rule, event) {
			return false
		}
	}

	return true
}

// getEventBit 获取事件类型的位标志
func (m *Matcher) getEventBit(action listener.Action) uint8 {
	switch action {
	case listener.ActionInsert:
		return EventBitInsert
	case listener.ActionUpdate:
		return EventBitUpdate
	case listener.ActionDelete:
		return EventBitDelete
	default:
		return 0
	}
}

// matchTable 匹配表名（向后兼容方法）
// ruleDatabase: 规则中配置的数据库名
// ruleTable: 规则中配置的表名
// eventTable: 事件中的表名，格式为 "database.table"
func (m *Matcher) matchTable(ruleDatabase, ruleTable, eventTable string) bool {
	// 使用预计算的FullTableName（如果可用）
	// 否则直接比较，避免fmt.Sprintf分配
	expectedTable := fmt.Sprintf("%s.%s", ruleDatabase, ruleTable)
	return expectedTable == eventTable
}

// matchFilterOptimized 优化的过滤条件匹配（使用 CEL 表达式引擎）
//
// 优先使用预编译的CEL程序，如果不存在则使用缓存或按需编译。
func (m *Matcher) matchFilterOptimized(rule *Rule, event *listener.Event) bool {
	// 如果规则已有预编译的CEL程序，直接使用
	if rule.CompiledFilter != nil {
		return m.evalCompiledFilter(rule.CompiledFilter, event)
	}

	// 否则使用缓存或按需编译（在EvaluateFilterWithCache中处理）
	result, err := EvaluateFilterWithCache(rule.FilterCacheKey, event)
	if err != nil {
		// 如果 CEL 评估失败，记录错误但返回 false（安全起见，不匹配规则）
		// 注意：这里需要导入 logger，但为了避免循环依赖，暂时不记录日志
		// 在实际使用中，错误会在上层被记录
		return false
	}
	return result
}

// evalCompiledFilter 执行预编译的CEL程序
func (m *Matcher) evalCompiledFilter(prg cel.Program, event *listener.Event) bool {
	// 准备变量（使用对象池）
	vars := getVarsFromPool()
	defer putVarsToPool(vars)

	vars["Table"] = event.Table
	vars["Action"] = string(event.Action)
	vars["Schema"] = event.Schema
	vars["TableName"] = event.TableName
	vars["NewRow"] = event.NewRow
	vars["OldRow"] = event.OldRow

	// 执行表达式
	out, _, err := prg.Eval(vars)
	if err != nil {
		return false
	}

	// 转换为布尔值
	result, ok := out.Value().(bool)
	if !ok {
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
// 更新后会重建索引。
func (m *Matcher) UpdateRules(rules []Rule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules = rules
	m.buildIndex()
}
