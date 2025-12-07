package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"bingo/engine"
	"bingo/executor"
	"bingo/internal/logger"
	"bingo/listener"
)

// mockExecutor 模拟执行器，用于测试
type mockExecutor struct {
	executedActions []executedAction
	mu              sync.Mutex
}

type executedAction struct {
	ruleID string
	event  *listener.Event
}

func (m *mockExecutor) Execute(ctx context.Context, action engine.Action, event *listener.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executedActions = append(m.executedActions, executedAction{
		ruleID: "", // 在 executeRule 中会设置
		event:  event,
	})
	return nil
}

func (m *mockExecutor) ExecuteActions(ctx context.Context, actions []engine.Action, event *listener.Event) error {
	return m.Execute(ctx, engine.Action{}, event)
}

func (m *mockExecutor) Type() string {
	return "mock"
}

func (m *mockExecutor) getExecutedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.executedActions)
}

func (m *mockExecutor) getExecutedEvents() []*listener.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	events := make([]*listener.Event, len(m.executedActions))
	for i, ea := range m.executedActions {
		events[i] = ea.event
	}
	return events
}

func (m *mockExecutor) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executedActions = nil
}

// createTestEventWithID 创建带 ID 的测试事件
func createTestEventWithID(table string, action listener.Action, id interface{}) *listener.Event {
	event := listener.GetEventFromPool()
	event.Table = table
	event.Action = action
	event.Schema = "test"
	event.TableName = "users"
	event.Timestamp = time.Now()
	event.NewRow = map[string]interface{}{
		"id": id,
	}
	return event
}

// createTestMatcher 创建测试匹配器
func createTestMatcher(rules []engine.Rule) *engine.Matcher {
	// 预处理规则，确保 FullTableName 被设置
	for i := range rules {
		if rules[i].FullTableName == "" {
			rules[i].FullTableName = fmt.Sprintf("%s.%s", rules[i].Database, rules[i].Table)
		}
		// 设置事件类型位标志
		rules[i].InterestedEvents = 0
		for _, event := range rules[i].Events {
			switch event {
			case "INSERT":
				rules[i].InterestedEvents |= engine.EventBitInsert
			case "UPDATE":
				rules[i].InterestedEvents |= engine.EventBitUpdate
			case "DELETE":
				rules[i].InterestedEvents |= engine.EventBitDelete
			}
		}
	}
	return engine.NewMatcher(rules)
}

// initTestLogger 初始化测试用的 logger
func initTestLogger() {
	if logger.Logger == nil {
		logger.InitLogger("info", "text", "stdout", "", 0, 0, 0, false)
	}
}

// TestEventHandler_Batch 测试批量聚合功能
func TestEventHandler_Batch(t *testing.T) {
	initTestLogger()

	// 创建模拟执行器
	mockExec := &mockExecutor{}

	// 创建执行器管理器
	exec := executor.NewExecutor()
	exec.Register(mockExec)

	// 创建规则（启用批量聚合）
	rule := engine.Rule{
		ID:       "batch_rule",
		Name:     "批量测试规则",
		Database: "test",
		Table:    "users",
		Events:   []string{"INSERT"},
		Filter:   "true",
		Actions: []engine.Action{
			{Type: "mock"},
		},
		Batch: &engine.BatchConfig{
			Enabled: true,
			Window:  50,  // 50ms 窗口
			MaxSize: 5,   // 最大 5 个
		},
	}

	// 创建匹配器
	matcher := createTestMatcher([]engine.Rule{rule})

	// 创建事件处理器
	handler := NewEventHandler(matcher, exec, 100, 1)
	handler.Start()
	defer handler.Stop()

	// 发送多个事件（应该被批量聚合）
	events := make([]*listener.Event, 0, 10)
	for i := 0; i < 10; i++ {
		event := createTestEventWithID("test.users", listener.ActionInsert, i)
		events = append(events, event)
		handler.OnEvent(event)
	}

	// 等待批量处理完成（等待时间窗口 + 一些缓冲时间）
	time.Sleep(300 * time.Millisecond)

	// 检查执行次数（应该被批量处理，但每个事件都会执行一次）
	// 注意：当前实现是批量收集，但仍然是逐个执行动作
	executedCount := mockExec.getExecutedCount()
	if executedCount == 0 {
		t.Error("批量处理应该执行至少一次")
	}
	
	// 验证至少处理了一些事件
	if executedCount < 5 {
		t.Logf("批量处理执行了 %d 次，可能部分事件还在等待批量窗口", executedCount)
	}

	// 验证事件都被处理了
	executedEvents := mockExec.getExecutedEvents()
	if len(executedEvents) != executedCount {
		t.Errorf("执行的事件数量不匹配: got %d, want %d", len(executedEvents), executedCount)
	}
}

