package logger

import (
	"io"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Benchmark 测试说明：
//
// 运行所有 benchmark 测试：
//   go test -bench=. -benchmem ./internal/logger
//
// 运行特定测试：
//   go test -bench=BenchmarkInfo -benchmem ./internal/logger
//
// 对比 JSON 和 Text 格式：
//   go test -bench=BenchmarkFormatComparison -benchmem ./internal/logger
//
// 性能指标说明：
//   - ns/op: 每次操作耗时（纳秒）
//   - B/op: 每次操作分配的内存（字节）
//   - allocs/op: 每次操作的内存分配次数
//
// 注意：测试输出到 io.Discard，避免实际文件 I/O 影响性能测试结果
//
// 性能对比说明：
//   - JSON: 使用 zap 原生 JSONEncoder（生产环境格式）
//   - Text: 使用自定义 Text Formatter（自定义格式）
//   - NativeZap: 使用 zap 原生 ConsoleEncoder（开发环境格式，用于对比）

// setupLoggerNoColor 设置无颜色的 logger（输出到 io.Discard）
func setupLoggerNoColor() {
	// 解析日志级别
	zapLevel := zapcore.InfoLevel

	// 配置编码器
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    customLevelEncoder,
		EncodeTime:     customTimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	encoder := newCustomEncoder(encoderConfig, false, true) // enableGoroutineID=true 用于测试

	// 输出到 io.Discard，避免实际 I/O 影响性能测试
	writeSyncer := zapcore.AddSync(io.Discard)

	// 创建核心
	core := zapcore.NewCore(encoder, writeSyncer, zapLevel)

	// 创建 logger
	Logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	Sugar = Logger.Sugar()
}

// setupLoggerNoColor 设置无颜色的 logger（输出到 io.Discard）
func setupLoggerNoGoroutineID() {
	// 解析日志级别
	zapLevel := zapcore.InfoLevel

	// 配置编码器
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    customLevelEncoder,
		EncodeTime:     customTimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	encoder := newCustomEncoder(encoderConfig, false, false) // enableGoroutineID=true 用于测试

	// 输出到 io.Discard，避免实际 I/O 影响性能测试
	writeSyncer := zapcore.AddSync(io.Discard)

	// 创建核心
	core := zapcore.NewCore(encoder, writeSyncer, zapLevel)

	// 创建 logger
	Logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	Sugar = Logger.Sugar()
}

// setupLoggerJSON 设置 JSON 格式的 logger（输出到 io.Discard）
func setupLoggerJSON() {
	zapLevel := zapcore.InfoLevel
	encoderConfig := zap.NewProductionEncoderConfig()
	encoder := zapcore.NewJSONEncoder(encoderConfig)

	// 输出到 io.Discard
	writeSyncer := zapcore.AddSync(io.Discard)

	core := zapcore.NewCore(encoder, writeSyncer, zapLevel)
	Logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	Sugar = Logger.Sugar()
}

// setupNativeZapLogger 设置原生 zap logger（ConsoleEncoder，用于性能对比）
func setupNativeZapLogger() {
	zapLevel := zapcore.InfoLevel

	// 使用原生 zap 的 ConsoleEncoder（开发环境常用的文本格式）
	encoderConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(encoderConfig)

	// 输出到 io.Discard
	writeSyncer := zapcore.AddSync(io.Discard)

	core := zapcore.NewCore(encoder, writeSyncer, zapLevel)
	Logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	Sugar = Logger.Sugar()
}

// BenchmarkInfo 测试 Info 级别日志性能
func BenchmarkInfo(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Info("测试日志消息")
		}
	})
}

// BenchmarkInfoWithFields 测试带字段的 Info 日志性能
func BenchmarkInfoWithFields(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Infow("测试日志消息",
				"key1", "value1",
				"key2", 123,
				"key3", true,
			)
		}
	})
}

// BenchmarkInfoManyFields 测试多个字段的 Info 日志性能
func BenchmarkInfoManyFields(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Infow("测试日志消息",
				"key1", "value1",
				"key2", "value2",
				"key3", "value3",
				"key4", "value4",
				"key5", "value5",
				"key6", 123,
				"key7", 456,
				"key8", true,
				"key9", false,
				"key10", 3.14,
			)
		}
	})
}

