package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// RedirectStdLog 重定向标准库的 log 输出到 zap
func RedirectStdLog() {
	if Logger == nil {
		return
	}

	// 使用 zap 的 RedirectStdLogAt 功能
	// 这会替换标准库的 log 输出，将标准库的日志重定向到我们的 zap logger
	// 这样 canal 库使用标准库 log 的输出就会被我们的 logger 捕获
	undo, err := zap.RedirectStdLogAt(Logger, zapcore.InfoLevel)
	if err == nil {
		// 保存 undo 函数以便后续恢复（如果需要）
		_ = undo
	}
}

var Logger *zap.Logger
var Sugar *zap.SugaredLogger

// InitLogger 初始化日志
func InitLogger(level, format, output, file string, maxSize, maxBackups, maxAge int) error {
	// 解析日志级别
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	// 配置编码器
	var encoderConfig zapcore.EncoderConfig
	if format == "json" {
		encoderConfig = zap.NewProductionEncoderConfig()
	} else {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	var encoder zapcore.Encoder
	if format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// 配置输出
	var writeSyncer zapcore.WriteSyncer
	if output == "file" && file != "" {
		// 文件输出，支持日志轮转
		lumberjackLogger := &lumberjack.Logger{
			Filename:   file,
			MaxSize:    maxSize, // MB
			MaxBackups: maxBackups,
			MaxAge:     maxAge, // days
			Compress:   true,
		}
		writeSyncer = zapcore.AddSync(lumberjackLogger)
	} else {
		// 标准输出
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	// 创建核心
	core := zapcore.NewCore(encoder, writeSyncer, zapLevel)

	// 创建 logger，跳过一层调用（因为我们的封装函数）
	// 这样日志会显示实际调用者的位置，而不是封装函数的位置
	Logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	Sugar = Logger.Sugar()

	// 重定向标准库的 log 输出到 zap（用于 canal 等库的日志）
	RedirectStdLog()

	return nil
}

// Sync 同步日志缓冲区
func Sync() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}

// 以下函数提供简化的调用方式，直接使用 logger.Infow() 而不是 logger.Sugar.Infow()

// Debug 记录 Debug 级别日志
func Debug(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Debugw(msg, keysAndValues...)
	}
}

// Info 记录 Info 级别日志
func Info(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Infow(msg, keysAndValues...)
	}
}

// Infof 记录 Info 级别日志（格式化字符串）
func Infof(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Infof(template, args...)
	}
}

// Warn 记录 Warn 级别日志
func Warn(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Warnw(msg, keysAndValues...)
	}
}

// Warnf 记录 Warn 级别日志（格式化字符串）
func Warnf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Warnf(template, args...)
	}
}

// Error 记录 Error 级别日志
func Error(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Errorw(msg, keysAndValues...)
	}
}

// Errorf 记录 Error 级别日志（格式化字符串）
func Errorf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Errorf(template, args...)
	}
}

// Fatal 记录 Fatal 级别日志并退出
func Fatal(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Fatalw(msg, keysAndValues...)
	}
}

// Fatalf 记录 Fatal 级别日志并退出（格式化字符串）
func Fatalf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Fatalf(template, args...)
	}
}

// Debugw 记录 Debug 级别日志（with fields）
func Debugw(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Debugw(msg, keysAndValues...)
	}
}

// Infow 记录 Info 级别日志（with fields）
func Infow(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Infow(msg, keysAndValues...)
	}
}

// Warnw 记录 Warn 级别日志（with fields）
func Warnw(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Warnw(msg, keysAndValues...)
	}
}

// Errorw 记录 Error 级别日志（with fields）
func Errorw(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Errorw(msg, keysAndValues...)
	}
}

// Fatalw 记录 Fatal 级别日志并退出（with fields）
func Fatalw(msg string, keysAndValues ...interface{}) {
	if Sugar != nil {
		Sugar.Fatalw(msg, keysAndValues...)
	}
}
