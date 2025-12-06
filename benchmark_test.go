package main

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"bingo/engine"
	"bingo/executor"
	"bingo/internal/logger"
	"bingo/listener"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Benchmark 测试说明：
//
// 运行所有 benchmark 测试：
//   go test -bench=. -benchmem -benchtime=3s .
//
// 运行特定测试：
//   go test -bench=BenchmarkMatcher -benchmem -benchtime=3s .
//
// 运行并生成详细报告：
//   go test -bench=. -benchmem -benchtime=3s -cpuprofile=cpu.prof -memprofile=mem.prof .
//
// 性能指标说明：
//   - ns/op: 每次操作耗时（纳秒）
//   - B/op: 每次操作分配的内存（字节）
//   - allocs/op: 每次操作的内存分配次数
//
// 理论最大性能计算：
//   1. 计算单线程最大吞吐量：
//      events/sec = 1,000,000,000 / ns/op
//      例如：ns/op = 45.26ns，则吞吐量 = 1,000,000,000 / 45.26 ≈ 22,094,000 events/sec
//
//   2. 计算多线程并发最大吞吐量：
//      并发吞吐量 = 单线程吞吐量 × CPU核心数 × 并发效率系数
//      并发效率系数通常在 0.7-0.9 之间（考虑锁竞争、上下文切换等）
//
//   3. 估算实际生产环境吞吐量：
//      实际吞吐量 = 理论吞吐量 × 实际负载系数
//      实际负载系数考虑：
//      - 网络I/O延迟（Redis、Webhook等）
//      - 磁盘I/O（日志写入）
//      - 规则复杂度（CEL表达式评估）
//      - 规则数量（匹配开销）
//
// 性能瓶颈分析：
//   1. 规则匹配：规则数量线性影响性能（O(n)）
//   2. CEL表达式：表达式复杂度影响评估时间
//   3. 动作执行：网络I/O是主要瓶颈（Redis、Webhook等）
//   4. 内存分配：频繁分配可能触发GC，影响性能
//
// 优化建议：
//   1. 减少规则数量：合并相似规则，使用更精确的过滤条件
//   2. 优化CEL表达式：简化表达式，避免复杂计算
//   3. 批量处理：使用批量操作减少网络往返
//   4. 连接池：复用连接减少连接开销
//   5. 异步处理：非关键动作异步执行

// setupBenchmarkLogger 设置用于benchmark的logger（输出到io.Discard）
func setupBenchmarkLogger() {
	zapLevel := zapcore.InfoLevel
	encoderConfig := zap.NewProductionEncoderConfig()
	encoder := zapcore.NewJSONEncoder(encoderConfig)
	writeSyncer := zapcore.AddSync(io.Discard)
	core := zapcore.NewCore(encoder, writeSyncer, zapLevel)
	logger.Logger = zap.New(core, zap.AddCallerSkip(1))
	logger.Sugar = logger.Logger.Sugar()
}

// createTestEvent 创建测试事件
func createTestEvent(table string, action listener.Action) *listener.Event {
	return &listener.Event{
		Table:     table,
		Action:    action,
		Timestamp: time.Now(),
		Schema:    "test",
		TableName: "users",
		NewRow: map[string]interface{}{
			"id":         100,
			"name":       "test_user",
			"email":      "test@example.com",
			"status":     "active",
			"age":        25,
			"balance":    1000.50,
			"created_at": time.Now(),
		},
		OldRow: map[string]interface{}{
			"id":      100,
			"name":    "test_user_old",
			"email":   "old@example.com",
			"status":  "inactive",
			"age":     24,
			"balance": 500.25,
		},
	}
}

// createTestRules 创建测试规则
func createTestRules(count int) []engine.Rule {
	rules := make([]engine.Rule, count)
	for i := 0; i < count; i++ {
		rules[i] = engine.Rule{
			ID:       fmt.Sprintf("rule_%d", i),
			Name:     fmt.Sprintf("测试规则 %d", i),
			Database: "test",
			Table:    "users",
			Events:   []string{"INSERT", "UPDATE", "DELETE"},
			Filter:   "true",
			Actions: []engine.Action{
				{Type: "log", Message: "测试消息"},
			},
		}
	}
	return rules
}

// BenchmarkMatcher_Match_NoRules 测试无规则时的匹配性能（基线）
func BenchmarkMatcher_Match_NoRules(b *testing.B) {
	setupBenchmarkLogger()
	matcher := engine.NewMatcher([]engine.Rule{})
	event := createTestEvent("test.users", listener.ActionInsert)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = matcher.Match(event)
		}
	})
}