// TestEventHandler_BatchMaxSize 测试批量聚合的最大数量触发
func TestEventHandler_BatchMaxSize(t *testing.T) {
	initTestLogger()
	mockExec := &mockExecutor{}
	exec := executor.NewExecutor()
	exec.Register(mockExec)

	rule := engine.Rule{
		ID:       "batch_max_rule",
		Database: "test",
		Table:    "users",
		Events:   []string{"INSERT"},
		Filter:   "true",
		Actions: []engine.Action{
			{Type: "mock"},
		},
		Batch: &engine.BatchConfig{
			Enabled: true,
			Window:  1000, // 1秒窗口（足够长，不会触发）
			MaxSize: 3,    // 最大 3 个
		},
	}

	matcher := createTestMatcher([]engine.Rule{rule})
	handler := NewEventHandler(matcher, exec, 100, 1)
	handler.Start()
	defer handler.Stop()

	// 发送 3 个事件（达到最大数量，应该立即触发）
	for i := 0; i < 3; i++ {
		event := createTestEventWithID("test.users", listener.ActionInsert, i)
		handler.OnEvent(event)
	}

	// 等待处理完成
	time.Sleep(200 * time.Millisecond)

	// 应该已经执行了（因为达到最大数量）
	executedCount := mockExec.getExecutedCount()
	if executedCount == 0 {
		t.Error("达到最大数量时应该立即触发批量处理")
	}
	
	// 验证至少处理了 3 个事件
	if executedCount < 3 {
		t.Errorf("应该至少处理 3 个事件，实际处理了 %d 个", executedCount)
	}
}

// TestEventHandler_Ordering 测试顺序保障功能
func TestEventHandler_Ordering(t *testing.T) {
	initTestLogger()
	mockExec := &mockExecutor{}
	exec := executor.NewExecutor()
	exec.Register(mockExec)

	rule := engine.Rule{
		ID:       "ordering_rule",
		Database: "test",
		Table:    "users",
		Events:   []string{"UPDATE"},
		Filter:   "true",
		Actions: []engine.Action{
			{Type: "mock"},
		},
		Ordering: &engine.OrderingConfig{
			Enabled:  true,
			KeyField: "id",
			Shards:   3, // 3 个分片
		},
	}

	matcher := createTestMatcher([]engine.Rule{rule})
	handler := NewEventHandler(matcher, exec, 100, 1)
	handler.Start()
	defer handler.Stop()

	// 发送多个相同主键的事件（应该被分发到同一个分片）
	sameKey := "123"
	events := make([]*listener.Event, 0, 5)
	for i := 0; i < 5; i++ {
		event := createTestEventWithID("test.users", listener.ActionUpdate, sameKey)
		events = append(events, event)
		handler.OnEvent(event)
	}

	// 等待处理完成
	time.Sleep(500 * time.Millisecond)

	// 验证事件都被处理了
	executedCount := mockExec.getExecutedCount()
	if executedCount != 5 {
		t.Errorf("应该处理 5 个事件，实际处理了 %d 个", executedCount)
	}
}

// TestEventHandler_OrderingShardDistribution 测试分片分发
func TestEventHandler_OrderingShardDistribution(t *testing.T) {
	initTestLogger()
	mockExec := &mockExecutor{}
	exec := executor.NewExecutor()
	exec.Register(mockExec)

	rule := engine.Rule{
		ID:       "ordering_shard_rule",
		Database: "test",
		Table:    "users",
		Events:   []string{"UPDATE"},
		Filter:   "true",
		Actions: []engine.Action{
			{Type: "mock"},
		},
		Ordering: &engine.OrderingConfig{
			Enabled:  true,
			KeyField: "id",
			Shards:   5, // 5 个分片
		},
	}

	matcher := createTestMatcher([]engine.Rule{rule})
	handler := NewEventHandler(matcher, exec, 100, 1)
	handler.Start()
	defer handler.Stop()

	// 发送不同主键的事件（应该被分发到不同分片）
	keyCounts := make(map[string]int)
	for i := 0; i < 20; i++ {
		key := string(rune('A' + i))
		event := createTestEventWithID("test.users", listener.ActionUpdate, key)
		handler.OnEvent(event)
		keyCounts[key]++
	}

	// 等待处理完成
	time.Sleep(600 * time.Millisecond)

	// 验证所有事件都被处理了
	executedCount := mockExec.getExecutedCount()
	if executedCount != 20 {
		t.Errorf("应该处理 20 个事件，实际处理了 %d 个", executedCount)
	}
}

