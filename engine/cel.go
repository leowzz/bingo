package engine

import (
	"fmt"
	"sync"

	"bingo/listener"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
)

var (
	// celEnv 全局 CEL 环境，在第一次使用时初始化
	celEnv *cel.Env

	// filterCache 全局CEL表达式编译缓存（复杂表达式按需编译并缓存）
	filterCache sync.Map // map[string]cel.Program

	// varsPool CEL变量映射对象池，减少内存分配
	varsPool = sync.Pool{
		New: func() interface{} {
			return make(map[string]interface{}, 6)
		},
	}
)

// initCELEnv 初始化 CEL 环境
func initCELEnv() error {
	if celEnv != nil {
		return nil
	}

	// 定义变量类型
	// NewRow 和 OldRow 是 map[string]interface{} 类型
	// 其他字段是基本类型
	env, err := cel.NewEnv(
		cel.Declarations(
			// 事件基本信息
			decls.NewVar("Table", decls.String),
			decls.NewVar("Action", decls.String),
			decls.NewVar("Schema", decls.String),
			decls.NewVar("TableName", decls.String),
			// 行数据
			decls.NewVar("NewRow", decls.NewMapType(decls.String, decls.Dyn)),
			decls.NewVar("OldRow", decls.NewMapType(decls.String, decls.Dyn)),
		),
	)
	if err != nil {
		return fmt.Errorf("创建 CEL 环境失败: %w", err)
	}

	celEnv = env
	return nil
}

// EvaluateFilter 评估 CEL 过滤表达式
func EvaluateFilter(filter string, event *listener.Event) (bool, error) {
	// 如果过滤条件为空或为 "true"，直接返回 true
	if filter == "" || filter == "true" {
		return true, nil
	}

	// 初始化 CEL 环境
	if err := initCELEnv(); err != nil {
		return false, err
	}

	// 编译表达式
	ast, issues := celEnv.Compile(filter)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("编译 CEL 表达式失败: %w", issues.Err())
	}

	// 创建程序
	prg, err := celEnv.Program(ast)
	if err != nil {
		return false, fmt.Errorf("创建 CEL 程序失败: %w", err)
	}

	// 准备变量
	vars := map[string]interface{}{
		"Table":     event.Table,
		"Action":    string(event.Action),
		"Schema":    event.Schema,
		"TableName": event.TableName,
		"NewRow":    event.NewRow,
		"OldRow":    event.OldRow,
	}

	// 执行表达式
	out, _, err := prg.Eval(vars)
	if err != nil {
		return false, fmt.Errorf("执行 CEL 表达式失败: %w", err)
	}

	// 转换为布尔值
	result, ok := out.Value().(bool)
	if !ok {
		// 如果不是布尔值，尝试转换
		if out.Type() == types.BoolType {
			result = out.Value().(bool)
		} else {
			return false, fmt.Errorf("CEL 表达式结果不是布尔类型: %v", out.Value())
		}
	}

	return result, nil
}

// EvaluateFilterWithCache 评估 CEL 过滤表达式（带缓存）
//
// 优先从缓存中获取编译后的程序，如果不存在则编译并缓存。
func EvaluateFilterWithCache(filterKey string, event *listener.Event) (bool, error) {
	// 如果过滤条件为空或为 "true"，直接返回 true
	if filterKey == "" || filterKey == "true" {
		return true, nil
	}

	// 初始化 CEL 环境
	if err := initCELEnv(); err != nil {
		return false, err
	}

	// 从缓存中获取编译后的程序
	if cached, ok := filterCache.Load(filterKey); ok {
		prg := cached.(cel.Program)
		return evalProgram(prg, event)
	}

	// 缓存未命中，编译表达式
	ast, issues := celEnv.Compile(filterKey)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("编译 CEL 表达式失败: %w", issues.Err())
	}

	prg, err := celEnv.Program(ast)
	if err != nil {
		return false, fmt.Errorf("创建 CEL 程序失败: %w", err)
	}

	// 缓存编译后的程序
	filterCache.Store(filterKey, prg)

	// 执行程序
	return evalProgram(prg, event)
}

// evalProgram 执行CEL程序
func evalProgram(prg cel.Program, event *listener.Event) (bool, error) {
	// 从对象池获取变量映射
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
		return false, fmt.Errorf("执行 CEL 表达式失败: %w", err)
	}

	// 转换为布尔值
	result, ok := out.Value().(bool)
	if !ok {
		// 如果不是布尔值，尝试转换
		if out.Type() == types.BoolType {
			result = out.Value().(bool)
		} else {
			return false, fmt.Errorf("CEL 表达式结果不是布尔类型: %v", out.Value())
		}
	}

	return result, nil
}

// getVarsFromPool 从对象池获取变量映射
func getVarsFromPool() map[string]interface{} {
	vars := varsPool.Get().(map[string]interface{})
	// 清空map（保留容量）
	for k := range vars {
		delete(vars, k)
	}
	return vars
}

// putVarsToPool 将变量映射归还到对象池
func putVarsToPool(vars map[string]interface{}) {
	// 清空map内容（保留容量）
	for k := range vars {
		delete(vars, k)
	}
	varsPool.Put(vars)
}