// BenchmarkMatcher_Match_1Rule 测试1条规则时的匹配性能
func BenchmarkMatcher_Match_1Rule(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(1)
	matcher := engine.NewMatcher(rules)
	event := createTestEvent("test.users", listener.ActionInsert)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = matcher.Match(event)
		}
	})
}

// BenchmarkMatcher_Match_10Rules 测试10条规则时的匹配性能
func BenchmarkMatcher_Match_10Rules(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(10)
	matcher := engine.NewMatcher(rules)
	event := createTestEvent("test.users", listener.ActionInsert)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = matcher.Match(event)
		}
	})
}

// BenchmarkMatcher_Match_100Rules 测试100条规则时的匹配性能
func BenchmarkMatcher_Match_100Rules(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(100)
	matcher := engine.NewMatcher(rules)
	event := createTestEvent("test.users", listener.ActionInsert)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = matcher.Match(event)
		}
	})
}

// BenchmarkMatcher_Match_1000Rules 测试1000条规则时的匹配性能
func BenchmarkMatcher_Match_1000Rules(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(1000)
	matcher := engine.NewMatcher(rules)
	event := createTestEvent("test.users", listener.ActionInsert)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = matcher.Match(event)
		}
	})
}

// BenchmarkCEL_SimpleFilter 测试简单CEL表达式性能
func BenchmarkCEL_SimpleFilter(b *testing.B) {
	setupBenchmarkLogger()
	event := createTestEvent("test.users", listener.ActionInsert)
	filter := "Action == 'INSERT'"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = engine.EvaluateFilter(filter, event)
		}
	})
}

// BenchmarkCEL_ComplexFilter 测试复杂CEL表达式性能
func BenchmarkCEL_ComplexFilter(b *testing.B) {
	setupBenchmarkLogger()
	event := createTestEvent("test.users", listener.ActionUpdate)
	filter := "Action == 'UPDATE' && NewRow['status'] == 'active' && OldRow['status'] != 'active' && NewRow['balance'] > 100.0"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = engine.EvaluateFilter(filter, event)
		}
	})
}

// BenchmarkCEL_VeryComplexFilter 测试非常复杂的CEL表达式性能
func BenchmarkCEL_VeryComplexFilter(b *testing.B) {
	setupBenchmarkLogger()
	event := createTestEvent("test.users", listener.ActionUpdate)
	filter := "(Action == 'UPDATE' || Action == 'INSERT') && " +
		"NewRow['status'] == 'active' && " +
		"OldRow['status'] != 'active' && " +
		"NewRow['balance'] > 100.0 && " +
		"NewRow['age'] >= 18 && " +
		"NewRow['age'] <= 100"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = engine.EvaluateFilter(filter, event)
		}
	})
}

// BenchmarkExecutor_LogAction 测试日志执行器性能
func BenchmarkExecutor_LogAction(b *testing.B) {
	setupBenchmarkLogger()
	exec := executor.NewExecutor()
	event := createTestEvent("test.users", listener.ActionInsert)
	action := engine.Action{
		Type:    "log",
		Message: "测试消息: {{.Table}} - {{.Action}}",
		Level:   "info",
	}
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = exec.Execute(ctx, action, event)
		}
	})
}

// BenchmarkEventProcessing_EndToEnd 测试端到端事件处理性能（匹配+执行）
func BenchmarkEventProcessing_EndToEnd(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(10)
	matcher := engine.NewMatcher(rules)
	exec := executor.NewExecutor()
	event := createTestEvent("test.users", listener.ActionInsert)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			matchedRules, err := matcher.Match(event)
			if err != nil {
				b.Fatal(err)
			}
			for _, rule := range matchedRules {
				_ = exec.ExecuteActions(ctx, rule.Actions, event)
			}
		}
	})
}

// BenchmarkEventProcessing_EndToEnd_WithFilter 测试带过滤条件的端到端性能
func BenchmarkEventProcessing_EndToEnd_WithFilter(b *testing.B) {
	setupBenchmarkLogger()
	rules := []engine.Rule{
		{
			ID:       "rule1",
			Database: "test",
			Table:    "users",
			Events:   []string{"INSERT", "UPDATE"},
			Filter:   "NewRow['status'] == 'active' && NewRow['balance'] > 100.0",
			Actions: []engine.Action{
				{Type: "log", Message: "活跃用户事件"},
			},
		},
	}
	matcher := engine.NewMatcher(rules)
	exec := executor.NewExecutor()
	event := createTestEvent("test.users", listener.ActionUpdate)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			matchedRules, err := matcher.Match(event)
			if err != nil {
				b.Fatal(err)
			}
			for _, rule := range matchedRules {
				_ = exec.ExecuteActions(ctx, rule.Actions, event)
			}
		}
	})
}

