package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// EventsTotal 总事件数
	EventsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bingo_events_total",
			Help: "总事件数",
		},
		[]string{"table", "action"},
	)

	// RulesMatchedTotal 规则匹配数
	RulesMatchedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bingo_rules_matched_total",
			Help: "规则匹配数",
		},
		[]string{"rule_id", "table", "action"},
	)

	// ActionsExecutedTotal 动作执行数
	ActionsExecutedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bingo_actions_executed_total",
			Help: "动作执行数",
		},
		[]string{"action_type", "rule_id"},
	)

	// ActionsFailedTotal 动作失败数
	ActionsFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bingo_actions_failed_total",
			Help: "动作失败数",
		},
		[]string{"action_type", "rule_id", "error_type"},
	)

	// ProcessingDuration 处理耗时（秒）
	ProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bingo_processing_duration_seconds",
			Help:    "处理耗时（秒）",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"rule_id", "action_type"},
	)
)

// RecordEvent 记录事件
func RecordEvent(table, action string) {
	EventsTotal.WithLabelValues(table, action).Inc()
}

// RecordRuleMatched 记录规则匹配
func RecordRuleMatched(ruleID, table, action string) {
	RulesMatchedTotal.WithLabelValues(ruleID, table, action).Inc()
}

// RecordActionExecuted 记录动作执行
func RecordActionExecuted(actionType, ruleID string) {
	ActionsExecutedTotal.WithLabelValues(actionType, ruleID).Inc()
}

// RecordActionFailed 记录动作失败
func RecordActionFailed(actionType, ruleID, errorType string) {
	ActionsFailedTotal.WithLabelValues(actionType, ruleID, errorType).Inc()
}

// RecordProcessingDuration 记录处理耗时
func RecordProcessingDuration(ruleID, actionType string, duration float64) {
	ProcessingDuration.WithLabelValues(ruleID, actionType).Observe(duration)
}