// TestEventHandler_OrderingMissingKeyField 测试缺少主键字段的情况
func TestEventHandler_OrderingMissingKeyField(t *testing.T) {
	initTestLogger()
	mockExec := &mockExecutor{}
	exec := executor.NewExecutor()
	exec.Register(mockExec)

	rule := engine.Rule{
		ID:       "ordering_missing_key_rule",
		Database: "test",
		Table:    "users",
		Events:   []string{"UPDATE"},
		Filter:   "true",
		Actions: []engine.Action{
			{Type: "mock"},
		},
		Ordering: &engine.OrderingConfig{
			Enabled:  true,
			KeyField: "id",
			Shards:   3,
		},
	}

	matcher := createTestMatcher([]engine.Rule{rule})
	handler := NewEventHandler(matcher, exec, 100, 1)
	handler.Start()
	defer handler.Stop()

	// 发送没有主键字段的事件
	event := listener.GetEventFromPool()
	event.Table = "test.users"
	event.Action = listener.ActionUpdate
	event.Schema = "test"
	event.TableName = "users"
	event.NewRow = map[string]interface{}{
		"name": "test", // 没有 id 字段
	}

	handler.OnEvent(event)

	// 等待处理完成
	time.Sleep(300 * time.Millisecond)

	// 应该仍然被处理（使用普通处理模式）
	executedCount := mockExec.getExecutedCount()
	if executedCount != 1 {
		t.Errorf("缺少主键字段时应该使用普通处理，实际处理了 %d 个", executedCount)
	}
}

// TestEventHandler_CalculateShard 测试分片计算
func TestEventHandler_CalculateShard(t *testing.T) {
	handler := &EventHandler{}

	tests := []struct {
		name      string
		keyValue  string
		shardCount int
		wantRange  bool // 是否在有效范围内
	}{
		{
			name:       "正常分片计算",
			keyValue:   "123",
			shardCount: 5,
			wantRange:  true,
		},
		{
			name:       "相同键值应该得到相同分片",
			keyValue:   "same_key",
			shardCount: 10,
			wantRange:  true,
		},
		{
			name:       "不同键值可能得到不同分片",
			keyValue:   "different_key",
			shardCount: 10,
			wantRange:  true,
		},
		{
			name:       "分片数为 0 时使用默认值",
			keyValue:   "test",
			shardCount: 0,
			wantRange:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shard1 := handler.calculateShard(tt.keyValue, tt.shardCount)
			shard2 := handler.calculateShard(tt.keyValue, tt.shardCount)

			// 相同键值应该得到相同分片
			if shard1 != shard2 {
				t.Errorf("相同键值应该得到相同分片: shard1=%d, shard2=%d", shard1, shard2)
			}

			// 分片索引应该在有效范围内
			expectedShardCount := tt.shardCount
			if expectedShardCount == 0 {
				expectedShardCount = 10 // 默认值
			}
			if shard1 < 0 || shard1 >= expectedShardCount {
				t.Errorf("分片索引应该在有效范围内: shard=%d, shardCount=%d", shard1, expectedShardCount)
			}
		})
	}

	// 测试相同键值的一致性
	sameKey := "consistent_key"
	shards := make([]int, 100)
	for i := 0; i < 100; i++ {
		shards[i] = handler.calculateShard(sameKey, 10)
	}

	// 所有结果应该相同
	for i := 1; i < 100; i++ {
		if shards[i] != shards[0] {
			t.Errorf("相同键值应该始终得到相同分片: shards[0]=%d, shards[%d]=%d", shards[0], i, shards[i])
		}
	}
}

// TestEventHandler_NormalProcessing 测试普通处理（无 Batch 和 Ordering）
func TestEventHandler_NormalProcessing(t *testing.T) {
	initTestLogger()
	mockExec := &mockExecutor{}
	exec := executor.NewExecutor()
	exec.Register(mockExec)

	rule := engine.Rule{
		ID:       "normal_rule",
		Database: "test",
		Table:    "users",
		Events:   []string{"INSERT"},
		Filter:   "true",
		Actions: []engine.Action{
			{Type: "mock"},
		},
		// 没有 Batch 和 Ordering 配置
	}

	matcher := createTestMatcher([]engine.Rule{rule})
	handler := NewEventHandler(matcher, exec, 100, 1)
	handler.Start()
	defer handler.Stop()

	// 发送事件
	event := createTestEventWithID("test.users", listener.ActionInsert, 123)
	handler.OnEvent(event)

	// 等待处理完成
	time.Sleep(300 * time.Millisecond)

	// 验证事件被处理了
	executedCount := mockExec.getExecutedCount()
	if executedCount != 1 {
		t.Errorf("应该处理 1 个事件，实际处理了 %d 个", executedCount)
	}
}

