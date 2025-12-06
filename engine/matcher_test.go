package engine

import (
	"testing"

	"bingo/listener"
)

func TestMatcher_Match(t *testing.T) {
	rules := []Rule{
		{
			ID:       "rule1",
			Name:     "用户表规则",
			Database: "test",
			Table:    "users",
			Events:   []string{"INSERT", "UPDATE"},
			Filter:   "true",
			Actions: []Action{
				{Type: "log", Message: "User event"},
			},
		},
		{
			ID:       "rule2",
			Name:     "订单表规则",
			Database: "test",
			Table:    "orders",
			Events:   []string{"UPDATE"},
			Filter:   "true",
			Actions: []Action{
				{Type: "log", Message: "Order event"},
			},
		},
		{
			ID:       "rule3",
			Name:     "用户删除规则",
			Database: "test",
			Table:    "users",
			Events:   []string{"DELETE"},
			Filter:   "true",
			Actions: []Action{
				{Type: "log", Message: "User deleted"},
			},
		},
	}

	matcher := NewMatcher(rules)

	tests := []struct {
		name            string
		event           *listener.Event
		expectedRuleIDs []string
	}{
		{
			name: "匹配用户 INSERT 规则",
			event: &listener.Event{
				Table:  "test.users",
				Action: listener.ActionInsert,
			},
			expectedRuleIDs: []string{"rule1"},
		},
		{
			name: "匹配用户 UPDATE 规则",
			event: &listener.Event{
				Table:  "test.users",
				Action: listener.ActionUpdate,
			},
			expectedRuleIDs: []string{"rule1"},
		},
		{
			name: "匹配用户 DELETE 规则",
			event: &listener.Event{
				Table:  "test.users",
				Action: listener.ActionDelete,
			},
			expectedRuleIDs: []string{"rule3"},
		},
		{
			name: "匹配订单 UPDATE 规则",
			event: &listener.Event{
				Table:  "test.orders",
				Action: listener.ActionUpdate,
			},
			expectedRuleIDs: []string{"rule2"},
		},
		{
			name: "不匹配任何规则",
			event: &listener.Event{
				Table:  "test.products",
				Action: listener.ActionInsert,
			},
			expectedRuleIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, err := matcher.Match(tt.event)
			if err != nil {
				t.Fatalf("Match() error = %v", err)
			}

			if len(matched) != len(tt.expectedRuleIDs) {
				t.Errorf("Match() returned %d rules, want %d", len(matched), len(tt.expectedRuleIDs))
			}

			matchedIDs := make(map[string]bool)
			for _, rule := range matched {
				matchedIDs[rule.ID] = true
			}

			for _, expectedID := range tt.expectedRuleIDs {
				if !matchedIDs[expectedID] {
					t.Errorf("Match() missing rule ID: %s", expectedID)
				}
			}
		})
	}
}

func TestMatcher_matchTable(t *testing.T) {
	matcher := NewMatcher([]Rule{})

	tests := []struct {
		name         string
		ruleDatabase string
		ruleTable    string
		eventTable   string
		expected     bool
	}{
		{
			name:         "精确匹配",
			ruleDatabase: "test",
			ruleTable:    "users",
			eventTable:   "test.users",
			expected:     true,
		},
		{
			name:         "数据库名不匹配",
			ruleDatabase: "prod",
			ruleTable:    "users",
			eventTable:   "test.users",
			expected:     false,
		},
		{
			name:         "表名不匹配",
			ruleDatabase: "test",
			ruleTable:    "users",
			eventTable:   "test.orders",
			expected:     false,
		},
		{
			name:         "完全不匹配",
			ruleDatabase: "prod",
			ruleTable:    "orders",
			eventTable:   "test.users",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.matchTable(tt.ruleDatabase, tt.ruleTable, tt.eventTable)
			if result != tt.expected {
				t.Errorf("matchTable() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMatcher_matchEvent(t *testing.T) {
	matcher := NewMatcher([]Rule{})

	tests := []struct {
		name        string
		ruleEvents  []string
		eventAction string
		expected    bool
	}{
		{
			name:        "匹配 INSERT",
			ruleEvents:  []string{"INSERT", "UPDATE"},
			eventAction: "INSERT",
			expected:    true,
		},
		{
			name:        "匹配 UPDATE",
			ruleEvents:  []string{"INSERT", "UPDATE"},
			eventAction: "UPDATE",
			expected:    true,
		},
		{
			name:        "不匹配 DELETE",
			ruleEvents:  []string{"INSERT", "UPDATE"},
			eventAction: "DELETE",
			expected:    false,
		},
		{
			name:        "空规则列表",
			ruleEvents:  []string{},
			eventAction: "INSERT",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.matchEvent(tt.ruleEvents, tt.eventAction)
			if result != tt.expected {
				t.Errorf("matchEvent() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMatcher_GetRuleByID(t *testing.T) {
	rules := []Rule{
		{ID: "rule1", Name: "Rule 1", Database: "test", Table: "test_table", Events: []string{"INSERT"}, Actions: []Action{{Type: "log"}}},
		{ID: "rule2", Name: "Rule 2", Database: "test", Table: "test_table", Events: []string{"INSERT"}, Actions: []Action{{Type: "log"}}},
	}

	matcher := NewMatcher(rules)

	tests := []struct {
		name      string
		ruleID    string
		wantFound bool
		wantName  string
	}{
		{
			name:      "找到规则",
			ruleID:    "rule1",
			wantFound: true,
			wantName:  "Rule 1",
		},
		{
			name:      "找不到规则",
			ruleID:    "rule3",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := matcher.GetRuleByID(tt.ruleID)
			if tt.wantFound {
				if err != nil {
					t.Fatalf("GetRuleByID() error = %v", err)
				}
				if rule.Name != tt.wantName {
					t.Errorf("GetRuleByID() Name = %v, want %v", rule.Name, tt.wantName)
				}
			} else {
				if err == nil {
					t.Error("GetRuleByID() expected error, got nil")
				}
			}
		})
	}
}
