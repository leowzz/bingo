package engine

import (
	"fmt"

	"bingo/listener"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
)

var (
	// celEnv 全局 CEL 环境，在第一次使用时初始化
	celEnv *cel.Env
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