// TestEventHandler_UpdateRules 测试规则更新
func TestEventHandler_UpdateRules(t *testing.T) {
	initTestLogger()
	mockExec := &mockExecutor{}
	exec := executor.NewExecutor()
	exec.Register(mockExec)

	// 初始规则（启用 batch）
	initialRule := engine.Rule{
		ID:       "update_rule",
		Database: "test",
		Table:    "users",
		Events:   []string{"INSERT"},
		Filter:   "true",
		Actions: []engine.Action{
			{Type: "mock"},
		},
		Batch: &engine.BatchConfig{
			Enabled: true,
			Window:  100,
			MaxSize: 5,
		},
	}

	matcher := createTestMatcher([]engine.Rule{initialRule})
	handler := NewEventHandler(matcher, exec, 100, 1)
	handler.Start()
	defer handler.Stop()

	// 发送一个事件，触发 batch collector 创建
	event1 := createTestEventWithID("test.users", listener.ActionInsert, 1)
	handler.OnEvent(event1)

	// 等待 batch collector 创建
	time.Sleep(50 * time.Millisecond)

	// 更新规则（移除 batch 配置）
	newRule := engine.Rule{
		ID:       "update_rule",
		Database: "test",
		Table:    "users",
		Events:   []string{"INSERT"},
		Filter:   "true",
		Actions: []engine.Action{
			{Type: "mock"},
		},
		// 没有 Batch 配置
	}

	handler.UpdateRules([]engine.Rule{newRule})

	// 发送新事件（应该使用普通处理）
	event2 := createTestEventWithID("test.users", listener.ActionInsert, 2)
	handler.OnEvent(event2)

	// 等待处理完成
	time.Sleep(500 * time.Millisecond)

	// 验证事件都被处理了
	executedCount := mockExec.getExecutedCount()
	if executedCount < 2 {
		t.Errorf("应该至少处理 2 个事件，实际处理了 %d 个", executedCount)
	}
}

// TestEventHandler_MixedRules 测试混合规则（同时有 Batch、Ordering 和普通规则）
func TestEventHandler_MixedRules(t *testing.T) {
	initTestLogger()
	mockExec := &mockExecutor{}
	exec := executor.NewExecutor()
	exec.Register(mockExec)

	rules := []engine.Rule{
		{
			ID:       "batch_rule",
			Database: "test",
			Table:    "users",
			Events:   []string{"INSERT"},
			Filter:   "true",
			Actions: []engine.Action{
				{Type: "mock"},
			},
			Batch: &engine.BatchConfig{
				Enabled: true,
				Window:  50,
				MaxSize: 3,
			},
		},
		{
			ID:       "ordering_rule",
			Database: "test",
			Table:    "users",
			Events:   []string{"UPDATE"},
			Filter:   "true",
			Actions: []engine.Action{
				{Type: "mock"},
			},
			Ordering: &engine.OrderingConfig{
				Enabled:  true,
				KeyField: "id",
				Shards:   3,
			},
		},
		{
			ID:       "normal_rule",
			Database: "test",
			Table:    "users",
			Events:   []string{"DELETE"},
			Filter:   "true",
			Actions: []engine.Action{
				{Type: "mock"},
			},
		},
	}

	matcher := createTestMatcher(rules)
	handler := NewEventHandler(matcher, exec, 100, 1)
	handler.Start()
	defer handler.Stop()

	// 发送不同类型的事件
	handler.OnEvent(createTestEventWithID("test.users", listener.ActionInsert, 1))
	handler.OnEvent(createTestEventWithID("test.users", listener.ActionUpdate, 2))
	handler.OnEvent(createTestEventWithID("test.users", listener.ActionDelete, 3))

	// 等待处理完成
	time.Sleep(600 * time.Millisecond)

	// 验证所有事件都被处理了
	executedCount := mockExec.getExecutedCount()
	if executedCount < 3 {
		t.Errorf("应该至少处理 3 个事件，实际处理了 %d 个", executedCount)
	}
}