// BenchmarkConcurrentEventProcessing 测试并发事件处理性能
func BenchmarkConcurrentEventProcessing(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(10)
	matcher := engine.NewMatcher(rules)
	exec := executor.NewExecutor()
	ctx := context.Background()

	// 模拟工作池
	workerCount := 10
	eventQueue := make(chan *listener.Event, 1000)

	// 启动工作线程
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for event := range eventQueue {
				matchedRules, _ := matcher.Match(event)
				for _, rule := range matchedRules {
					_ = exec.ExecuteActions(ctx, rule.Actions, event)
				}
			}
		}()
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			event := createTestEvent("test.users", listener.ActionInsert)
			select {
			case eventQueue <- event:
			default:
				// 队列满时跳过
			}
		}
	})

	close(eventQueue)
	wg.Wait()
}

// BenchmarkMatcher_Match_DifferentTables 测试不同表名匹配的性能
func BenchmarkMatcher_Match_DifferentTables(b *testing.B) {
	setupBenchmarkLogger()
	// 创建针对不同表的规则
	rules := []engine.Rule{
		{ID: "rule1", Database: "test", Table: "users", Events: []string{"INSERT"}, Filter: "true", Actions: []engine.Action{{Type: "log"}}},
		{ID: "rule2", Database: "test", Table: "orders", Events: []string{"INSERT"}, Filter: "true", Actions: []engine.Action{{Type: "log"}}},
		{ID: "rule3", Database: "test", Table: "products", Events: []string{"INSERT"}, Filter: "true", Actions: []engine.Action{{Type: "log"}}},
		{ID: "rule4", Database: "test", Table: "payments", Events: []string{"INSERT"}, Filter: "true", Actions: []engine.Action{{Type: "log"}}},
		{ID: "rule5", Database: "test", Table: "logs", Events: []string{"INSERT"}, Filter: "true", Actions: []engine.Action{{Type: "log"}}},
	}
	matcher := engine.NewMatcher(rules)
	event := createTestEvent("test.users", listener.ActionInsert)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = matcher.Match(event)
		}
	})
}

// BenchmarkMatcher_Match_WithCELFilter 测试带CEL过滤的匹配性能
func BenchmarkMatcher_Match_WithCELFilter(b *testing.B) {
	setupBenchmarkLogger()
	rules := []engine.Rule{
		{
			ID:       "rule1",
			Database: "test",
			Table:    "users",
			Events:   []string{"INSERT", "UPDATE"},
			Filter:   "NewRow['status'] == 'active'",
			Actions:  []engine.Action{{Type: "log", Message: "活跃用户"}},
		},
		{
			ID:       "rule2",
			Database: "test",
			Table:    "users",
			Events:   []string{"UPDATE"},
			Filter:   "NewRow['balance'] > OldRow['balance']",
			Actions:  []engine.Action{{Type: "log", Message: "余额增加"}},
		},
	}
	matcher := engine.NewMatcher(rules)
	event := createTestEvent("test.users", listener.ActionUpdate)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = matcher.Match(event)
		}
	})
}

// BenchmarkEventCreation 测试事件创建性能
func BenchmarkEventCreation(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = createTestEvent("test.users", listener.ActionInsert)
		}
	})
}

// BenchmarkRuleUpdate 测试规则更新性能（模拟热重载）
func BenchmarkRuleUpdate(b *testing.B) {
	setupBenchmarkLogger()
	matcher := engine.NewMatcher(createTestRules(100))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newRules := createTestRules(100)
		matcher.UpdateRules(newRules)
	}
}

// BenchmarkThroughput_Estimation 估算理论最大吞吐量
// 通过测量单次操作耗时，计算理论最大吞吐量
func BenchmarkThroughput_Estimation(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(10)
	matcher := engine.NewMatcher(rules)
	exec := executor.NewExecutor()
	event := createTestEvent("test.users", listener.ActionInsert)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			matchedRules, _ := matcher.Match(event)
			for _, rule := range matchedRules {
				_ = exec.ExecuteActions(ctx, rule.Actions, event)
			}
		}
	})
}