// BenchmarkInfof 测试格式化字符串的 Info 日志性能
func BenchmarkInfof(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Infof("测试日志消息: %s, %d, %v", "value1", 123, true)
		}
	})
}

// BenchmarkError 测试 Error 级别日志性能
func BenchmarkError(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Error("测试错误消息")
		}
	})
}

// BenchmarkErrorWithFields 测试带字段的 Error 日志性能
func BenchmarkErrorWithFields(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Errorw("测试错误消息",
				"error", "something went wrong",
				"code", 500,
				"table", "users",
			)
		}
	})
}

// BenchmarkDebug 测试 Debug 级别日志性能（会被过滤）
func BenchmarkDebug(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Debug("测试调试消息")
		}
	})
}

// BenchmarkWarn 测试 Warn 级别日志性能
func BenchmarkWarn(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Warn("测试警告消息")
		}
	})
}

// BenchmarkMixedLevels 测试混合日志级别性能
func BenchmarkMixedLevels(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 4 {
			case 0:
				Info("info message")
			case 1:
				Warn("warn message")
			case 2:
				Error("error message")
			case 3:
				Debug("debug message")
			}
			i++
		}
	})
}

// BenchmarkJSONFormat 测试 JSON 格式日志性能
func BenchmarkJSONFormat(b *testing.B) {
	setupLoggerJSON()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Infow("测试日志消息",
				"key1", "value1",
				"key2", 123,
				"key3", true,
			)
		}
	})
}

// BenchmarkTextFormat 测试文本格式日志性能（自定义格式）
func BenchmarkTextFormat(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Infow("测试日志消息",
				"key1", "value1",
				"key2", 123,
				"key3", true,
			)
		}
	})
}

// BenchmarkFormatComparison 对比 JSON、Text 和原生 Zap 格式性能
func BenchmarkFormatComparison(b *testing.B) {
	b.Run("JSON", func(b *testing.B) {
		setupLoggerJSON()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				Infow("消息", "key", "value")
			}
		})
	})

	b.Run("Text", func(b *testing.B) {
		setupLoggerNoColor()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				Infow("消息", "key", "value")
			}
		})
	})

	b.Run("Text_NoGoroutineID", func(b *testing.B) {
		setupLoggerNoGoroutineID()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				Infow("消息", "key", "value")
			}
		})
	})

	b.Run("NativeZap", func(b *testing.B) {
		setupNativeZapLogger()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				Infow("消息", "key", "value")
			}
		})
	})
}

// BenchmarkComplexEvent 测试复杂事件日志性能（模拟实际使用场景）
func BenchmarkComplexEvent(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Infow("规则匹配成功",
				"table", "users",
				"action", "insert",
				"matched_count", 2,
				"rule_ids", []string{"rule1", "rule2"},
				"timestamp", 1234567890,
				"has_old_row", false,
				"has_new_row", true,
			)
		}
	})
}

// BenchmarkLoggerSimple 测试简单日志消息性能
func BenchmarkLoggerSimple(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Info("测试日志消息")
		}
	})
}

// BenchmarkConcurrentLogging 测试并发日志写入性能
func BenchmarkConcurrentLogging(b *testing.B) {
	setupLoggerNoColor()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Infow("并发测试",
				"goroutine_id", 123,
				"event_id", 456,
				"status", "processing",
			)
		}
	})
}

// BenchmarkNativeZap 测试原生 zap logger 性能（用于对比）
func BenchmarkNativeZap(b *testing.B) {
	setupNativeZapLogger()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Info("测试日志消息")
		}
	})
}

// BenchmarkNativeZapWithFields 测试原生 zap logger 带字段的性能（用于对比）
func BenchmarkNativeZapWithFields(b *testing.B) {
	setupNativeZapLogger()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Infow("测试日志消息",
				"key1", "value1",
				"key2", 123,
				"key3", true,
			)
		}
	})
}