// BenchmarkMatcher_Match_RuleScaling 测试规则数量对性能的影响（子测试）
func BenchmarkMatcher_Match_RuleScaling(b *testing.B) {
	setupBenchmarkLogger()
	ruleCounts := []int{1, 10, 50, 100, 500, 1000}

	for _, count := range ruleCounts {
		b.Run(fmt.Sprintf("Rules_%d", count), func(b *testing.B) {
			rules := createTestRules(count)
			matcher := engine.NewMatcher(rules)
			event := createTestEvent("test.users", listener.ActionInsert)

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = matcher.Match(event)
				}
			})
		})
	}
}

// BenchmarkCEL_FilterComplexity 测试CEL表达式复杂度对性能的影响（子测试）
func BenchmarkCEL_FilterComplexity(b *testing.B) {
	setupBenchmarkLogger()
	event := createTestEvent("test.users", listener.ActionUpdate)

	filters := map[string]string{
		"Simple":  "Action == 'UPDATE'",
		"Medium":  "Action == 'UPDATE' && NewRow['status'] == 'active'",
		"Complex": "Action == 'UPDATE' && NewRow['status'] == 'active' && OldRow['status'] != 'active' && NewRow['balance'] > 100.0",
		"VeryComplex": "(Action == 'UPDATE' || Action == 'INSERT') && " +
			"NewRow['status'] == 'active' && " +
			"OldRow['status'] != 'active' && " +
			"NewRow['balance'] > 100.0 && " +
			"NewRow['age'] >= 18 && " +
			"NewRow['age'] <= 100",
	}

	for name, filter := range filters {
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = engine.EvaluateFilter(filter, event)
				}
			})
		})
	}
}

// BenchmarkEventProcessing_WorkerPoolScaling 测试工作池大小对性能的影响
func BenchmarkEventProcessing_WorkerPoolScaling(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(10)
	matcher := engine.NewMatcher(rules)
	exec := executor.NewExecutor()
	ctx := context.Background()

	workerCounts := []int{1, 5, 10, 20, 50}

	for _, workers := range workerCounts {
		b.Run(fmt.Sprintf("Workers_%d", workers), func(b *testing.B) {
			eventQueue := make(chan *listener.Event, 10000)
			var wg sync.WaitGroup

			// 启动工作线程
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for event := range eventQueue {
						matchedRules, _ := matcher.Match(event)
						for _, rule := range matchedRules {
							_ = exec.ExecuteActions(ctx, rule.Actions, event)
						}
					}
				}()
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					event := createTestEvent("test.users", listener.ActionInsert)
					select {
					case eventQueue <- event:
					default:
						// 队列满时跳过
					}
				}
			})

			close(eventQueue)
			wg.Wait()
		})
	}
}

// BenchmarkEventProcessing_QueueSizeImpact 测试队列大小对性能的影响
func BenchmarkEventProcessing_QueueSizeImpact(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(10)
	matcher := engine.NewMatcher(rules)
	exec := executor.NewExecutor()
	ctx := context.Background()

	queueSizes := []int{100, 1000, 10000, 100000}
	workerCount := 10

	for _, queueSize := range queueSizes {
		b.Run(fmt.Sprintf("QueueSize_%d", queueSize), func(b *testing.B) {
			eventQueue := make(chan *listener.Event, queueSize)
			var wg sync.WaitGroup

			// 启动工作线程
			for i := 0; i < workerCount; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for event := range eventQueue {
						matchedRules, _ := matcher.Match(event)
						for _, rule := range matchedRules {
							_ = exec.ExecuteActions(ctx, rule.Actions, event)
						}
					}
				}()
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					event := createTestEvent("test.users", listener.ActionInsert)
					select {
					case eventQueue <- event:
					default:
						// 队列满时跳过
					}
				}
			})

			close(eventQueue)
			wg.Wait()
		})
	}
}

// BenchmarkEventProcessing_MultipleActions 测试多个动作的执行性能
func BenchmarkEventProcessing_MultipleActions(b *testing.B) {
	setupBenchmarkLogger()
	rules := []engine.Rule{
		{
			ID:       "rule1",
			Database: "test",
			Table:    "users",
			Events:   []string{"INSERT"},
			Filter:   "true",
			Actions: []engine.Action{
				{Type: "log", Message: "动作1"},
				{Type: "log", Message: "动作2"},
				{Type: "log", Message: "动作3"},
				{Type: "log", Message: "动作4"},
				{Type: "log", Message: "动作5"},
			},
		},
	}
	matcher := engine.NewMatcher(rules)
	exec := executor.NewExecutor()
	event := createTestEvent("test.users", listener.ActionInsert)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			matchedRules, _ := matcher.Match(event)
			for _, rule := range matchedRules {
				_ = exec.ExecuteActions(ctx, rule.Actions, event)
			}
		}
	})
}

// BenchmarkEventProcessing_MultipleMatchedRules 测试多个匹配规则的处理性能
func BenchmarkEventProcessing_MultipleMatchedRules(b *testing.B) {
	setupBenchmarkLogger()
	// 创建10条都会匹配的规则
	rules := createTestRules(10)
	matcher := engine.NewMatcher(rules)
	exec := executor.NewExecutor()
	event := createTestEvent("test.users", listener.ActionInsert)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			matchedRules, _ := matcher.Match(event)
			for _, rule := range matchedRules {
				_ = exec.ExecuteActions(ctx, rule.Actions, event)
			}
		}
	})
}

// BenchmarkMemory_EventAllocation 测试事件分配的内存使用
func BenchmarkMemory_EventAllocation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = createTestEvent("test.users", listener.ActionInsert)
		}
	})
}

// BenchmarkMemory_RuleAllocation 测试规则分配的内存使用
func BenchmarkMemory_RuleAllocation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = createTestRules(10)
	}
}

// BenchmarkMatcher_Match_1000Rules_Optimized 测试优化后的1000条规则匹配性能
// 预期性能：从 ~69µs 降至 ~1-5µs（10-50倍提升）
func BenchmarkMatcher_Match_1000Rules_Optimized(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(1000)
	matcher := engine.NewMatcher(rules)
	event := createTestEvent("test.users", listener.ActionInsert)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = matcher.Match(event)
		}
	})
}

// BenchmarkMatcher_Match_Optimized_Comparison 对比优化前后的性能
func BenchmarkMatcher_Match_Optimized_Comparison(b *testing.B) {
	setupBenchmarkLogger()
	ruleCounts := []int{1, 10, 100, 1000}

	for _, count := range ruleCounts {
		b.Run(fmt.Sprintf("Optimized_%dRules", count), func(b *testing.B) {
			rules := createTestRules(count)
			matcher := engine.NewMatcher(rules)
			event := createTestEvent("test.users", listener.ActionInsert)

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = matcher.Match(event)
				}
			})
		})
	}
}

// BenchmarkMatcher_Match_WithIndex 测试索引查找的性能
func BenchmarkMatcher_Match_WithIndex(b *testing.B) {
	setupBenchmarkLogger()
	// 创建针对不同表的规则，测试索引效果
	rules := []engine.Rule{
		{ID: "rule1", Database: "test", Table: "users", Events: []string{"INSERT"}, Filter: "true", Actions: []engine.Action{{Type: "log"}}},
		{ID: "rule2", Database: "test", Table: "orders", Events: []string{"INSERT"}, Filter: "true", Actions: []engine.Action{{Type: "log"}}},
		{ID: "rule3", Database: "test", Table: "products", Events: []string{"INSERT"}, Filter: "true", Actions: []engine.Action{{Type: "log"}}},
		{ID: "rule4", Database: "test", Table: "payments", Events: []string{"INSERT"}, Filter: "true", Actions: []engine.Action{{Type: "log"}}},
		{ID: "rule5", Database: "test", Table: "logs", Events: []string{"INSERT"}, Filter: "true", Actions: []engine.Action{{Type: "log"}}},
	}
	// 添加更多规则以测试索引效果
	for i := 0; i < 995; i++ {
		rules = append(rules, engine.Rule{
			ID:       fmt.Sprintf("rule_%d", i+6),
			Database: "test",
			Table:    fmt.Sprintf("table_%d", i),
			Events:   []string{"INSERT"},
			Filter:   "true",
			Actions:  []engine.Action{{Type: "log"}},
		})
	}
	matcher := engine.NewMatcher(rules)
	event := createTestEvent("test.users", listener.ActionInsert)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = matcher.Match(event)
		}
	})
}

// BenchmarkEventProcessing_EndToEnd_Optimized 测试优化后的端到端性能
func BenchmarkEventProcessing_EndToEnd_Optimized(b *testing.B) {
	setupBenchmarkLogger()
	rules := createTestRules(100)
	matcher := engine.NewMatcher(rules)
	exec := executor.NewExecutor()
	event := createTestEvent("test.users", listener.ActionInsert)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			matchedRules, _ := matcher.Match(event)
			for _, rule := range matchedRules {
				_ = exec.ExecuteActions(ctx, rule.Actions, event)
			}
		}
	})
}
